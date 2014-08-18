package images_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
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
		UnstubFreighterFn()
	})

	newImage := func() *freighterclient.Image {
		return &freighterclient.Image{
			ID:          "1a",
			Name:        "source1",
			Tag:         "sourcetag1",
			CreatedAt:   1903,
			ActualSize:  13,
			VirtualSize: 29,
		}
	}

	Describe("GET /api/v0/hosts/{slug}/images", func() {
		doRequest := func(host string) {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%v/images", host), nil)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		var (
			img1 *freighterclient.Image
			img2 *freighterclient.Image
		)

		BeforeEach(func() {
			img1 = newImage()
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

		It("responds with the images from Freighter", func() {
			doRequest("foo")

			Expect(mockClient.ListImagesCalled).To(Equal(1))
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
				doRequest("foo")

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
				doRequest("foo")

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

		Context("when host does not exist for user", func() {
			It("returns HTTP 404 Not Found", func() {
				doRequest("no-such-host")

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

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/hosts/foo/images"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/image", func() {
		doRequest := func(host, image string, files map[string]string) {
			req, err := NewFileRequest("POST",
				fmt.Sprintf("/api/v0/hosts/%v/images", host),
				map[string]string{"name": image},
				files,
			)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer "+t.Token)
			handler.ServeHTTP(res, req)
		}

		It("responds with image on success", func() {
			mockClient.BuildImageReturn = &freighterclient.Image{
				Name: "test",
			}
			doRequest("foo", "scratcher", map[string]string{"file": "FROM fake"})
			j := struct {
				Image freighterclient.Image `json:"image"`
			}{}
			sent, err := ioutil.ReadAll(mockClient.BuildImageReader)
			Expect(err).To(BeNil())
			Expect(string(sent)).To(Equal("FROM fake"))
			Expect(mockClient.BuildImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
			err = json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Image.Name).To(Equal("test"))
		})

		It("requires name", func() {
			doRequest("foo", "", map[string]string{"file": "filecontent"})
			Expect(mockClient.BuildImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("name is required"))
		})

		It("requires file", func() {
			doRequest("foo", "name", map[string]string{})
			Expect(mockClient.BuildImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("file is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.BuildImageError = &freighterclient.StatusError{418}
			doRequest("foo", "scratcher", map[string]string{"file": "FROM fake"})
			Expect(mockClient.BuildImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}/image/{image}", func() {
		doRequest := func(host, image string) {
			req, err := NewAuthedRequest(t, "DELETE", fmt.Sprintf("/api/v0/hosts/%v/images", host), &url.Values{"image": []string{"elixir"}})
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("calls RemoveImage", func() {
			doRequest("foo", "elixir")

			Expect(mockClient.RemoveImageCalled).To(Equal(1))
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/pull", func() {
		doRequest := func(host string, params *url.Values) {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/pull", host), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("responds with image on success", func() {
			mockClient.PullImageReturn = newImage()
			doRequest("foo", &url.Values{"repository": []string{"scratch"}})
			Expect(mockClient.PullImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
			j := struct {
				Image freighterclient.Image `json:"image"`
			}{}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Image.ID).To(Equal("1a"))
			Expect(j.Image.Name).To(Equal("source1"))
		})

		It("requires repository param", func() {
			doRequest("foo", nil)
			Expect(mockClient.PullImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("repository is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.PullImageError = &freighterclient.StatusError{418}
			doRequest("foo", &url.Values{"repository": []string{"scratch"}})
			Expect(mockClient.PullImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/push", func() {
		doRequest := func(host string, params *url.Values) {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/push", host), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("calls PushImage", func() {
			doRequest("foo", &url.Values{
				"image":       []string{"devenv"},
				"destination": []string{"myuser/scratch"},
			})
			Expect(mockClient.PushImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("requires destination param", func() {
			doRequest("foo", &url.Values{
				"image": []string{"devenv"},
			})
			Expect(mockClient.PushImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("destination is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.PushImageError = &freighterclient.StatusError{418}
			doRequest("foo", &url.Values{
				"image":       []string{"devenv"},
				"destination": []string{"scratch"},
			})
			Expect(mockClient.PushImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/tag", func() {
		doRequest := func(host string, params *url.Values) {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/tag", host), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		It("calls TagImage", func() {
			doRequest("foo", &url.Values{
				"image": []string{"scratch"},
				"tag":   []string{"staging"},
			})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.ReceivedImage).To(Equal("scratch"))
			Expect(mockClient.ReceivedTag).To(Equal("staging"))
			Expect(mockClient.TagImageForce).To(BeFalse())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("sends force param", func() {
			doRequest("foo", &url.Values{
				"image": []string{"scratch"},
				"tag":   []string{"staging"},
				"force": []string{"1"},
			})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.TagImageForce).To(BeTrue())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("requires tag param", func() {
			doRequest("foo", &url.Values{
				"image": []string{"scratch"},
			})
			Expect(mockClient.TagImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("tag is required"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.TagImageError = &freighterclient.StatusError{418}
			doRequest("foo", &url.Values{
				"image": []string{"scratch"},
				"tag":   []string{"staging"},
			})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})
})
