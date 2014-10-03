package images_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/flusher"
	"github.com/nitrous-io/nitrous-common/logger"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("controllers/images", func() {
	var (
		db  *db.Database
		svr *server.Server
	)

	BeforeEach(func() {
		var err error

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		svr = server.NewServer("", db, &logger.Logger{Level: logger.LogLevelQuiet})
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/hosts/{slug}/images/{image}/push", func() {
		It("calls images/push on the freighter server", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			fs.start(db, ctx)
			defer fs.Close()

			u, h := ctx.user, ctx.host
			ctx.insertImage(fs, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(fs.pushCalled).To(BeTrue())
		})

		It("requires repository param", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			defer fs.Close()

			u, h := ctx.user, ctx.host
			ctx.insertImage(fs, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{})
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("repository is required"))
			Expect(fs.pushCalled).To(BeFalse())
		})

		It("returns 404 if the image could not be located", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			defer fs.Close()

			u, h := ctx.user, ctx.host
			ctx.insertImage(fs, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			res := ctx.doPush(svr, "foo", "scratchy:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
			Expect(fs.pushCalled).To(BeFalse())
		})

		It("returns 401 if freighterclient returns an ErrImagePushAuthRequired", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			fs.start(db, ctx)
			defer fs.Close()

			u, h := ctx.user, ctx.host
			ctx.insertImage(fs, db)
			_, err := image.Find(db, u, h, "scratch:latest")
			Expect(err).To(BeNil(), "failed to find image after push")

			fs.pushUnAuthed = true

			res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("registry_auth_failed"))
			Expect(errDesc).To(Equal(freighterclient.ErrImagePushAuthRequired.Error()))
			Expect(fs.pushCalled).To(BeTrue())
		})

		Context("event publishing", func() {
			It("publishes an image pushing event with progress data", func() {
				fs := newFreighterServer("scratch", "latest")
				ctx := newImgTestCtx(fs, db)
				fs.start(db, ctx)
				defer fs.Close()

				u, h := ctx.user, ctx.host
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())
				ctx.insertImage(fs, db)

				ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})

				timeout := time.After(100 * time.Millisecond)
				select {
				case ev := <-ch:
					// check if the first event we got is image pushing event
					e := ev.(*controllers.Event)
					Expect(e.Name).To(Equal(controllers.EventImagePushing))

					i, err := image.Find(db, u, h, "scratch:latest")
					Expect(err).To(BeNil())
					data := e.Data.(*controllers.ProgressEvent)
					Expect(data.ID).To(Equal(i.ID), "emitted image pushing with invalid image")
					Expect(data.Progress).To(Equal(`{"status":"Pushing..."}`), "failed to include progress data")
				case <-timeout:
					Fail("expected EventImagePushing but timed out")
				}
			})

			It("publishes an image pushed event with the image and the target repository", func() {
				fs := newFreighterServer("scratch", "latest")
				ctx := newImgTestCtx(fs, db)
				fs.start(db, ctx)
				defer fs.Close()

				u, h := ctx.user, ctx.host
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())
				ctx.insertImage(fs, db)

				res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
				Expect(res.Code).To(Equal(http.StatusOK))

				var e *controllers.Event
				timeout := time.After(1000 * time.Millisecond)
				for {
					if e != nil && e.Name == controllers.EventImagePushed {
						break
					}
					select {
					case ev := <-ch:
						e = ev.(*controllers.Event)
					case <-timeout:
						Fail("expected EventImagePushed but timed out")
					}
				}
				evtData, ok := e.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				img, ok := evtData["image"].(*serializers.Image)
				Expect(ok).To(BeTrue())
				i, err := image.Find(db, u, h, "scratch:latest")
				Expect(err).To(BeNil())
				Expect(img).To(Equal(serializers.SerializeImage(i)))
				Expect(evtData["repository"]).To(Equal("nitrousio/devenv"))
			})

			It("publishes an image pushed failed event with the image, target repository and the error", func() {
				fs := newFreighterServer("scratch", "latest")
				fs.pushError = true
				ctx := newImgTestCtx(fs, db)
				fs.start(db, ctx)
				defer fs.Close()

				u, h := ctx.user, ctx.host
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())
				ctx.insertImage(fs, db)

				res := ctx.doPush(svr, "foo", "scratch:latest", &url.Values{"repository": []string{"nitrousio/devenv"}})
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("internal_server_error"))

				var e *controllers.Event
				timeout := time.After(1000 * time.Millisecond)
				for {
					if e != nil && e.Name == controllers.EventImagePushFailed {
						break
					}
					select {
					case ev := <-ch:
						e = ev.(*controllers.Event)
					case <-timeout:
						Fail("expected EventImagePushFailed but timed out")
					}
				}
				evtData, ok := e.Data.(map[string]interface{})
				Expect(ok).To(BeTrue())
				img, ok := evtData["image"].(*serializers.Image)
				Expect(ok).To(BeTrue())
				i, err := image.Find(db, u, h, "scratch:latest")
				Expect(err).To(BeNil())
				Expect(img).To(Equal(serializers.SerializeImage(i)))
				Expect(evtData["repository"]).To(Equal("nitrousio/devenv"))
				Expect(evtData["error"]).NotTo(BeEmpty())
			})

		})

		It("returns a 202 Accepted when the async param is sent", func() {
			fs := newFreighterServer("scratch", "latest")
			ctx := newImgTestCtx(fs, db)
			fs.start(db, ctx)
			defer fs.Close()

			ctx.insertImage(fs, db)

			params := &url.Values{}
			params.Set("async", "true")
			params.Set("repository", "nitrousio/devenv")
			res := ctx.doPush(svr, "foo", "scratch:latest", params)
			Expect(res.Code).To(Equal(http.StatusAccepted))
		})
	})
})

func (c *imgTestCtx) doPush(svr *server.Server, host, slug string, params *url.Values) *httptest.ResponseRecorder {
	req, err := NewAuthedFormRequest(c.token, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/%v/push", host, slug), params)
	Expect(err).To(BeNil(), "failed to POST images/push")
	res := httptest.NewRecorder()
	svr.Router().ServeHTTP(res, req)
	return res
}

func (s *freighterServer) pushImage(w http.ResponseWriter, r *http.Request) {
	s.pushCalled = true

	if s.pushUnAuthed {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, `{"error":"Bob? Is that you?"}`)
		return
	}

	if s.pushError {
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
	fmt.Fprintln(stream, `{"status":"Pushing..."}`)
	fmt.Fprintln(stream, `{"status":"Pushing", "progress":"1/? (n/a)", "progressDetail":{"current":1}}`)
	fmt.Fprintln(stream, `{"status":"Pushing", "progress":"2/? (n/a)", "progressDetail":{"current":1}}`)
}
