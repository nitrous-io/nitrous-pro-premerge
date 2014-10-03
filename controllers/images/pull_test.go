package images_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/nitrous-io/nitrous-common/flusher"
	"github.com/nitrous-io/nitrous-common/logger"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"
	"github.com/nitrous-io/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	cert.KeyLen = 512
}

var _ = Describe("controllers/images/pull", func() {
	var (
		db  *db.Database
		svr *server.Server
	)

	BeforeEach(func() {
		var err error
		db, err = test.TestDatabase("nitrous_test")
		Expect(err).To(BeNil(), "failed to setup test database")
		svr = server.NewServer("", db, &logger.Logger{Level: logger.LogLevelQuiet})
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	It("creates an image record", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		ctx.doPull(svr, "foo", &url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		})

		u, h := ctx.user, ctx.host
		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to store image in db")
		Expect(i.UserID).To(Equal(u.ID))
		Expect(i.HostID).To(Equal(h.ID))
		Expect(i.FreighterID).To(Equal("123asd"))
		Expect(i.Name).To(Equal("scratcher v1 test"))
		Expect(i.Slug).To(Equal("scratch"))
		Expect(i.Tag).To(Equal("latest"))
		Expect(i.CreatedAt).NotTo(BeNil())
		Expect(i.UpdatedAt).NotTo(BeNil())
	})

	It("uses the default image if repository is not given", func() {
		fs := newFreighterServer("nitrousio/ubuntu", "trusty")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		ctx.doPull(svr, "foo", nil)

		u, h := ctx.user, ctx.host
		i, err := image.Find(db, u, h, "nitrousio/ubuntu:trusty")
		Expect(err).To(BeNil(), "failed to store image in db")
		Expect(i.Slug).To(Equal("nitrousio/ubuntu"))
		Expect(i.Tag).To(Equal("trusty"))
	})

	It("removes the image and emits images removed event when freighter failed to pull image", func() {
		fs := newFreighterServer("scratch", "latest")
		fs.pullError = true
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		u, h := ctx.user, ctx.host
		ch := svr.Events.Subscribe(u.Topic())
		defer svr.Events.Unsubscribe(ch, u.Topic())

		ctx.doPull(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(Equal(sql.ErrNoRows), "expect the image to not exist in db, but got %v", i)

		ech := make(chan *controllers.Event, 1)

		go func() {
			var e *controllers.Event
			for {
				e = (<-ch).(*controllers.Event)
				if e.Name == controllers.EventImageRemoved {
					ech <- e
					break
				}
			}
		}()

		select {
		case <-time.After(50 * time.Millisecond):
			Fail("timeout while getting image removed event")
		case e := <-ech:
			evtData, ok := e.Data.(map[string]*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(evtData["image"].Repo).To(Equal("scratch:latest"), "failed to serialize image response")
		}
	})

	It("responds with progress data and image data on success", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		res := ctx.doPull(svr, "foo", &url.Values{
			"repository": []string{"scratch"},
			"name":       []string{"scratcher v1 test"},
		})

		j := struct {
			Image serializers.Image `json:"image"`
		}{}

		// includes progress data
		Expect(res.Body).To(ContainSubstring(`{"status":"Pulling..."}`), "failed to include progress data")

		// keep parsing and ignore the progress data to get the final image
		scanner := bufio.NewScanner(res.Body)
		for scanner.Scan() {
			json.Unmarshal(scanner.Bytes(), &j)
			if j.Image.ID > 0 {
				break
			}
		}
		Expect(j.Image.FreighterID).To(Equal("123asd"), "missing final image in response")
	})

	It("publishes an image pulling event with progress data", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		u, h := ctx.user, ctx.host
		ch := svr.Events.Subscribe(u.Topic())
		defer svr.Events.Unsubscribe(ch, u.Topic())

		ctx.doPull(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to find image after pull")

		timeout := time.After(100 * time.Millisecond)
		select {
		case ev := <-ch:
			// check if the first event we got is image pulling event
			e := ev.(*controllers.Event)
			data := e.Data.(*controllers.ProgressEvent)
			Expect(e.Name).To(Equal(controllers.EventImagePulling), "expected image pulling event")
			Expect(data.ID).To(Equal(i.ID), "emitted image pulling with invalid image")
			Expect(data.Progress).To(Equal(`{"status":"Pulling..."}`), "failed to include progress data")
		case <-timeout:
			Fail("expected EventImagePulled but timed out")
		}
	})

	It("publishes an image pulled event with the pulled image", func() {
		fs := newFreighterServer("scratch", "latest")
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		u, h := ctx.user, ctx.host
		ch := svr.Events.Subscribe(u.Topic())
		defer svr.Events.Unsubscribe(ch, u.Topic())

		ctx.doPull(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		i, err := image.Find(db, u, h, "scratch:latest")
		Expect(err).To(BeNil(), "failed to find image after pull")

		var e *controllers.Event
		timeout := time.After(1000 * time.Millisecond)
		for {
			if e != nil && e.Name == controllers.EventImagePulled {
				break
			}
			select {
			case ev := <-ch:
				e = ev.(*controllers.Event)
			case <-timeout:
				Fail("expected EventImagePulled but timed out")
			}
		}
		evtData, ok := e.Data.(map[string]*serializers.Image)
		Expect(ok).To(BeTrue())
		img := evtData["image"]
		Expect(img.ID).To(Equal(i.ID), "failed to serialize correct image")
		Expect(img.FreighterID).To(Equal("123asd"), "failed to serialize freighter response")
	})

	It("returns 500 on freighterclient error and emits image pull failed event", func() {
		fs := newFreighterServer("scratch", "latest")
		fs.pullError = true
		ctx := newImgTestCtx(fs, db)
		fs.start(db, ctx)
		defer fs.Close()

		ch := svr.Events.Subscribe(ctx.user.Topic())
		defer svr.Events.Unsubscribe(ch, ctx.user.Topic())

		res := ctx.doPull(svr, "foo", &url.Values{"repository": []string{"scratch"}})

		errType, _ := test.ErrorSlug(res.Body)
		Expect(errType).To(Equal("internal_server_error"))

		ech := make(chan *controllers.Event, 1)

		go func() {
			var e *controllers.Event
			for {
				e = (<-ch).(*controllers.Event)
				if e.Name == controllers.EventImagePullFailed {
					ech <- e
					break
				}
			}
		}()

		select {
		case <-time.After(50 * time.Millisecond):
			Expect(true).To(BeFalse(), "timeout while getting image pull failed event")
		case e := <-ech:
			evtData, ok := e.Data.(map[string]*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(evtData["image"].Slug).To(Equal("scratch"), "failed to serialize freighter response")
		}
	})

	Context("when image already exists", func() {
		It("updates the existing image record with the new info from Freighter", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)
			fs.imageUpdated = true
			fs.start(db, ctx)
			defer fs.Close()

			ctx.doPull(svr, "foo", &url.Values{
				"repository": []string{"scratch"},
				"update":     []string{"true"},
			})

			i, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find stored image after pull")
			Expect(i.FreighterID).To(Equal("123asd"))
		})

		It("does not update the existing image record if update=false", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)

			fs.imageUpdated = false
			fs.start(db, ctx)
			defer fs.Close()

			ctx.doPull(svr, "foo", &url.Values{
				"repository": []string{"scratch"},
				"update":     []string{"false"},
			})

			i2, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil())
			Expect(i2.FreighterID).To(Equal("cafebabe"))
		})

		It("does not remove the image when freighter fails to pull image", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)

			fs.imageUpdated = true
			fs.pullError = true
			fs.start(db, ctx)
			defer fs.Close()

			ctx.doPull(svr, "foo", &url.Values{"repository": []string{"scratch"}})

			_, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil(), "expect the old image to not be deleted")
		})

		It("does not emit images removed event when freighter fails to pull image", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			ctx.insertImage(fs, db)

			fs.imageUpdated = true
			fs.pullError = true
			fs.start(db, ctx)
			defer fs.Close()

			ch := svr.Events.Subscribe(ctx.user.Topic())
			defer svr.Events.Unsubscribe(ch, ctx.user.Topic())

			ctx.doPull(svr, "foo", &url.Values{"repository": []string{"scratch"}})
			close(ch)

			_, err := image.Find(db, ctx.user, ctx.host, "scratch:latest")
			Expect(err).To(BeNil(), "expect the old image to not be deleted")

			failed := make(chan *controllers.Event, 1)
			finished := make(chan struct{}, 1)

			go func() {
				var e *controllers.Event
				var ok bool
				for {
					e, ok = (<-ch).(*controllers.Event)
					if !ok {
						close(finished)
						break
					}
					if e.Name == controllers.EventImageRemoved {
						failed <- e
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("timeout while getting image image removed event")
			case fe := <-failed:
				Expect(fe).To(BeNil(), "should not receive image removed event")
			case <-finished:
				// finished without receiving image removed
			}
		})
	})

	Context("when we want to pull asynchronously", func() {
		It("returns 202 Accepted with the image stored in db", func() {
			fs := newFreighterServer("scratch", "latest")
			defer fs.Close()

			ctx := newImgTestCtx(fs, db)

			params := &url.Values{}
			params.Set("async", "true")
			params.Set("repository", "scratch")
			res := ctx.doPull(svr, "foo", params)

			Expect(res.Code).To(Equal(http.StatusAccepted))

			j := struct {
				Image serializers.Image `json:"image"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			// These fields are not yet set.
			Expect(j.Image.FreighterID).To(BeEmpty())
			Expect(j.Image.BuiltAt).To(BeNil())
		})
	})
})

func (c *imgTestCtx) doPull(svr *server.Server, host string, params *url.Values) *httptest.ResponseRecorder {
	req, err := test.NewAuthedFormRequest(c.token, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/pull", host), params)
	Expect(err).To(BeNil(), "failed to POST images/pull")
	res := httptest.NewRecorder()
	svr.Router().ServeHTTP(res, req)
	return res
}

func (s *freighterServer) pullImage(w http.ResponseWriter, r *http.Request) {
	if s.pullError {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"error":"internal server error. dont know why"}`)
		return
	}

	// If image exists, return HTTP 304 Not Modified.
	if r.PostFormValue("update") == "false" && s.existingImage != nil {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	stream := flusher.NewWriteFlusher(w)
	fmt.Fprintln(stream, `{"status":"Pulling..."}`)
	fmt.Fprintln(stream, `{"status":"Pulling","progress":"1 B/ 100 B"}`)
	fmt.Fprintln(stream, `{"status":"Pulling","progress":"2 B/ 100 B"}`)
}
