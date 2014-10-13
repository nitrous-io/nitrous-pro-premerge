package tokens_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/tokens"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/analytics"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/tokens")
}

func init() {
	tokens.LoginAttemptsWaitTime = map[int]time.Duration{
		3: time.Duration(0),
		6: time.Millisecond * 200,
		9: time.Millisecond * 400,
	}
	tokens.LoginAttemptsFinalWaitTime = time.Millisecond * 600
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
		svr = server.NewServer("", db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("POST /tokens", func() {
		var (
			u                       *user.User
			fakeAnalyticsClient     *FakeAnalytics
			unstubAnalyticsClientFn func()
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

			fakeAnalyticsClient, unstubAnalyticsClientFn = StubAnalyticsClient()

			params = &url.Values{}
			params.Set("grant_type", "password")
			params.Set("username", "foo")
			params.Set("password", "123456")
		})

		AfterEach(func() {
			unstubAnalyticsClientFn()
		})

		doRequest := func() {
			var err error
			req, err = NewFormRequest("POST", "/api/v0/oauth/token", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		logins := []string{
			"foo",
			"foo@example.com",
		}

		for _, login := range logins {
			l := login

			Context("when requested with valid login("+l+") and password", func() {
				var j map[string]interface{}

				BeforeEach(func() {
					params.Set("username", l)
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

				It("tracks the 'User Signed In' event", func() {
					Expect(fakeAnalyticsClient.TrackEventCalled).To(Equal(1))
					Expect(fakeAnalyticsClient.Events).To(Equal([]string{analytics.UserSignedIn}))
					Expect(fakeAnalyticsClient.User.ID).To(Equal(u.ID))
				})

				It("tracks the user as a new analytics profile", func() {
					Expect(fakeAnalyticsClient.CreateProfileCalled).To(Equal(1))
					Expect(fakeAnalyticsClient.User.ID).To(Equal(u.ID))
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
		}

		Context("when a wrong password is given", func() {
			It("responds with 400 bad request", func() {
				params.Set("password", "111111")
				doRequest()

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				Expect(j["error"]).To(Equal("invalid_grant"))
			})

			It("should not delay under 3 requests", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				params.Set("password", "111111")

				start := time.Now()
				doRequest()
				Expect(time.Now().Sub(start).Seconds()).To(BeNumerically("<", 0.2))

				start = time.Now()
				doRequest()
				Expect(time.Now().Sub(start).Seconds()).To(BeNumerically("<", 0.2))

				start = time.Now()
				doRequest()
				Expect(time.Now().Sub(start).Seconds()).To(BeNumerically("<", 0.2))
			})

			It("should delay correctly", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				params.Set("password", "111111")

				start := time.Now()
				doRequest()
				Expect(time.Now().Sub(start).Seconds()).To(BeNumerically("<", 0.2))

				_, err = db.Exec("UPDATE users SET login_attempts = 4 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				start = time.Now()
				doRequest()
				diff := time.Now().Sub(start).Seconds()
				Expect(diff).To(BeNumerically(">=", 0.2))
				Expect(diff).To(BeNumerically("<", 0.4))

				_, err = db.Exec("UPDATE users SET login_attempts = 7 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				start = time.Now()
				doRequest()
				diff = time.Now().Sub(start).Seconds()
				Expect(diff).To(BeNumerically(">=", 0.4))
				Expect(diff).To(BeNumerically("<", 0.6))

				_, err = db.Exec("UPDATE users SET login_attempts = 10 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				start = time.Now()
				doRequest()
				diff = time.Now().Sub(start).Seconds()
				Expect(diff).To(BeNumerically(">=", 0.6))
			})

			It("should reset the delay after a successful auth", func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 3 WHERE username = $1", u.Username)
				Expect(err).To(BeNil())

				params.Set("password", "123456")
				start := time.Now()
				doRequest()
				Expect(time.Now().Sub(start).Seconds()).To(BeNumerically(">=", 0.2))

				start = time.Now()
				doRequest()
				Expect(time.Now().Sub(start).Seconds()).To(BeNumerically("<", 0.2))
			})

			It("shouldn't track any events", func() {
				Expect(fakeAnalyticsClient.TrackEventCalled).To(Equal(0))
				Expect(fakeAnalyticsClient.Events).To(BeEmpty())
				Expect(fakeAnalyticsClient.User).To(BeNil())
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

			It("shouldn't track any events", func() {
				Expect(fakeAnalyticsClient.TrackEventCalled).To(Equal(0))
				Expect(fakeAnalyticsClient.Events).To(BeEmpty())
				Expect(fakeAnalyticsClient.User).To(BeNil())
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

				It("shouldn't track any events", func() {
					Expect(fakeAnalyticsClient.TrackEventCalled).To(Equal(0))
					Expect(fakeAnalyticsClient.Events).To(BeEmpty())
					Expect(fakeAnalyticsClient.User).To(BeNil())
				})
			})
		}
	})
})
