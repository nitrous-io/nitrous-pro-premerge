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
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			Id        string `json:"id"`
			IpAddress string `json:"ip_address"`
		}{
			Provider:  provider,
			Id:        "random_id",
			IpAddress: "0.0.0.0",
		}

		payload, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}

		w.WriteHeader(201)
		w.Write(payload)
	}))
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
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		mockHostbuilder = createMockHostbuilder()
		os.Setenv("HOSTBUILDER_URL", mockHostbuilder.URL)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		os.Setenv("HOSTBUILDER_URL", "")
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
})
