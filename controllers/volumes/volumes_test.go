package volumes_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/volumes")
}

var _ = Describe("controllers/volumes", func() {
	var (
		db              *db.Database
		svr             *server.Server
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = WrapInTransaction(db, u.Insert)
		Expect(err).To(BeNil())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug:      "myhost",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
			U: u,
		})
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		UnstubFreighterFn()
	})

	makeVolumes := func(slugs ...string) {
		for _, s := range slugs {
			v := volume.Volume{HostID: hs.H.ID, Slug: s}
			err := v.Insert(db)
			Expect(err).To(BeNil())
		}
	}

	Describe("GET /api/v0/hosts/:slug/volumes", func() {
		doRequest := func() {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%s/volumes", hs.H.Slug), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		type vols struct {
			Volumes []struct {
				Slug string `json:"slug"`
			} `json:"volumes"`
		}

		var j *vols

		BeforeEach(func() {
			j = &vols{}
		})

		Context("host has volumes", func() {
			BeforeEach(func() {
				makeVolumes("shared", "music", "db")
			})

			It("returns volumes in json", func() {
				doRequest()
				Expect(mockClient.ListVolumesCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusOK))

				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Volumes).To(HaveLen(3))
				slugs := []string{}
				for _, v := range j.Volumes {
					slugs = append(slugs, v.Slug)
				}
				// TODO on Gomega upgrade, can use ConsistOf([]string{"db", "music", shared"})
				Expect(slugs).To(ContainElement("db"))
				Expect(slugs).To(ContainElement("music"))
				Expect(slugs).To(ContainElement("shared"))
			})
		})

		Context("host has no volumes", func() {
			It("returns json", func() {
				doRequest()
				Expect(mockClient.ListVolumesCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusOK))

				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Volumes).To(HaveLen(0))
			})
		})

		Context("error encountered", func() {
			BeforeEach(func() {
				db.Close() // simulate error
			})

			It("returns error", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				slug, _ := ErrorSlug(res.Body)
				Expect(slug).To(ContainSubstring("internal_server_error"))
			})
		})
	})

	Describe("POST /api/v0/hosts/:slug/volumes", func() {
		doRequest := func(vol string) {
			params := &url.Values{"slug": []string{vol}, "name": []string{"some label"}}
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%s/volumes", hs.H.Slug), params)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		Context("successful create", func() {
			It("returns an error without calling freighterclient", func() {
				doRequest("metric")
				Expect(mockClient.CreateVolumeCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusCreated))
				j := struct {
					Volume volume.Volume
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				vols, err := volume.FindAll(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(vols).To(HaveLen(1))
				Expect(vols[0].Slug).To(Equal("metric"))
				Expect(vols[0].Name).To(Equal("some label"))
			})

			It("publishes a volume created event with the created volume", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest("metric")

				v, err := volume.Find(db, hs.H.ID, "metric")
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: controllers.EventVolumeCreated,
					Data: map[string]*serializers.Volume{
						"volume": serializers.SerializeVolume(v),
					},
				}))
				close(done)
			})

			It("converts the 'slug' form value to lowercase", func() {
				doRequest("TitleCaseSlug")
				Expect(mockClient.CreateVolumeCalled).To(Equal(1))
				Expect(mockClient.CreateVolumeSlug).To(Equal("titlecaseslug"))

				vols, err := volume.FindAll(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(vols).To(HaveLen(1))
				Expect(vols[0].Slug).To(Equal("titlecaseslug"))
			})
		})

		Context("when slug already exists", func() {
			BeforeEach(func() {
				v := &volume.Volume{HostID: hs.H.ID, Slug: "taken"}
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())
			})

			It("returns an error without calling freighterclient", func() {
				doRequest("taken")
				Expect(mockClient.CreateVolumeCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				_, desc := ErrorSlug(res.Body)
				Expect(desc).To(ContainSubstring("already exists"))
			})
		})

		Context("when Freighter client returns error", func() {
			BeforeEach(func() {
				mockClient.CreateVolumeError = fmt.Errorf("volume too loud")
			})

			It("responds with 500 and no volume is added to db", func() {
				doRequest("sonic-boom")
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				Expect(mockClient.CreateVolumeCalled).To(Equal(1))
				vols, err := volume.FindAll(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(vols).To(HaveLen(0))
			})
		})

		Context("when host is unavailable", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with 503 and no volume is added to db", func() {
				doRequest("sonic-boom")
				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				Expect(mockClient.CreateVolumeCalled).To(Equal(0))
				vols, err := volume.FindAll(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(vols).To(HaveLen(0))
			})
		})
	})

	Describe("DELETE /api/v0/hosts/:slug/volumes/:vslug", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedRequest(t, "DELETE", fmt.Sprintf("/api/v0/hosts/%s/volumes/%s", hs.H.Slug, vol), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			makeVolumes("deprecated", "conditioner")
			v, err := volume.Find(db, hs.H.ID, "deprecated")
			Expect(err).To(BeNil())
			Expect(v).NotTo(BeNil())
		})

		Context("successful destroy", func() {
			It("removes volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Volume *serializers.Volume
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				v, err := volume.Find(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				Expect(v).To(BeNil())

				v, err = volume.Find(db, hs.H.ID, "conditioner")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(v.Slug).To(Equal("conditioner"))
			})

			It("publishes a volume destroyed event with the destroyed volume", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest("deprecated")

				msg := <-ch
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventVolumeDestroyed))

				evtData, ok := evt.Data.(map[string]*serializers.Volume)
				Expect(ok).To(BeTrue())
				Expect(evtData["volume"].Slug).To(Equal("deprecated"))
				close(done)
			})
		})

		Context("when volume is not found", func() {
			It("returns 404 without calling freighterclient", func() {
				doRequest("nosuch")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when Freighter client returns error", func() {
			BeforeEach(func() {
				mockClient.RemoveVolumeError = fmt.Errorf("volume too loud")
			})

			It("returns 500 without destroying volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				slug, _ := ErrorSlug(res.Body)
				Expect(slug).To(Equal("internal_server_error"))
				v, err := volume.Find(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(v.Slug).To(Equal("deprecated"))
			})
		})
	})

	Describe("GET /api/v0/hosts/:slug/volumes/:vslug", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%s/volumes/%s", hs.H.Slug, vol), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		Context("when volume exist", func() {
			It("returns volume in JSON", func() {
				v := volume.Volume{HostID: hs.H.ID, Slug: "music", Name: "my music volume"}
				err := v.Insert(db)
				Expect(err).To(BeNil())

				doRequest("music")
				var j struct {
					Volume *serializers.Volume `json:"volume"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Volume.ID).To(Equal(v.ID))
				Expect(j.Volume.Name).To(Equal("my music volume"))
				Expect(j.Volume.Slug).To(Equal("music"))
				Expect(j.Volume.HostID).To(Equal(hs.H.ID))
				Expect(j.Volume.CreatedAt.IsZero()).NotTo(BeTrue())
				Expect(j.Volume.UpdatedAt.IsZero()).NotTo(BeTrue())
			})
		})

		Context("when volume is not found", func() {
			It("returns 404 Not Found", func() {
				doRequest("nosuch")
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})
	})

	Describe("GET /api/v0/hosts/:slug/volumes/:vslug/info", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%s/volumes/%s/info", hs.H.Slug, vol), nil)
			Expect(err).To(BeNil())
			svr.Router().ServeHTTP(res, req)
		}

		BeforeEach(func() {
			makeVolumes("music")
			v, err := volume.Find(db, hs.H.ID, "music")
			Expect(err).To(BeNil())
			Expect(v).NotTo(BeNil())
		})

		Context("successful get", func() {
			BeforeEach(func() {
				mockClient.VolumeInfoReturn = &freighterclient.Volume{
					Size: 666,
				}
			})

			It("returns info", func() {
				doRequest("music")
				Expect(mockClient.VolumeInfoCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Vol struct {
						Size uint64 `json:"size"`
					} `json:"info"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Vol.Size).To(Equal(uint64(666)))
			})
		})

		Context("when volume is not found", func() {
			It("returns 404 without calling freighterclient", func() {
				doRequest("nosuch")
				Expect(mockClient.VolumeInfoCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when Freighter client returns error", func() {
			BeforeEach(func() {
				mockClient.VolumeInfoError = fmt.Errorf("volume is deafening")
			})

			It("returns 500", func() {
				doRequest("music")
				Expect(mockClient.VolumeInfoCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				slug, _ := ErrorSlug(res.Body)
				Expect(slug).To(Equal("internal_server_error"))
			})
		})
	})
})
