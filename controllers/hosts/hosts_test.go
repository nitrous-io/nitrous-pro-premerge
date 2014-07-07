package hosts_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/config"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/server"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/hosts")
}

func createMockHostbuilder() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/hosts", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()

		mustFail := os.Getenv("HOSTBUILDER_MUST_FAIL") // is there a better way?
		if mustFail != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("some_error"))
			return
		}

		provider := r.FormValue("provider")

		m := struct {
			Provider  string `json:"provider"`
			ID        string `json:"id"`
			IPAddress string `json:"ip_address"`
		}{
			Provider:  provider,
			ID:        "random_id",
			IPAddress: "0.0.0.0",
		}

		payload, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	})

	mux.HandleFunc("/hosts/delete", func(w http.ResponseWriter, r *http.Request) {
		mustFail := os.Getenv("HOSTBUILDER_MUST_FAIL") // is there a better way?
		if mustFail != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("some_error"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/hosts/start", func(w http.ResponseWriter, r *http.Request) {
		mustFail := os.Getenv("HOSTBUILDER_MUST_FAIL") // is there a better way?
		if mustFail != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("some_error"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/hosts/stop", func(w http.ResponseWriter, r *http.Request) {
		mustFail := os.Getenv("HOSTBUILDER_MUST_FAIL") // is there a better way?
		if mustFail != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("some_error"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/hosts/restart", func(w http.ResponseWriter, r *http.Request) {
		mustFail := os.Getenv("HOSTBUILDER_MUST_FAIL") // is there a better way?
		if mustFail != "" {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("some_error"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	return httptest.NewServer(mux)
}

var _ = Describe("controllers/hosts", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		params          *url.Values
		authTokenInputs AuthTokenInputs
		mockHostbuilder *httptest.Server
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		mockHostbuilder = createMockHostbuilder()
		env := &config.Env{HostbuilderURL: mockHostbuilder.URL}
		svr = server.NewServer("", db, logger, env)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		mockHostbuilder.Close()
	})

	Describe("GET /api/v0/hosts", func() {
		var (
			u *user.User
			t *token.Token
		)

		doRequest := func(token string) {
			req, err := NewRequest("GET", "/api/v0/hosts", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when user has hosts", func() {
				BeforeEach(func() {
					var err error

					r := &region.Region{Name: "apse1", Description: "Southeast Asia"}
					err = r.Insert(db)
					Expect(err).To(BeNil())

					p := &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
					err = p.Insert(db)
					Expect(err).To(BeNil())

					pt := &providertemplate.ProviderTemplate{
						RegionID: r.ID,
						PlanID:   p.ID,
						Name:     "digital-ocean-1",
						Provider: "digitalocean",
					}
					err = pt.Insert(db)
					Expect(err).To(BeNil())

					h1 := &host.Host{
						Name:               "foo",
						UserID:             u.ID,
						RegionID:           r.ID,
						PlanID:             p.ID,
						ProviderTemplateID: pt.ID,
					}
					err = h1.Insert(db)
					Expect(err).To(BeNil())
					h2 := &host.Host{
						Name:               "bar",
						UserID:             u.ID,
						RegionID:           r.ID,
						PlanID:             p.ID,
						ProviderTemplateID: pt.ID,
					}
					err = h2.Insert(db)
					Expect(err).To(BeNil())
				})

				It("responds with the authenticated user's hosts", func() {
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusOK))
					var j map[string][]host.Host
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					hosts, ok := j["hosts"]
					Expect(ok).To(BeTrue())
					Expect(hosts).To(HaveLen(2))

					Expect(hosts[0].Name).To(Equal("foo"))
					Expect(hosts[0].UserID).To(Equal(u.ID))

					Expect(hosts[1].Name).To(Equal("bar"))
					Expect(hosts[1].UserID).To(Equal(u.ID))
				})
			})

			Context("when user has no hosts", func() {
				It("responds with an empty array", func() {
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusOK))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					hosts, ok := j["hosts"].([]interface{})
					Expect(ok).To(BeTrue())
					Expect(hosts).To(BeEmpty())
				})
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
		var (
			u  *user.User
			t  *token.Token
			r1 *region.Region
			r2 *region.Region
			p1 *plan.Plan
			p2 *plan.Plan
			pt *providertemplate.ProviderTemplate
		)

		doRequest := func(token string) {
			req, err := NewRequest("POST", "/api/v0/hosts", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			r1 = &region.Region{Name: "apse1", Description: "Southeast Asia"}
			err = r1.Insert(db)
			Expect(err).To(BeNil())

			r2 = &region.Region{Name: "apse2", Description: "Southeast Asia"}
			err = r2.Insert(db)
			Expect(err).To(BeNil())

			p1 = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
			err = p1.Insert(db)
			Expect(err).To(BeNil())

			p2 = &plan.Plan{Name: "small", MaxContainers: 3, Price: 1500}
			err = p2.Insert(db)
			Expect(err).To(BeNil())

			pt = &providertemplate.ProviderTemplate{
				RegionID: r1.ID,
				PlanID:   p1.ID,
				Name:     "digital-ocean-1",
				Provider: "digitalocean",
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when request does not specify a size", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("size", "")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when a matching plan cannot be found for the specified size", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("size", "doubledouble")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when an invalid region is specified", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("size", "doubledouble")
					params.Set("region", "antarctica")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when no provider template found for the given plan and region", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("size", "small")
					params.Set("region", "apse2")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when hostbuilder successfully creates the host", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("name", "honeybadger")
					params.Set("size", "micro")
					params.Set("region", "apse1")
					doRequest("tokenblackguy")
				})

				It("creates a host record for the user", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(1))

					h := hosts[0]
					Expect(h.RegionID).To(Equal(r1.ID))
					Expect(h.PlanID).To(Equal(p1.ID))
					Expect(h.ProviderTemplateID).To(Equal(pt.ID))

					Expect(h.Name).To(Equal("honeybadger"))
					Expect(h.LongName.String).To(Equal("honeybadger"))

					Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
					Expect(h.State).To(Equal("created"))
					Expect(h.MaxContainers.Int64).To(Equal(p1.MaxContainers))
					Expect(h.ProviderIdentifier.String).To(Equal("random_id"))
				})

				It("responds with the created host", func() {
					Expect(res.Code).To(Equal(http.StatusCreated))

					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["host"]).NotTo(BeNil())
				})
			})

			Context("when hostbuilder fails to create the host", func() {
				BeforeEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "true")
					params = &url.Values{}
					params.Set("name", "honeybadger")
					params.Set("size", "micro")
					params.Set("region", "apse1")
					doRequest("tokenblackguy")
				})

				AfterEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "")
				})

				It("does not create a host for the user", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(0))
				})

				It("responds with 503 service unavailable", func() {
					Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
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

	Describe("DELETE /api/v0/hosts", func() {
		var (
			u  *user.User
			t  *token.Token
			r  *region.Region
			p  *plan.Plan
			pt *providertemplate.ProviderTemplate
		)

		doRequest := func(token string) {
			req, err := NewRequest("DELETE", "/api/v0/hosts", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
			err = r.Insert(db)
			Expect(err).To(BeNil())

			p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
			err = p.Insert(db)
			Expect(err).To(BeNil())

			pt = &providertemplate.ProviderTemplate{
				RegionID: r.ID,
				PlanID:   p.ID,
				Name:     "digital-ocean-1",
				Provider: "digitalocean",
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())

			h := &host.Host{
				Name:               "honeybadger",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when request does not specify a name", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when an invalid host name given", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "dreamhost")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when hostbuilder successfully deletes the host", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				It("deletes the host record", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(0))
				})

				It("responds with 200 OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to delete the host", func() {
				BeforeEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "true")
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				AfterEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "")
				})

				It("does not delete the host record", func() {
					hosts, err := host.FindAllByUser(db, u)
					Expect(err).To(BeNil())
					Expect(len(hosts)).To(Equal(1))
				})

				It("responds with 503 service unavailable", func() {
					Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts"
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/start", func() {
		var (
			u  *user.User
			t  *token.Token
			r  *region.Region
			p  *plan.Plan
			pt *providertemplate.ProviderTemplate
		)

		doRequest := func(token string) {
			req, err := NewRequest("POST", "/api/v0/hosts/start", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
			err = r.Insert(db)
			Expect(err).To(BeNil())

			p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
			err = p.Insert(db)
			Expect(err).To(BeNil())

			pt = &providertemplate.ProviderTemplate{
				RegionID: r.ID,
				PlanID:   p.ID,
				Name:     "digital-ocean-1",
				Provider: "digitalocean",
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())

			h := &host.Host{
				Name:               "honeybadger",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
				State:              "stopped",
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when request does not specify a name", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when an invalid host name given", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "dreamhost")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when hostbuilder successfully starts the host", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				It("sets the host record's state to running", func() {
					h, err := host.FindByUserAndName(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("running"))
				})

				It("responds with 200 OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to start the host", func() {
				BeforeEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "true")
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				AfterEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "")
				})

				It("does not change state of the host record", func() {
					h, err := host.FindByUserAndName(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("stopped"))
				})

				It("responds with 503 service unavailable", func() {
					Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/start"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/stop", func() {
		var (
			u  *user.User
			t  *token.Token
			r  *region.Region
			p  *plan.Plan
			pt *providertemplate.ProviderTemplate
		)

		doRequest := func(token string) {
			req, err := NewRequest("POST", "/api/v0/hosts/stop", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
			err = r.Insert(db)
			Expect(err).To(BeNil())

			p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
			err = p.Insert(db)
			Expect(err).To(BeNil())

			pt = &providertemplate.ProviderTemplate{
				RegionID: r.ID,
				PlanID:   p.ID,
				Name:     "digital-ocean-1",
				Provider: "digitalocean",
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())

			h := &host.Host{
				Name:               "honeybadger",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
				State:              "running",
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when request does not specify a name", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when an invalid host name given", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "dreamhost")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when hostbuilder successfully stops the host", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				It("sets the host record state to stopped", func() {
					h, err := host.FindByUserAndName(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("stopped"))
				})

				It("responds with 200 OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to stop the host", func() {
				BeforeEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "true")
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				AfterEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "")
				})

				It("does not change state of the host record", func() {
					h, err := host.FindByUserAndName(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("running"))
				})

				It("responds with 503 service unavailable", func() {
					Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/stop"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/restart", func() {
		var (
			u  *user.User
			t  *token.Token
			r  *region.Region
			p  *plan.Plan
			pt *providertemplate.ProviderTemplate
		)

		doRequest := func(token string) {
			req, err := NewRequest("POST", "/api/v0/hosts/restart", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t = &token.Token{
				UserID: u.ID,
				Token:  "tokenblackguy",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
			err = r.Insert(db)
			Expect(err).To(BeNil())

			p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
			err = p.Insert(db)
			Expect(err).To(BeNil())

			pt = &providertemplate.ProviderTemplate{
				RegionID: r.ID,
				PlanID:   p.ID,
				Name:     "digital-ocean-1",
				Provider: "digitalocean",
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())

			h := &host.Host{
				Name:               "honeybadger",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
				State:              "somestate",
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			Context("when request does not specify a name", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when an invalid host name given", func() {
				It("responds with 400 bad request", func() {
					params = &url.Values{}
					params.Set("name", "dreamhost")
					doRequest("tokenblackguy")

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
				})
			})

			Context("when hostbuilder successfully restarts the host", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				It("sets the host record state to running", func() {
					h, err := host.FindByUserAndName(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("running"))
				})

				It("responds with 200 OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when hostbuilder fails to restart the host", func() {
				BeforeEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "true")
					params = &url.Values{}
					params.Set("name", "honeybadger")
					doRequest("tokenblackguy")
				})

				AfterEach(func() {
					os.Setenv("HOSTBUILDER_MUST_FAIL", "")
				})

				It("does not change state of the host record", func() {
					h, err := host.FindByUserAndName(db, u, "honeybadger")
					Expect(err).To(BeNil())
					Expect(h.State).To(Equal("somestate"))
				})

				It("responds with 503 service unavailable", func() {
					Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/restart"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
