package images_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/images"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/images")
}

var _ = Describe("controllers/images", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		images.FreighterClient = mockClient

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
	})

	Describe("GET /api/v0/hosts/{slug}/images", func() {
		doRequest := func(host string, validAuth bool) {
			authToken := &token.Token{Token: "notgonnawork"}
			if validAuth {
				authToken = t
			}
			req, err := NewAuthedRequest(authToken, "GET", fmt.Sprintf("/api/v0/hosts/%v/images", host), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			Context("when host exists for user", func() {

				Context("when host has images", func() {
					var (
						img1 *freighterclient.Image
						img2 *freighterclient.Image
					)

					BeforeEach(func() {
						img1 = &freighterclient.Image{
							ID:          "1a",
							Name:        "source1",
							Tag:         "sourcetag1",
							CreatedAt:   1903,
							ActualSize:  13,
							VirtualSize: 29,
						}
						img2 = &freighterclient.Image{
							ID:          "2a",
							Name:        "source2",
							Tag:         "sourcetag2",
							CreatedAt:   1904,
							ActualSize:  14,
							VirtualSize: 30,
						}
						mockClient.ListImagesArray = []*freighterclient.Image{img1, img2}
					})

					It("calls the Freighter client to list images", func() {
						doRequest("foo", true)
						Expect(mockClient.ListImagesCalled).To(Equal(1))
					})

					It("responds with the images from Freighter", func() {
						doRequest("foo", true)

						Expect(res.Code).To(Equal(http.StatusOK))

						var j map[string][]freighterclient.Image
						data := res.Body.Bytes()
						err := json.Unmarshal(data, &j)
						Expect(err).To(BeNil())
						images, ok := j["images"]
						Expect(ok).To(BeTrue())
						Expect(images).To(HaveLen(2))

						// Fields from Freighter API.
						Expect(images[0].ID).To(Equal("1a"))
						Expect(images[0].Name).To(Equal("source1"))
						Expect(images[0].Tag).To(Equal("sourcetag1"))
						Expect(images[0].CreatedAt).To(Equal(int64(1903)))
						Expect(images[0].ActualSize).To(Equal(int64(13)))
						Expect(images[0].VirtualSize).To(Equal(int64(29)))
					})

					Context("when the Freighter client returns an error", func() {
						BeforeEach(func() {
							mockClient.ListImagesError = errors.New("you are not Chuck Norris")
						})

						It("responds with HTTP 500 Internal Server Error", func() {
							doRequest("foo", true)

							Expect(res.Code).To(Equal(http.StatusInternalServerError))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("internal_server_error"))
						})
					})

					Context("when host has no images", func() {
						BeforeEach(func() {
							mockClient.ListImagesArray = []*freighterclient.Image{}
						})

						It("responds with an empty array", func() {
							doRequest("foo", true)

							Expect(res.Code).To(Equal(http.StatusOK))
							j := map[string]interface{}{}

							data := res.Body.Bytes()
							err := json.Unmarshal(data, &j)
							Expect(err).To(BeNil())
							images, ok := j["images"].([]interface{})
							Expect(ok).To(BeTrue())
							Expect(images).To(BeEmpty())
						})
					})

					Context("when host IP address is not available", func() {
						BeforeEach(func() {
							_, err := CreateHostStack(db, &HostStack{
								H: &host.Host{
									Slug:      "bar",
									IPAddress: sql.NullString{},
								},
								U:  u,
								R:  hs.R,
								P:  hs.P,
								PT: hs.PT,
							})
							Expect(err).To(BeNil())
						})

						It("returns HTTP 400 Bad Request", func() {
							doRequest("bar", true)

							Expect(res.Code).To(Equal(http.StatusBadRequest))
							Expect(mockClient.ListImagesCalled).To(Equal(0))

							j := map[string]interface{}{}
							err := json.Unmarshal(res.Body.Bytes(), &j)
							Expect(err).To(BeNil())

							errType, _ := j["error"].(string)
							Expect(errType).To(Equal("invalid_request"))

							errDesc, _ := j["error_description"].(string)
							Expect(errDesc).To(Equal("host IP is unknown"))
						})
					})
				})

				Context("when host does not exist for user", func() {
					It("returns HTTP 404 Not Found", func() {
						doRequest("no-such-host", true)

						Expect(res.Code).To(Equal(http.StatusNotFound))
						Expect(mockClient.ListImagesCalled).To(Equal(0))

						j := map[string]interface{}{}
						err := json.Unmarshal(res.Body.Bytes(), &j)
						Expect(err).To(BeNil())

						errType, _ := j["error"].(string)
						Expect(errType).To(Equal("not_found"))

						errDesc, _ := j["error_description"].(string)
						Expect(errDesc).To(Equal("host does not exist"))
					})
				})
			})

			Context("oauth token failure modes", func() {
				BeforeEach(func() {
					authTokenInputs.Path = "/api/v0/hosts/foo/images"
					authTokenInputs.Method = "GET"
				})

				ItRequiresAuthToken(&authTokenInputs)
			})
		})
	})
})
