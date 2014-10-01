package hosts_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"

	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func init() {
	// shorten key length so tests run faster
	cert.KeyLen = 512
	keypairs.KeyLen = 512
}

func hostname(slug string) string {
	return fmt.Sprintf("%s.%s", slug, os.Getenv("HOST_HOSTNAME"))
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
}

func mockHostbuilderOK(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
	return &hostbuilder.Response{Status: "ok", IPAddress: "127.0.0.1", FreighterdPort: 65333}, nil
}
func mockHostbuilderError(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
	return &hostbuilder.Response{}, errors.New("some error occurred")
}

var _ = Describe("controllers/hosts", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		u *user.User
		t *token.Token
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		u = &user.User{
			Username: "foo",
			Password: "123456",
			Email:    "foo@example.com",
		}
		err = u.Insert(db)
		Expect(err).To(BeNil())
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/hosts", func() {
		doRequest := func() {
			req, err := NewRequest("GET", "/api/v0/hosts", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when user has hosts", func() {
			var hs1, hs2 *HostStack

			BeforeEach(func() {
				var err error
				hs1, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:           "foo",
						IPAddress:      sql.NullString{"1.2.3.4", true},
						FreighterdPort: sql.NullInt64{64000, true},
						Size:           sql.NullString{"micro", true},
						State:          host.StateRunning,
					},
					U: u,
				})
				Expect(err).To(BeNil())

				hs2, err = CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:           "bar",
						IPAddress:      sql.NullString{"2.4.6.8", true},
						FreighterdPort: sql.NullInt64{64001, true},
						Size:           sql.NullString{"large", true},
						State:          host.StateRunning,
					},
					U: u,
				})
				Expect(err).To(BeNil())
			})

			It("responds with the authenticated user's hosts", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Hosts []*serializers.Host `json:"hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Hosts).To(HaveLen(2))
				Expect(j.Hosts[0].ID).To(Equal(hs1.H.ID))
				Expect(j.Hosts[0].UserID).To(Equal(hs1.U.ID))
				Expect(j.Hosts[0].Slug).To(Equal("foo"))
				Expect(j.Hosts[0].IPAddress).To(Equal("1.2.3.4"))
				Expect(j.Hosts[0].FreighterdPort).To(Equal(int64(64000)))
				Expect(j.Hosts[0].Size).To(Equal("micro"))
				Expect(j.Hosts[0].State).To(Equal(host.StateRunning))
				Expect(j.Hosts[0].Hostname).To(Equal(hostname("foo")))
				Expect(j.Hosts[0].CreatedAt).NotTo(BeNil())

				Expect(j.Hosts[1].ID).To(Equal(hs2.H.ID))
				Expect(j.Hosts[1].UserID).To(Equal(hs2.U.ID))
				Expect(j.Hosts[1].Slug).To(Equal("bar"))
				Expect(j.Hosts[1].IPAddress).To(Equal("2.4.6.8"))
				Expect(j.Hosts[1].FreighterdPort).To(Equal(int64(64001)))
				Expect(j.Hosts[1].Size).To(Equal("large"))
				Expect(j.Hosts[1].State).To(Equal(host.StateRunning))
				Expect(j.Hosts[1].Hostname).To(Equal(hostname("bar")))
				Expect(j.Hosts[1].CreatedAt).NotTo(BeNil())
			})
		})

		Context("when user has no hosts", func() {
			It("responds with an empty array", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j struct {
					Hosts []*serializers.Host `json:"hosts"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Hosts).To(BeEmpty())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/hosts/{slug}", func() {
		var hs *HostStack

		doRequest := func(slug string) {
			req, err := NewRequest("GET", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:           "foo",
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					Size:           sql.NullString{"large", true},
					State:          host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			UnstubFreighterFn()
		})

		It("responds with host info from the database", func() {
			doRequest("foo")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
			Expect(j.Host.UserID).To(Equal(hs.U.ID))
			Expect(j.Host.Slug).To(Equal("foo"))
			Expect(j.Host.IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Host.FreighterdPort).To(Equal(int64(64000)))
			Expect(j.Host.Size).To(Equal("large"))
			Expect(j.Host.State).To(Equal(host.StateRunning))
			Expect(j.Host.Hostname).To(Equal(hostname("foo")))
			Expect(j.Host.RegionID).To(Equal(hs.R.ID))
			Expect(j.Host.PlanID).To(Equal(hs.P.ID))
			Expect(j.Host.CreatedAt).NotTo(BeNil())
			Expect(j.Host.UpdatedAt).NotTo(BeNil())
			Expect(j.Host.TerminatedAt).To(BeNil())
		})

		It("responds with host info even when called with the host ID", func() {
			doRequest(fmt.Sprintf("%d", hs.H.ID))
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Host.ID).To(Equal(hs.H.ID))
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("PUT /api/v0/hosts/{slug}", func() {
		var hs *HostStack

		doRequest := func(params *url.Values) {
			req, err := NewAuthedFormRequest(t, "PUT", "/api/v0/hosts/foo", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:           "foo",
					Name:           sql.NullString{"foohost", true},
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					Size:           sql.NullString{"large", true},
					State:          host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when name field is present and non-empty", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("name", "GhostHost")
				doRequest(params)
			})

			It("updates name", func() {
				updated, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal("GhostHost"))
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *hs.H.UpdatedAt))
			})
		})

		Context("when name field is present and empty", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("name", "")
				doRequest(params)
			})

			It("nullifies name", func() {
				updated, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal(""))
				Expect(updated.Name.Valid).To(BeFalse())
				Expect(*updated.UpdatedAt).To(BeTemporally(">", *hs.H.UpdatedAt))
			})
		})

		Context("when unused fields are given", func() {
			BeforeEach(func() {
				params := &url.Values{}
				params.Set("slug", "younochange")
				doRequest(params)
			})

			It("does not update fields", func() {
				updated, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(updated.Name.String).To(Equal("foohost"))
				Expect(updated.Slug).To(Equal("foo"))
				Expect(updated.UpdatedAt).To(Equal(hs.H.UpdatedAt))
			})
		})

		It("publishes a host updated event with the updated host", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			params := &url.Values{}
			params.Set("name", "GhostHost")
			doRequest(params)

			updated, err := host.FindByID(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(<-ch).To(Equal(&controllers.Event{
				Name: controllers.EventHostUpdated,
				Data: serializers.SerializeHost(updated),
			}))
			close(done)
		}, 3) // add longer timeout to allow goroutine to finish
	})

	Describe("GET /api/v0/hosts/{slug}/stats", func() {
		var (
			mockClient *freighterclient.MockClient
			hs         *HostStack
		)

		doRequest := func(period string) {
			var params *url.Values
			if period != "" {
				params = &url.Values{"period": []string{period}}
			}
			req, err := NewRequest("GET", "/api/v0/hosts/foo/stats", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error

			mockClient = &freighterclient.MockClient{}
			mockClient.HostInfoReturn = &freighterclient.HostInfo{
				Uptime:       30,
				MemoryUsage:  12345678,
				MemoryTotal:  17179869184,
				StorageUsage: 2000000000,
				StorageTotal: 1000000000000,
				NumCPU:       2,
				PerCPUUsage:  []float64{0.5, 30},
			}
			StubDefaultFreighterFn(mockClient)

			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:      "foo",
					IPAddress: sql.NullString{"1.2.3.4", true},
					State:     host.StateRunning,
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			UnstubFreighterFn()
		})

		It("calls the Freighter client to get host info", func() {
			doRequest("")
			Expect(mockClient.HostInfoCalled).To(Equal(1))
			Expect(mockClient.HostInfoPeriod).To(Equal(100 * time.Millisecond)) // Default is 100ms if not specified.

			doRequest("2s")
			Expect(mockClient.HostInfoCalled).To(Equal(2))
			Expect(mockClient.HostInfoPeriod).To(Equal(2 * time.Second))
		})

		It("responds with host stats from the Freighter API", func() {
			doRequest("")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Stats *serializers.HostInfo `json:"stats"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Stats.Uptime).To(Equal(uint64(30)))
			Expect(j.Stats.MemoryUsage).To(Equal(uint64(12345678)))
			Expect(j.Stats.MemoryTotal).To(Equal(uint64(17179869184)))
			Expect(j.Stats.StorageUsage).To(Equal(uint64(2000000000)))
			Expect(j.Stats.StorageTotal).To(Equal(uint64(1000000000000)))
			Expect(j.Stats.NumCPU).To(Equal(2))
			Expect(j.Stats.PerCPUUsage).To(Equal([]float64{0.5, 30}))
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("when host does not have an IP address", func() {
			BeforeEach(func() {
				hs.H.IPAddress = sql.NullString{}
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/stats"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}", func() {
		var (
			hs *HostStack
			c  *container.Container
		)

		doRequest := func(slug string) {
			req, err := NewRequest("DELETE", "/api/v0/hosts/"+slug, nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{Slug: "honeybadger", ProviderIdentifier: sql.NullString{"001", true}},
				U: u,
			})
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "beefface",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Slug:   "beef-jerky",
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when hostbuilder successfully deletes the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb
			})

			It("deletes the host record", func() {
				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})

			It("deletes all container records belonging to the host", func() {
				c2, err := container.Find(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(Equal(c))

				doRequest(hs.H.Slug)

				c2, err = container.Find(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(BeNil())
			})

			It("deletes keypairs associated to the host", func() {
				priv, _ := keypairs.GenerateKey()
				tx, _ := db.Beginx()
				keypairs.Create(tx, priv, "aes_key", hs.H.ID)
				tx.Commit()

				doRequest(hs.H.Slug)

				_, err := keypairs.Get(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "host's keypairs should be removed")
			})

			It("deletes certs associated to the host", func() {
				tx, _ := db.Beginx()
				crt := &cert.Cert{
					ServerCert: "servercert",
					ServerKey:  "serverkey",
				}
				Expect(crt.Insert(tx, hs.H.ID)).To(BeNil())
				tx.Commit()

				doRequest(hs.H.Slug)
				_, err := cert.Find(db, hs.H.ID)
				Expect(err).ToNot(BeNil(), "failed to remove host's certificates")
			})

			It("responds with status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal("honeybadger"))
				Expect(j.Host.IPAddress).To(Equal(""))
				Expect(j.Host.FreighterdPort).To(Equal(int64(0)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateTerminated))
				Expect(j.Host.Hostname).To(Equal(fmt.Sprintf("honeybadger.%s", os.Getenv("HOST_HOSTNAME"))))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).NotTo(BeNil())
			})

			It("publishes a host destroyed event with the destroyed host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				deleted, err := host.FindByID(db, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: controllers.EventHostDestroyed,
					Data: serializers.SerializeHost(deleted),
				}))
				close(done)
			}, 3) // add longer timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: errorResponse,
					status:  http.StatusBadRequest,
				})
				hosts.HostbuilderClient = hb
			})

			It("does not delete the host", func() {
				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("when host is provider identifier is nil", func() {
			It("should not make a request to hostbuilder", func() {
				hs.H.ProviderIdentifier = sql.NullString{String: "", Valid: false}
				hs.H.Update(db)

				// in this context, hostbuilder should not be called
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt := &FakeRoundTripper{
					message: errorResponse,
					status:  http.StatusBadRequest,
				}
				replaceTransport(hb, rt)
				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
				Expect(len(rt.requests)).To(Equal(0), "expect to not called hostbuilder")
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/start", func() {
		var hs *HostStack

		doRequest := func(slug string) {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/start", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{State: host.StateStopped},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is not in the stopped state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateRestarting
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not stopped"))
			})
		})

		Context("when hostbuilder successfully starts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb
			})

			It("updates the host state to running", func() {
				doRequest(hs.H.Slug)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host started event with the host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: controllers.EventHostStarted,
					Data: serializers.SerializeHost(h),
				}))
				close(done)
			}, 3) // add longer timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: errorResponse,
					status:  http.StatusBadRequest,
				})
				hosts.HostbuilderClient = hb
				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStarting))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/start"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/stop", func() {
		var hs *HostStack

		doRequest := func(slug string, params *url.Values) {
			req, err := NewFormRequest("POST", "/api/v0/hosts/"+slug+"/stop", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					State:     host.StateRunning,
					IPAddress: sql.NullString{"127.0.0.1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is already stopped", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug, nil)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("when hostbuilder successfully stops the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb
			})

			It("updates the host state to stopped", func() {
				doRequest(hs.H.Slug, nil)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopped))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateStopped))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host stopped event with the host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, nil)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: controllers.EventHostStopped,
					Data: serializers.SerializeHost(h),
				}))
				close(done)
			}, 3) // add longer timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: errorResponse,
					status:  http.StatusBadRequest,
				})

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug, nil)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopping))
			})
		})

		Context("when host has running containers", func() {
			var (
				c1, c2     *container.Container
				mockClient *freighterclient.MockClient
			)

			BeforeEach(func() {
				mockClient = &freighterclient.MockClient{}
				StubDefaultFreighterFn(mockClient)

				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
				mockClient.Containers = []*freighterclient.Container{
					&freighterclient.Container{ID: "beefface"},
					&freighterclient.Container{ID: "cafedead"},
				}
			})

			It("calls stop on all containers", func() {
				doRequest(hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.StopCalled).To(Equal(2))
			})

			It("returns error if stop failed", func() {
				mockClient.StopError = errors.New("can't stop won't stop")
				doRequest(hs.H.Slug, nil)
				Expect(mockClient.StopCalled).To(Equal(2))
				_, errDesc := ErrorSlug(res.Body)
				Expect(errDesc).To(Equal("unable to stop containers rails-box, python-box"))
			})

			It("ignores stop failures if ignore-containers param is true", func() {
				mockClient.StopError = errors.New("can't stop won't stop")
				params := &url.Values{}
				params.Set("ignore-containers", "true")
				doRequest(hs.H.Slug, params)
				Expect(mockClient.StopCalled).To(Equal(2))
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			checkStoppedMsg := func(msg interface{}, slug string) {
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventContainerStopped))
				cn, ok := evt.Data.(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.Slug).To(Equal(slug))
			}

			It("publishes container stopped events", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, nil)

				msg := <-ch
				checkStoppedMsg(msg, c1.Slug)
				msg = <-ch
				checkStoppedMsg(msg, c2.Slug)

				close(done)
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/stop"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/restart", func() {
		var hs *HostStack

		doRequest := func(slug string, params *url.Values) {
			req, err := NewFormRequest("POST", "/api/v0/hosts/"+slug+"/restart", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					State:     host.StateRunning,
					IPAddress: sql.NullString{"127.0.0.1", true},
				},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is not in the running state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb

				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				doRequest(hs.H.Slug, nil)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("when hostbuilder successfully restarts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})

				hosts.HostbuilderClient = hb
			})

			It("updates the host state to running", func() {
				doRequest(hs.H.Slug, nil)
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with the host and status OK", func() {
				doRequest(hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Host *serializers.Host `json:"host"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.ID).To(Equal(hs.H.ID))
				Expect(j.Host.Slug).To(Equal(hs.H.Slug))
				Expect(j.Host.IPAddress).To(Equal("127.0.0.1"))
				Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
				Expect(j.Host.Size).To(Equal(""))
				Expect(j.Host.State).To(Equal(host.StateRunning))
				Expect(j.Host.Hostname).To(Equal(hs.H.Hostname()))
				Expect(j.Host.RegionID).To(Equal(hs.R.ID))
				Expect(j.Host.PlanID).To(Equal(hs.P.ID))
				Expect(j.Host.CreatedAt).NotTo(BeNil())
				Expect(j.Host.UpdatedAt).NotTo(BeNil())
				Expect(j.Host.TerminatedAt).To(BeNil())
			})

			It("publishes a host restarted event with the host", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, nil)

				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(<-ch).To(Equal(&controllers.Event{
					Name: controllers.EventHostRestarted,
					Data: serializers.SerializeHost(h),
				}))
				close(done)
			}, 3) // increase timeout to allow goroutine to finish
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: errorResponse,
					status:  http.StatusBadRequest,
				})

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug, nil)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRestarting))
			})
		})

		Context("when host has running containers", func() {
			var (
				c1, c2     *container.Container
				mockClient *freighterclient.MockClient
			)

			BeforeEach(func() {
				mockClient = &freighterclient.MockClient{}
				StubDefaultFreighterFn(mockClient)

				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: okResponse,
					status:  http.StatusOK,
				})
				hosts.HostbuilderClient = hb

				c1 = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c1.Insert(db)
				Expect(err).To(BeNil())
				c2 = &container.Container{
					CID:    "cafedead",
					Slug:   "python-box",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err = c2.Insert(db)
				Expect(err).To(BeNil())
				mockClient.Containers = []*freighterclient.Container{
					&freighterclient.Container{ID: "beefface"},
					&freighterclient.Container{ID: "cafedead"},
				}
			})

			It("calls stop on all containers", func() {
				doRequest(hs.H.Slug, nil)
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(mockClient.StopCalled).To(Equal(2))
			})

			It("returns error if stop failed", func() {
				mockClient.StopError = errors.New("can't stop won't stop")
				doRequest(hs.H.Slug, nil)
				Expect(mockClient.StopCalled).To(Equal(2))
				_, errDesc := ErrorSlug(res.Body)
				Expect(errDesc).To(Equal("unable to stop containers rails-box, python-box"))
			})

			It("ignores stop failures if ignore-containers param is true", func() {
				mockClient.StopError = errors.New("can't stop won't stop")
				params := &url.Values{}
				params.Set("ignore-containers", "true")
				doRequest(hs.H.Slug, params)
				Expect(mockClient.StopCalled).To(Equal(2))
				Expect(res.Code).To(Equal(http.StatusOK))
			})

			checkStoppedMsg := func(msg interface{}, slug string) {
				evt, ok := msg.(*controllers.Event)
				Expect(ok).To(BeTrue())
				Expect(evt.Name).To(Equal(controllers.EventContainerStopped))
				cn, ok := evt.Data.(*serializers.Container)
				Expect(ok).To(BeTrue())
				Expect(cn.Slug).To(Equal(slug))
			}

			It("publishes container stopped events", func(done Done) {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(hs.H.Slug, nil)

				msg := <-ch
				checkStoppedMsg(msg, c1.Slug)
				msg = <-ch
				checkStoppedMsg(msg, c2.Slug)

				close(done)
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/restart"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})

var (
	okResponse    = `{"freighterd_port":65333,"ip_address":"127.0.0.1"}`
	errorResponse = `{"error":"some error occurred"}`
)

// replace the transport in hostbuilder client with a new fake transport
func replaceTransport(hb *hostbuilderclient.Hostbuilder, t http.RoundTripper) {
	hb.Client.Client.Transport = t
}

// FakeRoundTripper implements a fake Transport layer that records the executed
// requests and provide a fake response
//
// This is more lightweight than implementing a full HTTP server and should be
// approriate for some tests
type FakeRoundTripper struct {
	message  string
	status   int
	header   map[string]string
	requests []*http.Request
}

func (rt *FakeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	body := strings.NewReader(rt.message)
	rt.requests = append(rt.requests, r)
	res := &http.Response{
		StatusCode: rt.status,
		Body:       ioutil.NopCloser(body),
		Header:     make(http.Header),
	}
	for k, v := range rt.header {
		res.Header.Set(k, v)
	}
	return res, nil
}

func (rt *FakeRoundTripper) Reset() {
	rt.requests = nil
}
