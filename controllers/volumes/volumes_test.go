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

	"github.com/nitrous-io/nitrous/controllers/volumes"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/models/volume"
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
		volumes.FreighterClient = mockClient

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
		err = u.Insert(db)
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
				errType, _ := ErrorDetails(res.Body)
				Expect(errType).To(ContainSubstring("internal_server_error"))
			})
		})
	})

	Describe("POST /api/v0/hosts/:slug/volumes", func() {
		doRequest := func(vol string) {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%s/volumes", hs.H.Slug), &url.Values{"slug": []string{vol}})
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

				vols, err := volume.FindAllByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(vols).To(HaveLen(1))
				Expect(vols[0].Slug).To(Equal("metric"))
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
				_, errDesc := ErrorDetails(res.Body)
				Expect(errDesc).To(ContainSubstring("already exists"))
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
				vols, err := volume.FindAllByHostID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(vols).To(HaveLen(0))
			})
		})

		Context("when host is unavailable", func() {
			BeforeEach(func() {
				hs.H.UpdateState(db, host.StateStopped)
			})

			It("responds with 503 and no volume is added to db", func() {
				doRequest("sonic-boom")
				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				Expect(mockClient.CreateVolumeCalled).To(Equal(0))
				vols, err := volume.FindAllByHostID(db, hs.H.ID)
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
			v, err := volume.FindByHostIDAndSlug(db, hs.H.ID, "deprecated")
			Expect(err).To(BeNil())
			Expect(v).NotTo(BeNil())
		})

		Context("successful destroy", func() {
			It("removes volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Volume volume.Volume
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				v, err := volume.FindByHostIDAndSlug(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				Expect(v).To(BeNil())

				v, err = volume.FindByHostIDAndSlug(db, hs.H.ID, "conditioner")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(v.Slug).To(Equal("conditioner"))
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
				errType, _ := ErrorDetails(res.Body)
				Expect(errType).To(Equal("internal_server_error"))
				v, err := volume.FindByHostIDAndSlug(db, hs.H.ID, "deprecated")
				Expect(err).To(BeNil())
				Expect(v).NotTo(BeNil())
				Expect(v.Slug).To(Equal("deprecated"))
			})
		})

		Context("when host is unavailable", func() {
			BeforeEach(func() {
				hs.H.UpdateState(db, host.StateStopped)
			})

			It("returns 503 without destroying volume", func() {
				doRequest("deprecated")
				Expect(mockClient.RemoveVolumeCalled).To(Equal(0))
				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				v, err := volume.FindByHostIDAndSlug(db, hs.H.ID, "deprecated")
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

		BeforeEach(func() {
			makeVolumes("music")
			v, err := volume.FindByHostIDAndSlug(db, hs.H.ID, "music")
			Expect(err).To(BeNil())
			Expect(v).NotTo(BeNil())
		})

		Context("successful get", func() {
			BeforeEach(func() {
				mockClient.VolumeInfoReturn = &freighterclient.Volume{
					Slug: "music",
					Size: 666,
				}
			})

			It("returns info", func() {
				doRequest("music")
				Expect(mockClient.VolumeInfoCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				j := struct {
					Vol struct {
						Slug string `json:"slug"`
						Size uint64 `json:"size"`
					} `json:"volume"`
				}{}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Vol.Slug).To(Equal("music"))
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
				errType, _ := ErrorDetails(res.Body)
				Expect(errType).To(Equal("internal_server_error"))
			})
		})
	})
})
