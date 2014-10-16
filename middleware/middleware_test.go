package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/server"

	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "middleware")
}

var _ = Describe("middleware", func() {
	var (
		db      *db.Database
		svr     *server.Server
		handler *mux.Router
		res     *httptest.ResponseRecorder
		env     *middleware.Env
		emptyMw middleware.Middleware
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)

		handler = mux.NewRouter()
		res = httptest.NewRecorder()

		emptyMw = func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
			return nil
		}
		env = &middleware.Env{
			Logger:  logger,
			DB:      db,
			Tracker: NewFakeTracker(),
		}
	})

	Describe("authentication middleware", func() {
		var (
			doRequest func(string)
			u         *user.User
			mw        func(middleware.Middleware) middleware.Middleware
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			t := &token.Token{
				UserID: u.ID,
				Token:  "foobar",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			h := func(w http.ResponseWriter, r *http.Request) {
				err := mw(emptyMw)(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler.HandleFunc("/authaction", h).Methods("GET")
		})

		itWorks := func() {
			Context("when a valid oauth token is given", func() {
				BeforeEach(func() {
					doRequest("foobar")
				})

				It("sets the User in the environment", func() {
					Expect(env.User).To(Equal(u))
				})

				It("Responds with 200 OK", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when an invalid oauth token is given", func() {
				BeforeEach(func() {
					doRequest("invalidtoken")
				})

				It("responds with 401 Unauthorized", func() {
					Expect(res.Code).To(Equal(http.StatusUnauthorized))
					Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_token"`))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_token"))
					Expect(j["error_description"]).To(Equal("auth token is invalid"))
				})
			})

			Context("when no oauth token is given", func() {
				BeforeEach(func() {
					doRequest("")
				})

				It("responds with 400 Bad Request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_request"`))
					j := map[string]interface{}{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
					Expect(j["error_description"]).To(Equal("auth token is required"))
				})
			})
		}

		// authenticate using Authorization header
		Describe("Auth", func() {
			BeforeEach(func() {
				mw = middleware.Auth
				doRequest = func(token string) {
					req, err := NewRequest("GET", "/authaction", nil)
					Expect(err).To(BeNil())
					if token != "" {
						req.Header.Set("Authorization", "Bearer "+token)
					}
					handler.ServeHTTP(res, req)
				}
			})

			itWorks()
		})

		// authenticate using Authorization header OR query string (?token=)
		Describe("AuthQS", func() {
			BeforeEach(func() {
				mw = middleware.AuthQS
			})

			Context("auth using Authorization header", func() {
				BeforeEach(func() {
					doRequest = func(token string) {
						req, err := NewRequest("GET", "/authaction", nil)
						Expect(err).To(BeNil())
						if token != "" {
							req.Header.Set("Authorization", "Bearer "+token)
						}
						handler.ServeHTTP(res, req)
					}
				})

				itWorks()
			})

			Context("auth using token query string", func() {
				BeforeEach(func() {
					doRequest = func(token string) {
						params := &url.Values{}
						params.Set("token", token)
						req, err := NewRequest("GET", "/authaction", params)
						Expect(err).To(BeNil())
						handler.ServeHTTP(res, req)
					}
				})

				itWorks()
			})
		})
	})

	Describe("event tracking middleware", func() {
		var (
			u  *user.User
			mw func(event string, mw middleware.Middleware) middleware.Middleware
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())
			env.User = u

			env.RemoteAddr = "192.168.1.1"

			mw = middleware.Track
			h := func(w http.ResponseWriter, r *http.Request) {
				err := middleware.Track("Action Called", emptyMw)(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler.HandleFunc("/action", h).Methods("GET")

			req, err := NewRequest("GET", "/action", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		})

		It("sets the TrackEventName in the environment", func() {
			Expect(env.TrackEventName).To(Equal("Action Called"))
		})

		It("adds $ip to event data", func() {
			t := env.Tracker.(*FakeTracker)
			Expect(t.TrackEventProperties[u.ID][0]["$ip"]).To(Equal("192.168.1.1"))
		})

		It("Responds with 200 OK", func() {
			Expect(res.Code).To(Equal(http.StatusOK))
		})
	})
})
