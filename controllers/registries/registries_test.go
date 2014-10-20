package registries_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"

	"github.com/nitrous-io/nitrous/models/registry"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/registries")
}

var _ = Describe("controllers/registries", func() {
	var (
		db              *ndb.Database
		handler         *mux.Router
		svr             *server.Server
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		u  *user.User
		t  *token.Token
		hs *HostStack

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	type errorResponse struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	type overrides map[string]string

	unmarshalErrorResponse := func() (error, *errorResponse) {
		errRes := &errorResponse{}
		err := json.Unmarshal(res.Body.Bytes(), errRes)
		return err, errRes
	}

	ItRespondsHostNotFound := func(doRequest func(overrides)) {
		It("responds host not found error", func() {
			doRequest(overrides{"slug": "nanana-8928"})

			Expect(res.Code).To(Equal(http.StatusNotFound))

			err, j := unmarshalErrorResponse()
			Expect(err).To(BeNil())

			Expect(j.Error).To(Equal("host_not_found"))
			Expect(j.ErrorDescription).To(Equal("host does not exist"))
		})
	}

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

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
				Slug:      "foo",
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

	Describe("GET /api/v0/hosts/{slug}/registries", func() {
		doRequest := func(o overrides) {
			slug := "foo"
			if _, ok := o["slug"]; ok {
				slug = o["slug"]
				delete(o, "slug")
			}

			req, err := NewRequest("GET", "/api/v0/hosts/"+slug+"/registries", nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("when user has logged-in to registries", func() {
			BeforeEach(func() {
				r1 := &registry.Registry{
					UserID:   hs.U.ID,
					HostID:   hs.H.ID,
					URL:      "hub1.example.com",
					Username: "foo",
					Email:    "foo@example.com",
				}
				err := r1.Insert(db)
				Expect(err).To(BeNil())

				r2 := &registry.Registry{
					UserID:   hs.U.ID,
					HostID:   hs.H.ID,
					URL:      "hub2.example.com",
					Username: "foo",
					Email:    "foo@example.com",
				}
				err = r2.Insert(db)
				Expect(err).To(BeNil())
			})

			It("responds with the registries user has logged in", func() {
				doRequest(overrides{})

				Expect(res.Code).To(Equal(http.StatusOK))
				var j map[string][]serializers.Registry
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				registries, ok := j["registries"]
				Expect(ok).To(BeTrue())
				Expect(registries).To(HaveLen(2))

				Expect(registries[0].URL).To(Equal("hub1.example.com"))
				Expect(registries[1].URL).To(Equal("hub2.example.com"))
			})
		})

		Context("when user has not logged in to registries", func() {
			It("responds with an empty array", func() {
				doRequest(overrides{})

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				registries, ok := j["registries"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(registries).To(BeEmpty())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/registries"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/registries", func() {
		Context("when a valid oauth token is given", func() {

			doRequest := func(o overrides) {
				var ok bool

				slug := "foo"
				if _, ok = o["slug"]; ok {
					slug = o["slug"]
					delete(o, "slug")
				}

				if _, ok = o["username"]; !ok {
					o["username"] = "dockeruser"
				}

				if _, ok = o["password"]; !ok {
					o["password"] = "123456"
				}

				if _, ok = o["email"]; !ok {
					o["email"] = "dockeruser@example.com"
				}

				if _, ok = o["url"]; !ok {
					o["url"] = "hub.example.com"
				}

				params := &url.Values{}
				for k, v := range o {
					params.Set(k, v)

				}

				req, err := NewRequest("POST", "/api/v0/hosts/"+slug+"/registries", params)
				Expect(err).To(BeNil())
				req.Header.Set("Authorization", "Bearer valid-token")
				handler.ServeHTTP(res, req)
			}

			Context("with a valid host available for user", func() {
				Context("when username is not provided", func() {
					It("returns username required error", func() {
						doRequest(overrides{"username": ""})

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("invalid_request"))
						Expect(j.ErrorDescription).To(Equal("username is required"))
					})
				})

				Context("when password is not provided", func() {
					It("returns password required error", func() {
						doRequest(overrides{"password": ""})

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("invalid_request"))
						Expect(j.ErrorDescription).To(Equal("password is required"))
					})
				})

				Context("when email is not provided", func() {
					It("returns email required error", func() {
						doRequest(overrides{"email": ""})

						Expect(res.Code).To(Equal(http.StatusBadRequest))

						err, j := unmarshalErrorResponse()
						Expect(err).To(BeNil())

						Expect(j.Error).To(Equal("invalid_request"))
						Expect(j.ErrorDescription).To(Equal("email is required"))
					})
				})

				Context("when all params are provided", func() {
					It("calls the Freighter client with the given params to login to the registry", func() {
						doRequest(overrides{})

						Expect(mockClient.RegistryLoginCalled).To(Equal(1))
						Expect(mockClient.ReceivedUsername).To(Equal("dockeruser"))
						Expect(mockClient.ReceivedPassword).To(Equal("123456"))
						Expect(mockClient.ReceivedEmail).To(Equal("dockeruser@example.com"))
						Expect(mockClient.ReceivedRegistry).To(Equal("hub.example.com"))
					})

					Context("when registry login succeeds", func() {
						It("creates a registry record for the user and host", func() {
							doRequest(overrides{})
							regs, err := registry.FindAll(db, hs.U.ID, hs.H.ID)
							Expect(err).To(BeNil())
							Expect(regs).To(HaveLen(1))

							r := regs[0]
							Expect(r.Username).To(Equal("dockeruser"))
							Expect(r.Email).To(Equal("dockeruser@example.com"))
							Expect(r.URL).To(Equal("hub.example.com"))
						})

						It("responds with the registry user has logged in", func() {
							doRequest(overrides{})
							Expect(res.Code).To(Equal(http.StatusOK))
							var j map[string]serializers.Registry
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())
							r, ok := j["registry"]
							Expect(ok).To(BeTrue())

							Expect(r.Username).To(Equal("dockeruser"))
							Expect(r.Email).To(Equal("dockeruser@example.com"))
							Expect(r.URL).To(Equal("hub.example.com"))
						})

						It("publishes a registry added event with the registry", func(done Done) {
							ch := svr.Events.Subscribe(u.Topic())
							defer svr.Events.Unsubscribe(ch, u.Topic())

							doRequest(overrides{})

							regs, err := registry.FindAll(db, hs.U.ID, hs.H.ID)
							Expect(err).To(BeNil())
							Expect(regs).To(HaveLen(1))
							r := regs[0]

							Expect(<-ch).To(Equal(&controllers.Event{
								Name: controllers.EventRegistryAdded,
								Data: map[string]*serializers.Registry{
									"registry": serializers.SerializeRegistry(r),
								},
							}))
							close(done)
						})
					})

					Context("when registry login fails", func() {
						BeforeEach(func() {
							mockClient.RegistryLoginError = freighterclient.ErrInvalidParams
							doRequest(overrides{})
						})

						It("does not create a registry record for the user and host", func() {
							regs, err := registry.FindAll(db, hs.U.ID, hs.H.ID)
							Expect(err).To(BeNil())
							Expect(regs).To(BeEmpty())
						})

						It("informs user that login failed", func() {
							Expect(res.Code).To(Equal(http.StatusBadRequest))

							err, j := unmarshalErrorResponse()
							Expect(err).To(BeNil())

							Expect(j.Error).To(Equal("login_failed"))
							Expect(j.ErrorDescription).To(Equal("registry login failed"))
						})
					})

					Context("when already logged in to the registry", func() {
						It("responds with an error mentioning already logged in to the registry", func() {
							r := &registry.Registry{
								UserID:   hs.U.ID,
								HostID:   hs.H.ID,
								URL:      "hub.example.com",
								Username: "foo",
								Email:    "foo@example.com",
							}
							err := r.Insert(db)
							Expect(err).To(BeNil())

							doRequest(overrides{})

							Expect(res.Code).To(Equal(http.StatusBadRequest))
							err, j := unmarshalErrorResponse()
							Expect(err).To(BeNil())

							Expect(j.Error).To(Equal("login_failed"))
							Expect(j.ErrorDescription).To(Equal("already logged in to this registry"))
						})
					})

					It("responds with internal server error for other errors", func() {
						mockClient.RegistryLoginError = errors.New("some other error")
						doRequest(overrides{})

						Expect(res.Code).To(Equal(http.StatusInternalServerError))
					})
				})
			})

			Context("with an invalid host", func() {
				ItRespondsHostNotFound(doRequest)
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/registries"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}/registries", func() {
		doRequest := func(o overrides) {
			slug := "foo"
			if _, ok := o["slug"]; ok {
				slug = o["slug"]
			}

			r := "http://hub.example.com"
			if _, ok := o["registry"]; ok {
				r = o["registry"]
			}

			req, err := NewRequest("DELETE", "/api/v0/hosts/"+slug+"/registries/", nil)
			req.URL.Path += url.QueryEscape(r)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		Context("with a valid oauth token and host", func() {
			var r *registry.Registry
			BeforeEach(func() {
				r = &registry.Registry{
					UserID:   hs.U.ID,
					HostID:   hs.H.ID,
					URL:      "http://hub.example.com",
					Username: "foo",
					Email:    "foo@example.com",
				}
				err := r.Insert(db)
				Expect(err).To(BeNil())
			})

			Context("when registry is given as a URL", func() {
				It("calls the Freighter client with the given registry to logout", func() {
					doRequest(overrides{"registry": r.URL})

					Expect(mockClient.RegistryLogoutCalled).To(Equal(1))
					Expect(mockClient.ReceivedRegistry).To(Equal("http://hub.example.com"))
				})
			})

			Context("when registry is referred as an ID", func() {
				It("calls the Freighter client with the given registry's URL to logout", func() {
					doRequest(overrides{"registry": strconv.FormatInt(r.ID, 10)})

					Expect(mockClient.RegistryLogoutCalled).To(Equal(1))
					Expect(mockClient.ReceivedRegistry).To(Equal("http://hub.example.com"))
				})
			})

			Context("when log out is successful", func() {
				BeforeEach(func() {
					mockClient.RegistryLogoutError = nil
				})

				It("deletes the registry record", func() {
					doRequest(overrides{})
					rx, err := registry.FindAll(db, hs.U.ID, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(rx).To(HaveLen(0))
				})

				It("returns HTTP 200 OK if logged out successfully", func() {
					doRequest(overrides{})
					Expect(res.Code).To(Equal(http.StatusOK))

					var j map[string]string
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["status"]).To(Equal("ok"))
				})

				It("publishes a registry removed event with the registry", func(done Done) {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					doRequest(overrides{})

					Expect(<-ch).To(Equal(&controllers.Event{
						Name: controllers.EventRegistryRemoved,
						Data: map[string]*serializers.Registry{
							"registry": serializers.SerializeRegistry(r),
						},
					}))
					close(done)
				})
			})

			Context("when log out fails", func() {
				BeforeEach(func() {
					mockClient.RegistryLogoutError = freighterclient.ErrInvalidParams
					doRequest(overrides{})
				})

				It("does not delete the registry record", func() {
					rx, err := registry.FindAll(db, hs.U.ID, hs.H.ID)
					Expect(err).To(BeNil())
					Expect(rx).To(HaveLen(1))
				})

				It("informs user if logout failed", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))

					err, j := unmarshalErrorResponse()
					Expect(err).To(BeNil())

					Expect(j.Error).To(Equal("logout_failed"))
					Expect(j.ErrorDescription).To(Equal("registry logout failed"))
				})
			})

			Context("when given registry does not exist", func() {
				It("calls the Freighter client with the given registry's URL to logout", func() {
					doRequest(overrides{"registry": "fail.example.com"})

					Expect(res.Code).To(Equal(http.StatusNotFound))
				})
			})

			It("returns internal server error for other errors", func() {
				mockClient.RegistryLogoutError = errors.New("some other error")
				doRequest(overrides{})

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		Context("with an invalid host", func() {
			ItRespondsHostNotFound(doRequest)
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/registries/10"
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
