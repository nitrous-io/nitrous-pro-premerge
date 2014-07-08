package users_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/config"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/users")
}

var _ = Describe("controllers/users", func() {
	var (
		db              *db.Database
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		req             *http.Request
		svr             *server.Server
		params          *url.Values
		authTokenInputs AuthTokenInputs
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		env := &config.Env{}
		svr = server.NewServer("", db, logger, env)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /api/v0/users", func() {
		doRequest := func() {
			var err error
			req, err = NewFormRequest("POST", "/api/v0/users", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when valid params are received", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foo@example.com")
				params.Set("username", "foo")
				params.Set("password", "123456")
				params.Set("name", "Foo Bar")
				doRequest()
			})

			It("responds with ok", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["status"]).To(Equal("ok"))
			})

			It("creates a user record in the db", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Name).To(Equal("Foo Bar"))
				Expect(u.CreatedAt).NotTo(BeNil())
			})

			It("encrypts the password using bcrypt", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(bcrypt.Verify(u.EncryptedPassword, "123456")).To(BeTrue())
			})
		})

		for _, v := range []string{"email", "username", "password"} {
			Context(fmt.Sprintf(`when the required param "%s" is missing`, v), func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("email", "foo@example.com")
					params.Set("username", "foo")
					params.Set("password", "123456")
					params.Del(v)
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal(v + " can't be blank"))
				})
			})
		}

		Context("when other types of validation fails", func() {
			BeforeEach(func() {
				params = &url.Values{}
				params.Set("email", "foobar")
				params.Set("username", "foo")
				params.Set("password", "123456")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]string{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("validation_failed"))
				Expect(j["error_description"]).To(Equal("email is not valid"))
			})
		})

		Context("with an existing user", func() {
			BeforeEach(func() {
				_, err := db.Exec("INSERT INTO users (username, email, encrypted_password) VALUES ('foo', 'foo@example.com', '!@#$%')")
				Expect(err).To(BeNil())
				params = &url.Values{}
				params.Set("email", "lol@example.com")
				params.Set("username", "bar")
				params.Set("password", "123456")
			})

			Context("when an existing username is given", func() {
				BeforeEach(func() {
					params.Set("username", "foo")
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal("username is taken"))
				})
			})

			Context("when an existing email is given", func() {
				BeforeEach(func() {
					params.Set("email", "foo@example.com")
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("validation_failed"))
					Expect(j["error_description"]).To(Equal("email is taken"))
				})
			})
		})
	})

	Describe("GET /api/v0/user", func() {
		var (
			u *user.User
			t *token.Token
		)

		doRequest := func(token string) {
			var err error
			req, err = NewRequest("GET", "/api/v0/user", nil)
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
				Token:  "foobarbazqux",
			}
			err = t.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when a valid oauth token is given", func() {
			BeforeEach(func() {
				doRequest("foobarbazqux")
			})

			It("responds with the authenticated user's details", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				userinfo, ok := j["user"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				Expect(userinfo).NotTo(BeNil())
				Expect(userinfo["username"]).To(Equal("foo"))
				Expect(userinfo["email"]).To(Equal("foo@example.com"))
				Expect(userinfo["created_at"]).NotTo(BeNil())
				Expect(userinfo["updated_at"]).NotTo(BeNil())
				Expect(userinfo["password"]).To(BeNil())
				Expect(userinfo["encrypted_password"]).To(BeNil())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/user"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
