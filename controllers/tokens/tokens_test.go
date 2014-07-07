package tokens_test

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
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/tokens")
}

var _ = Describe("controllers/tokens", func() {
	var (
		db      *db.Database
		handler *mux.Router
		res     *httptest.ResponseRecorder
		req     *http.Request
		svr     *server.Server
		params  *url.Values
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
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /tokens", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			Expect(u.ID).NotTo(BeZero())

			params = &url.Values{}
			params.Set("grant_type", "password")
			params.Set("username", "foo")
			params.Set("password", "123456")
		})

		doRequest := func() {
			var err error
			req, err = NewFormRequest("POST", "/oauth/token", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when valid params are received", func() {
			var j map[string]interface{}

			BeforeEach(func() {
				doRequest()
				j = map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
			})

			It("responds with a unique access token", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(j["access_token"]).NotTo(BeNil())
				Expect(j["token_type"]).To(Equal("Bearer"))
			})

			It("persists the token in the database", func() {
				t, err := token.FindByToken(db, j["access_token"].(string))
				Expect(err).To(BeNil())
				Expect(t).NotTo(BeNil())
				Expect(t.UserID).To(Equal(u.ID))
				Expect(t.Token).To(Equal(j["access_token"]))
				Expect(t.CreatedAt).NotTo(BeNil())
			})

			It("has no-cache headers", func() {
				Expect(res.Header().Get("Cache-Control")).To(Equal("no-store"))
				Expect(res.Header().Get("Pragma")).To(Equal("no-cache"))
			})

			Context("making another token request", func() {
				var prevToken interface{}

				BeforeEach(func() {
					res = httptest.NewRecorder()
					prevToken = j["access_token"]
					doRequest()
				})

				It("responds with a different access token", func() {
					Expect(res.Code).To(Equal(http.StatusOK))
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["access_token"]).NotTo(Equal(prevToken))
					Expect(j["token_type"]).To(Equal("Bearer"))
				})
			})
		})

		Context("when a wrong password is given", func() {
			BeforeEach(func() {
				params.Set("password", "111111")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_grant"))
			})
		})

		Context("when grant type is not password", func() {
			BeforeEach(func() {
				params.Set("grant_type", "authorization_code")
				doRequest()
			})

			It("responds with 400 bad request", func() {
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("unsupported_grant_type"))
			})
		})

		for _, v := range []string{"username", "password"} {
			Context(fmt.Sprintf(`when the required param "%s" is missing`, v), func() {
				BeforeEach(func() {
					params.Del(v)
					doRequest()
				})

				It("responds with 400 bad request", func() {
					Expect(res.Code).To(Equal(http.StatusBadRequest))
					j := map[string]string{}
					err := json.Unmarshal(res.Body.Bytes(), &j)
					Expect(err).To(BeNil())
					Expect(j["error"]).To(Equal("invalid_request"))
					Expect(j["error_description"]).To(Equal(v + " is required"))
				})
			})
		}
	})
})
