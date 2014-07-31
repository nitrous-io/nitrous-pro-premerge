package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
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

		emptyMw = func(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
			return nil
		}
		env = &middleware.Env{}
	})

	Describe("Vars", func() {
		It("sets the Vars in the environment", func() {
			h := func(w http.ResponseWriter, r *http.Request) {
				middleware.Vars(emptyMw)(env, svr, w, r)
			}
			handler.HandleFunc("/path/{name}", h).Methods("GET")

			req, err := NewRequest("GET", "/path/foo", nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)

			Expect(env.Vars["name"]).To(Equal("foo"))
		})
	})

	Describe("Auth", func() {
		var (
			u *user.User
			t *token.Token
		)
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
				Token:  "foobar",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())

			h := func(w http.ResponseWriter, r *http.Request) {
				err := middleware.Auth(emptyMw)(env, svr, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler.HandleFunc("/authaction", h).Methods("GET")
		})

		doRequest := func(token string) {
			req, err := NewRequest("GET", "/authaction", nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

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

				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_token"))
				Expect(errDesc).To(Equal("auth token is invalid"))
			})
		})

		Context("when no oauth token is given", func() {
			BeforeEach(func() {
				doRequest("")
			})

			It("responds with 400 Bad Request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				Expect(res.Header().Get("WWW-Authenticate")).To(Equal(`Bearer realm="nitrous", error="invalid_request"`))
				errType, errDesc := ErrorDetails(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("auth token is required"))
			})
		})
	})
})
