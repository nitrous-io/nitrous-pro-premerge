package hosts_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/server"

	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
}

func mockHostbuilderOK(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
	return &hostbuilder.Response{Status: "ok"}, nil
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

		u  *user.User
		t  *token.Token
		hs *HostStack
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
			BeforeEach(func() {
				hs, err := CreateHostStack(db, &HostStack{
					H: &host.Host{Slug: "foo"},
					R: &region.Region{Name: "use"},
					P: &plan.Plan{Name: "large"},
					U: u,
				})
				Expect(err).To(BeNil())

				_, err = CreateHostStack(db, &HostStack{
					H:  &host.Host{Slug: "bar"},
					U:  u,
					R:  hs.R,
					P:  hs.P,
					PT: hs.PT,
				})
				Expect(err).To(BeNil())
			})

			It("responds with the authenticated user's hosts", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				var j map[string][]host.Host
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				hosts, ok := j["hosts"]
				Expect(ok).To(BeTrue())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].Slug).To(Equal("foo"))
				Expect(hosts[0].UserID).To(Equal(u.ID))

				Expect(hosts[1].Slug).To(Equal("bar"))
				Expect(hosts[1].UserID).To(Equal(u.ID))
			})
		})

		Context("when user has no hosts", func() {
			It("responds with an empty array", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				hosts, ok := j["hosts"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(hosts).To(BeEmpty())
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

	Describe("POST /api/v0/hosts", func() {
		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				R: &region.Region{Name: "use"},
				P: &plan.Plan{Name: "large"},
			})
			Expect(err).To(BeNil())
		})

		type overrides map[string]string
		doRequest := func(o overrides) {
			var ok bool

			if _, ok = o["slug"]; !ok {
				o["slug"] = "honeybadger"
			}

			if _, ok = o["size"]; !ok {
				o["size"] = hs.P.Name
			}

			if _, ok = o["region"]; !ok {
				o["region"] = hs.R.Name
			}

			if _, ok = o["name"]; !ok {
				o["name"] = "honeybadger don't care"
			}

			p := &url.Values{}
			for k, v := range o {
				p.Set(k, v)
			}

			req, err := NewRequest("POST", "/api/v0/hosts", p)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		// tests for required params
		for _, param := range []string{"size", "region", "slug"} {
			p := param

			Context("when request does not specify a "+p, func() {
				It("responds with "+p+" required", func() {
					doRequest(overrides{p: ""})

					Expect(res.Code).To(Equal(http.StatusBadRequest))

					errType, errDesc := ErrorDetails(res.Body)
					Expect(errType).To(Equal("invalid_request"))
					Expect(errDesc).To(Equal(p + " is required"))
				})
			})
		}

		Context("when a matching plan cannot be found for the specified size", func() {
			It("responds with plan not found", func() {
				doRequest(overrides{"size": "doubledouble"})

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("no matching plan found for the given size"))
			})
		})

		Context("when an invalid region is specified", func() {
			It("responds with region not found", func() {
				doRequest(overrides{"region": "antarctica"})

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("no matching region found for the given name"))
			})
		})

		Context("when no provider template found for the given plan and region", func() {
			It("responds with provider template not found error", func() {
				var r2 *region.Region

				r2 = &region.Region{Name: "usw", Description: "US West"}
				err := r2.Insert(db)
				Expect(err).To(BeNil())

				doRequest(overrides{"region": r2.Name})

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("no matching templates found for the given plan and regions combination"))
			})
		})

		Context("with valid parameters", func() {
			Context("when hostbuilder successfully creates a host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					ch := func(path string, provider string, config *hostbuilderclient.Config) (*hostbuilder.Response, error) {
						return &hostbuilder.Response{
							ID:             "abcd1234",
							IPAddress:      "0.0.0.0",
							Provider:       hs.PT.Name,
							FreighterdPort: uint16(65333),
						}, nil
					}
					hb.OverrideRequestHandler(ch)

					hosts.HostbuilderClient = hb

					doRequest(overrides{})
				})

				It("creates a host record for the user", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(1))

					h := hosts[0]
					Expect(h.RegionID).To(Equal(hs.R.ID))
					Expect(h.PlanID).To(Equal(hs.P.ID))
					Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

					Expect(h.Slug).To(Equal("honeybadger"))
					Expect(h.Name.String).To(Equal("honeybadger don't care"))

					Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
					Expect(h.State).To(Equal(host.StateRunning))
					Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
					Expect(h.Size.String).To(Equal(hs.P.Name))
					Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
					Expect(h.FreighterdPort.Int64).To(Equal(int64(65333)))
				})

				It("creates a keypair associated with that host", func() {
					hosts, _ := host.FindAllByUser(db, u)
					kp, err := keypairs.Get(db, hosts[0].ID)
					Expect(err).To(BeNil())
					Expect(kp.HostID).To(Equal(hosts[0].ID))
				})

				It("responds with the created host and status created", func() {
					Expect(res.Code).To(Equal(http.StatusCreated))

					var j struct {
						Status string                 `json:"status"`
						Host   map[string]interface{} `json:"host"`
					}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())

					Expect(j.Status).To(Equal("created"))
					Expect(j.Host["slug"].(string)).To(Equal("honeybadger"))
				})
			})

			Context("when hostbuilder fails to create the host", func() {
				BeforeEach(func() {
					hb := hostbuilderclient.NewHostbuilder("www.example.com")
					hb.OverrideRequestHandler(mockHostbuilderError)

					hosts.HostbuilderClient = hb

					doRequest(overrides{})
				})

				It("does not create a host for the user", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(0))
				})

				It("responds with a internal server error", func() {
					Expect(res.Code).To(Equal(http.StatusInternalServerError))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /api/v0/hosts/{slug}", func() {
		var (
			mockClient *freighterclient.MockClient
			hs         *HostStack
		)

		type expectedResp struct {
			Host struct {
				ID             int64  `json:"id"`
				Slug           string `json:"slug"`
				IPAddress      string `json:"ip_address"`
				FreighterdPort int64  `json:"freighterd_port"`
				State          string `json:"state"`
				Hostname       string `json:"hostname"`
				Region         *struct {
					ID          int64  `json:"id"`
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"region"`
				Plan *struct {
					ID   int64  `json:"id"`
					Name string `json:"name"`
				} `json:"plan"`
				Stats *struct {
					Uptime       uint64    `json:"uptime"`
					MemoryUsage  uint64    `json:"memory_usage"`
					MemoryTotal  uint64    `json:"memory_total"`
					StorageUsage uint64    `json:"storage_usage"`
					StorageTotal uint64    `json:"storage_total"`
					NumCPU       int       `json:"num_cpu"`
					PerCPUUsage  []float64 `json:"per_cpu_usage"`
				} `json:"stats"`
			} `json:"host"`
		}

		doRequest := func(slug string) {
			req, err := NewRequest("GET", "/api/v0/hosts/"+slug, nil)
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
			hosts.FreighterClient = mockClient

			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:           "foo",
					IPAddress:      sql.NullString{"1.2.3.4", true},
					FreighterdPort: sql.NullInt64{64000, true},
					State:          host.StateRunning,
				},
				R: &region.Region{Name: "use", Description: "US East"},
				P: &plan.Plan{Name: "large"},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		It("calls the Freighter client to get host info", func() {
			doRequest("foo")
			Expect(mockClient.HostInfoCalled).To(Equal(1))
		})

		It("responds with host info from the database and augments it with stats from the Freighter API", func() {
			doRequest("foo")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j expectedResp
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Host.ID).To(Equal(hs.H.ID))
			Expect(j.Host.Slug).To(Equal("foo"))
			Expect(j.Host.IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Host.FreighterdPort).To(Equal(int64(64000)))
			Expect(j.Host.State).To(Equal(host.StateRunning))
			Expect(j.Host.Hostname).To(Equal("foo.nitrouspro.com"))

			Expect(j.Host.Region.ID).To(Equal(hs.R.ID))
			Expect(j.Host.Region.Name).To(Equal("use"))
			Expect(j.Host.Region.Description).To(Equal("US East"))

			Expect(j.Host.Plan.ID).To(Equal(hs.P.ID))
			Expect(j.Host.Plan.Name).To(Equal("large"))

			Expect(j.Host.Stats.Uptime).To(Equal(uint64(30)))
			Expect(j.Host.Stats.MemoryUsage).To(Equal(uint64(12345678)))
			Expect(j.Host.Stats.MemoryTotal).To(Equal(uint64(17179869184)))
			Expect(j.Host.Stats.StorageUsage).To(Equal(uint64(2000000000)))
			Expect(j.Host.Stats.StorageTotal).To(Equal(uint64(1000000000000)))
			Expect(j.Host.Stats.NumCPU).To(Equal(2))
			Expect(j.Host.Stats.PerCPUUsage).To(Equal([]float64{0.5, 30}))
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.UpdateState(db, host.StateStopped)
			})

			It("excludes host stats from Freighter", func() {
				doRequest("foo")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j expectedResp
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.Slug).To(Equal("foo"))
				Expect(j.Host.Stats).To(BeNil())
			})
		})

		Context("when host does not have an IP address", func() {
			BeforeEach(func() {
				_, err := CreateHostStack(db, &HostStack{
					H: &host.Host{
						Slug:      "bar",
						IPAddress: sql.NullString{"", false},
					},
					U: u,
				})
				Expect(err).To(BeNil())
			})

			It("excludes host stats from Freighter", func() {
				doRequest("bar")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j expectedResp
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Host.Slug).To(Equal("bar"))
				Expect(j.Host.Stats).To(BeNil())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}", func() {
		var c *container.Container

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
				hb.OverrideRequestHandler(mockHostbuilderOK)
				hosts.HostbuilderClient = hb
			})

			It("deletes the host record", func() {
				doRequest(hs.H.Slug)

				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})

			It("deletes all container records belonging to the host", func() {
				c2, err := container.FindByUserAndSlug(db, hs.U, c.Slug)
				Expect(err).To(BeNil())
				Expect(c2).To(Equal(c))

				doRequest(hs.H.Slug)

				c2, err = container.FindByUserAndSlug(db, hs.U, c.Slug)
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

			It("responds with status OK", func() {
				doRequest(hs.H.Slug)
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when hostbuilder fails to delete the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("does not delete the host", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
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
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.UpdateState(db, host.StateRestarting)
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not stopped"))
			})
		})

		Context("when hostbuilder successfully starts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("updates the host state to running", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with status OK", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when hostbuilder fails to start the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
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
		doRequest := func(slug string) {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/stop", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{State: host.StateRunning},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is already stopped", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.UpdateState(db, host.StateStopped)
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("when hostbuilder successfully stops the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("updates the host state to stopped", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopped))
			})

			It("responds with HTTP 200 OK", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when hostbuilder fails to stop the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateStopping))
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

	Describe("Restart", func() {
		doRequest := func(slug string) {
			req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/restart", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			var err error
			hs, err = CreateHostStack(db, &HostStack{
				H: &host.Host{State: host.StateRunning},
				U: u,
			})
			Expect(err).To(BeNil())
		})

		Context("when host is not in the running state", func() {
			It("responds with HTTP 400 Bad Request", func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				hs.H.UpdateState(db, host.StateStopped)
				doRequest(hs.H.Slug)

				Expect(res.Code).To(Equal(http.StatusBadRequest))

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_state"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("when hostbuilder successfully restarts the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderOK)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("updates the host state to running", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRunning))
			})

			It("responds with status OK", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when hostbuilder fails to restart the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				hb.OverrideRequestHandler(mockHostbuilderError)

				hosts.HostbuilderClient = hb

				doRequest(hs.H.Slug)
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})

			It("leaves host in intermediate state", func() {
				h, err := host.FindByUserAndSlug(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h.State).To(Equal(host.StateRestarting))
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
