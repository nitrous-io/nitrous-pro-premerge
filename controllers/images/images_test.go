package images_test

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/isarchive"
	"github.com/nitrous-io/nitrous-common/logger"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
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
	RunSpecs(t, "controllers/images")
}

var _ = Describe("controllers/images", func() {
	var (
		db              *db.Database
		svr             *server.Server
		authTokenInputs AuthTokenInputs

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	makeDockerfile := func(tmpdir string) string {
		path := filepath.Join(tmpdir, "Dockerfile")
		err := ioutil.WriteFile(path, []byte("FROM scratch"), 0600)
		Expect(err).To(BeNil())
		return path
	}

	makeTar := func(dir, source, output string) error {
		cmd := exec.Command("tar", "cf", output, path.Base(source))
		cmd.Dir = dir
		return cmd.Run()
	}

	makeTempDir := func() string {
		tmpdir, err := ioutil.TempDir("", "nitro-build")
		Expect(err).To(BeNil())
		return tmpdir
	}

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer("", db, logger)
		authTokenInputs = AuthTokenInputs{Handler: svr.Router()}

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

	Describe("GET /api/v0/hosts/{slug}/images", func() {
		doRequest := func(host string, params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", fmt.Sprintf("/api/v0/hosts/%v/images", host), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		var (
			img1 *freighterclient.Image
			img2 *freighterclient.Image

			i1 *image.Image
			i2 *image.Image
		)

		BeforeEach(func() {
			img1 = &freighterclient.Image{
				ID:          "1a",
				Name:        "source1",
				Tag:         "sourcetag1",
				CreatedAt:   time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC).Unix(),
				ActualSize:  13,
				VirtualSize: 29,
			}
			img2 = &freighterclient.Image{
				ID:          "2a",
				Name:        "source2",
				Tag:         "sourcetag2",
				CreatedAt:   time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC).Unix(),
				ActualSize:  14,
				VirtualSize: 30,
			}
			mockClient.ListImagesArray = []*freighterclient.Image{img1, img2}

			i1 = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "1a",
				Name:        "source1 image",
				Tag:         "sourcetag1",
				Slug:        "source1",
			}
			err := i1.Insert(db)
			Expect(err).To(BeNil())

			i2 = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "2a",
				Name:        "source2 image",
				Tag:         "sourcetag2",
				Slug:        "source2",
			}
			err = i2.Insert(db)
			Expect(err).To(BeNil())
		})

		It("responds with images from the database augmented with data from Freighter", func() {
			res := doRequest("foo", nil)

			Expect(mockClient.ListImagesCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string][]serializers.Image
			data := res.Body.Bytes()
			err := json.Unmarshal(data, &j)
			Expect(err).To(BeNil())
			images, ok := j["images"]
			Expect(ok).To(BeTrue())
			Expect(images).To(HaveLen(2))

			Expect(images[0].FreighterID).To(Equal("1a"))
			Expect(images[0].Name).To(Equal("source1 image"))
			Expect(images[0].Slug).To(Equal("source1"))
			Expect(images[0].Tag).To(Equal("sourcetag1"))
			Expect(images[0].ActualSize).To(Equal(int64(13)))
			Expect(images[0].VirtualSize).To(Equal(int64(29)))
			builtAt := time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			Expect(images[0].BuiltAt).To(Equal(&builtAt))
			Expect(images[0].CreatedAt).NotTo(BeNil())
			Expect(images[0].UpdatedAt).NotTo(BeNil())

			Expect(images[1].FreighterID).To(Equal("2a"))
			Expect(images[1].Name).To(Equal("source2 image"))
			Expect(images[1].Slug).To(Equal("source2"))
			Expect(images[1].Tag).To(Equal("sourcetag2"))
			Expect(images[1].ActualSize).To(Equal(int64(14)))
			Expect(images[1].VirtualSize).To(Equal(int64(30)))
			builtAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
			Expect(images[1].BuiltAt).To(Equal(&builtAt))
			Expect(images[1].CreatedAt).NotTo(BeNil())
			Expect(images[1].UpdatedAt).NotTo(BeNil())
		})

		Context("when only hidden images are requested", func() {
			BeforeEach(func() {
				fOnlyImg := &freighterclient.Image{
					ID:          "3a",
					Name:        "source3",
					Tag:         "sourcetag3",
					CreatedAt:   time.Date(2014, time.August, 22, 0, 0, 0, 0, time.UTC).Unix(),
					ActualSize:  9000,
					VirtualSize: 9001,
				}

				mockClient.ListImagesArray = []*freighterclient.Image{img1, img2, fOnlyImg}
			})

			It("should list *only* the images not present in the DB", func() {
				res := doRequest("foo", &url.Values{"hidden": []string{"true"}})

				Expect(mockClient.ListImagesCalled).To(Equal(1))
				Expect(res.Code).To(Equal(http.StatusOK))

				var j map[string][]serializers.Image
				data := res.Body.Bytes()
				err := json.Unmarshal(data, &j)
				Expect(err).To(BeNil())
				images, ok := j["images"]
				Expect(ok).To(BeTrue())
				Expect(images).To(HaveLen(1))

				Expect(images[0].FreighterID).To(Equal("3a"))
				Expect(images[0].Name).To(BeEmpty())
				Expect(images[0].Slug).To(Equal("source3"))
				Expect(images[0].Tag).To(Equal("sourcetag3"))
				Expect(images[0].ActualSize).To(Equal(int64(9000)))
				Expect(images[0].VirtualSize).To(Equal(int64(9001)))
				builtAt := time.Date(2014, time.August, 22, 0, 0, 0, 0, time.UTC)
				Expect(images[0].BuiltAt).To(Equal(&builtAt))
				Expect(images[0].CreatedAt).To(BeNil())
				Expect(images[0].UpdatedAt).To(BeNil())
			})
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.ListImagesError = errors.New("you are not Chuck Norris")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				res := doRequest("foo", nil)

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
				_, err := db.Exec(`DELETE FROM images;`)
				Expect(err).To(BeNil())
			})

			It("responds with an empty array", func() {
				res := doRequest("foo", nil)

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

		Context("when host has images that are not in the database", func() {
			It("does not include those images in the response", func() {
				err := i2.Delete(db)
				Expect(err).To(BeNil())

				res := doRequest("foo", nil)

				var j map[string][]serializers.Image
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				images, ok := j["images"]
				Expect(ok).To(BeTrue())
				Expect(images).To(HaveLen(1))
				Expect(images[0].FreighterID).To(Equal("1a"))
			})
		})

		Context("when host does not exist for user", func() {
			It("returns HTTP 404 Not Found", func() {
				res := doRequest("no-such-host", nil)

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

	Describe("GET /api/v0/images", func() {
		doRequest := func() *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "GET", "/api/v0/images", nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		var (
			img1 *freighterclient.Image
			img2 *freighterclient.Image

			i1 *image.Image
			i2 *image.Image
		)

		BeforeEach(func() {
			hs2, err := CreateHostStack(db, &HostStack{
				H: &host.Host{
					Slug:      "bar",
					IPAddress: sql.NullString{"127.0.0.1", true},
				},
				U: hs.U,
			})
			Expect(err).To(BeNil())

			img1 = &freighterclient.Image{
				ID:          "1a",
				Name:        "source1",
				Tag:         "sourcetag1",
				CreatedAt:   time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC).Unix(),
				ActualSize:  13,
				VirtualSize: 29,
			}
			img2 = &freighterclient.Image{
				ID:          "2a",
				Name:        "source2",
				Tag:         "sourcetag2",
				CreatedAt:   time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC).Unix(),
				ActualSize:  14,
				VirtualSize: 30,
			}
			mockClient.ListImagesArray = []*freighterclient.Image{img1, img2}

			i1 = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "1a",
				Name:        "source1 image",
				Tag:         "sourcetag1",
				Slug:        "source1",
			}
			err = i1.Insert(db)
			Expect(err).To(BeNil())

			i2 = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs2.H.ID,
				FreighterID: "2a",
				Name:        "source2 image",
				Tag:         "sourcetag2",
				Slug:        "source2",
			}
			err = i2.Insert(db)
			Expect(err).To(BeNil())
		})

		It("responds with images from the database augmented with data from Freighter", func() {
			res := doRequest()

			Expect(mockClient.ListImagesCalled).To(Equal(2))
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string][]serializers.Image
			data := res.Body.Bytes()
			err := json.Unmarshal(data, &j)
			Expect(err).To(BeNil())
			images, ok := j["images"]
			Expect(ok).To(BeTrue())
			Expect(images).To(HaveLen(2))

			Expect(images[0].FreighterID).To(Equal("1a"))
			Expect(images[0].Name).To(Equal("source1 image"))
			Expect(images[0].Slug).To(Equal("source1"))
			Expect(images[0].Tag).To(Equal("sourcetag1"))
			Expect(images[0].ActualSize).To(Equal(int64(13)))
			Expect(images[0].VirtualSize).To(Equal(int64(29)))
			builtAt := time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			Expect(images[0].BuiltAt).To(Equal(&builtAt))
			Expect(images[0].CreatedAt).NotTo(BeNil())
			Expect(images[0].UpdatedAt).NotTo(BeNil())

			Expect(images[1].FreighterID).To(Equal("2a"))
			Expect(images[1].Name).To(Equal("source2 image"))
			Expect(images[1].Slug).To(Equal("source2"))
			Expect(images[1].Tag).To(Equal("sourcetag2"))
			Expect(images[1].ActualSize).To(Equal(int64(14)))
			Expect(images[1].VirtualSize).To(Equal(int64(30)))
			builtAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
			Expect(images[1].BuiltAt).To(Equal(&builtAt))
			Expect(images[1].CreatedAt).NotTo(BeNil())
			Expect(images[1].UpdatedAt).NotTo(BeNil())
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/images"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images", func() {
		doRequest := func(host string, params map[string]string, files map[string]io.Reader) *httptest.ResponseRecorder {
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)

			for k, v := range params {
				err := writer.WriteField(k, v)
				Expect(err).To(BeNil())
			}

			for n, c := range files {
				part, err := writer.CreateFormFile("file", n)
				Expect(err).To(BeNil())

				_, err = io.Copy(part, c)
				Expect(err).To(BeNil())
			}

			err := writer.Close()
			Expect(err).To(BeNil())

			req, err := http.NewRequest("POST",
				fmt.Sprintf("/api/v0/hosts/%v/images", host), body)
			Expect(err).To(BeNil())
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("Authorization", "Bearer "+t.Token)

			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("requires slug", func() {
			params := map[string]string{
				"slug": "",
				"code": "FROM scratch",
			}
			res := doRequest("foo", params, map[string]io.Reader{})

			Expect(mockClient.BuildImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("slug is required"))
		})

		It("requires file param to be set if both remote and code params are empty", func() {
			params := map[string]string{
				"slug":   "scratcher:mytag",
				"code":   "",
				"remote": "",
			}
			res := doRequest("foo", params, map[string]io.Reader{})

			Expect(mockClient.BuildImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(ContainSubstring("file is required"))
		})

		It("converts the name portion of the 'slug' form value to lowercase", func() {
			mockClient.InspectImageReturn = &freighterclient.Image{
				ID:   "cafebabe",
				Name: "scratcher",
				Tag:  "MyTag",
			}

			params := map[string]string{
				"slug": "scRaTchEr:MyTag",
				"code": "FROM fake",
			}
			doRequest("foo", params, map[string]io.Reader{})
			Expect(mockClient.BuildImageCalled).To(Equal(1))
			Expect(mockClient.BuildImageName).To(Equal("scratcher:MyTag"))
		})

		It("returns 500 on freighterclient error, removes image record and emits image build failed event", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			mockClient.BuildImageError = &freighterclient.StatusError{Code: 418}
			params := map[string]string{
				"slug": "scratcher",
				"code": "FROM scratch",
			}
			res := doRequest("foo", params, nil)

			Expect(mockClient.BuildImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))

			// removes image record
			i, err := image.Find(db, u, hs.H, "scratcher:mytag")
			Expect(err).To(Equal(sql.ErrNoRows), "expect the image to not exist in db, but got %v", i)

			// Read until we get a image build_failed event.
			var evt *controllers.Event
			for e := range ch {
				evt = e.(*controllers.Event)
				if evt.Name == controllers.EventImageBuildFailed {
					break
				}
			}
			Expect(evt.Name).To(Equal(controllers.EventImageBuildFailed))
			img := evt.Data.(*serializers.Image)
			Expect(img.Slug).To(Equal("scratcher"))
			close(done)
		})

		type buildImageParams struct {
			filename string
			contents io.Reader
			code     string
			remote   string
		}

		itBuildsImageSuccessfully := func(b buildImageParams) {
			var (
				params map[string]string
				file   map[string]io.Reader
				res    *httptest.ResponseRecorder

				filename = b.filename
				contents = b.contents
				code     = b.code
				remote   = b.remote
			)

			BeforeEach(func() {
				responseBody := `
					{"stream":"Step 0 : FROM ubuntu:latest\n"}
					{"stream":" ---\u003e 4300eb9d3c8d\n"}
					{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
					{"stream":" ---\u003e Using cache\n"}
					{"stream":" ---\u003e 3a3ed758c370\n"}
					{"stream":"Step 2 : CMD /usr/bin/top\n"}
					{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
					{"stream":" ---\u003e 4b6188aebe39\n"}
					{"stream":"Removing intermediate container 36b1479cc2e4\n"}
					{"stream":"Successfully built 4b6188aebe39\n"}
			    `
				mockClient.OutputStream = bytes.NewBufferString(responseBody)
				mockClient.InspectImageReturn = &freighterclient.Image{
					ID:   "cafebabe",
					Name: "scratcher",
					Tag:  "mytag",
				}

				file = map[string]io.Reader{}
				if filename != "" && contents != nil {
					file[filename] = contents
				}

				params = map[string]string{
					"slug":   "scratcher:mytag",
					"name":   "scratcher v1 test",
					"code":   code,
					"remote": remote,
				}
			})

			Context("when async param is not given", func() {
				It("creates an image record", func() {
					res = doRequest("foo", params, file)
					Expect(res.Code).To(Equal(http.StatusOK))
					i, err := image.Find(db, u, hs.H, "scratcher:mytag")
					Expect(err).To(BeNil())
					Expect(i.UserID).To(Equal(u.ID))
					Expect(i.HostID).To(Equal(hs.H.ID))
					Expect(i.FreighterID).To(Equal("cafebabe"))
					Expect(i.Name).To(Equal("scratcher v1 test"))
					Expect(i.Slug).To(Equal("scratcher"))
					Expect(i.Tag).To(Equal("mytag"))
					Expect(i.CreatedAt).NotTo(BeNil())
					Expect(i.UpdatedAt).NotTo(BeNil())
				})

				It("responds with build progress and image", func() {
					res = doRequest("foo", params, file)
					Expect(res.Code).To(Equal(http.StatusOK))

					if remote != "" {
						Expect(mockClient.BuildImageRemote).To(Equal(remote))
					} else {
						sent, err := ioutil.ReadAll(mockClient.BuildImageReader)
						Expect(err).To(BeNil())
						// check if the image sent is a tarball
						Expect(isarchive.IsArchive(sent)).To(BeTrue())
					}
					Expect(mockClient.BuildImageCalled).To(Equal(1))

					Expect(res.Body).To(ContainSubstring(`{"stream":"Step 0 : FROM ubuntu:latest\n"}`), "failed to include progress data")

					// Keep parsing and ignore the progress data to get the
					// final image data.
					j := struct {
						Image serializers.Image `json:"image"`
					}{}
					scanner := bufio.NewScanner(res.Body)
					for scanner.Scan() {
						json.Unmarshal(scanner.Bytes(), &j)
						if j.Image.ID > 0 {
							break
						}
					}
					i, err := image.Find(db, u, hs.H, "scratcher:mytag")
					Expect(err).To(BeNil())
					serialized := *serializers.SerializeImage(i)
					Expect(j.Image.ID).To(Equal(serialized.ID))
					Expect(j.Image.UserID).To(Equal(serialized.UserID))
					Expect(j.Image.HostID).To(Equal(serialized.HostID))
					Expect(j.Image.FreighterID).To(Equal(serialized.FreighterID))
					Expect(j.Image.Name).To(Equal(serialized.Name))
					Expect(j.Image.Slug).To(Equal(serialized.Slug))
					Expect(j.Image.Tag).To(Equal(serialized.Tag))
					Expect(j.Image.Repo).To(Equal(serialized.Repo))
					Expect(j.Image.ActualSize).To(Equal(serialized.ActualSize))
					Expect(j.Image.VirtualSize).To(Equal(serialized.VirtualSize))
					Expect(j.Image.BuiltAt).To(Equal(serialized.BuiltAt))
					// Because time.Time equality sucks when timezones are involved.
					Expect(j.Image.CreatedAt.UnixNano()).To(Equal(serialized.CreatedAt.UnixNano()))
					Expect(j.Image.UpdatedAt.UnixNano()).To(Equal(serialized.UpdatedAt.UnixNano()))
				})

				It("publishes image building events and a final image built event", func(done Done) {
					ch := svr.Events.Subscribe(u.Topic())
					defer svr.Events.Unsubscribe(ch, u.Topic())

					res = doRequest("foo", params, file)
					Expect(res.Code).To(Equal(http.StatusOK))

					i, err := image.Find(db, u, hs.H, "scratcher:mytag")
					Expect(err).To(BeNil())

					e := (<-ch).(*controllers.Event)
					Expect(e.Name).To(Equal(controllers.EventImageBuilding))
					data := e.Data.(*controllers.ProgressEvent)
					Expect(data.ID).To(Equal(i.ID))
					Expect(data.Progress).To(Equal(`{"stream":"Step 0 : FROM ubuntu:latest\n"}`))

					// Read all the intermediate image building events.
					var evt *controllers.Event
					for e := range ch {
						evt = e.(*controllers.Event)
						if evt.Name == controllers.EventImageBuilt {
							break
						}
					}
					Expect(evt).To(Equal(&controllers.Event{
						Name: controllers.EventImageBuilt,
						Data: serializers.SerializeImage(i),
					}))
					close(done)
				})
			})

			Context("when async param is true", func() {
				BeforeEach(func() {
					params["async"] = "true"
				})

				It("returns 202 accepted and builds image asynchronously", func() {
					res = doRequest("foo", params, file)
					Expect(res.Code).To(Equal(http.StatusAccepted))

					j := struct {
						Image serializers.Image `json:"image"`
					}{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					i, err := image.Find(db, u, hs.H, "scratcher:mytag")
					Expect(err).To(BeNil())
					Expect(j.Image.UserID).To(Equal(u.ID))
					Expect(j.Image.HostID).To(Equal(hs.H.ID))
					Expect(j.Image.Name).To(Equal("scratcher v1 test"))
					Expect(j.Image.Slug).To(Equal("scratcher"))
					Expect(j.Image.Tag).To(Equal("mytag"))
					Expect(j.Image.CreatedAt).NotTo(BeNil())
					Expect(j.Image.UpdatedAt).NotTo(BeNil())

					Expect(j.Image.ID).To(Equal(i.ID))
					Expect(j.Image.BuiltAt).To(BeNil())

					Eventually(func() int {
						return mockClient.BuildImageCalled
					}).Should(Equal(1))
				})
			})
		}

		Context("when a tar archive containing a Dockerfile is given", func() {
			tmpdir := makeTempDir()
			defer os.RemoveAll(tmpdir)

			// create a sample Dockerfile
			dfPath := makeDockerfile(tmpdir)

			// create a tmp file for the tar archive
			f, err := ioutil.TempFile(tmpdir, "build-valid-tar")
			Expect(err).To(BeNil())
			f.Close()

			// create a tar archive containing Dockerfile
			err = makeTar(tmpdir, dfPath, f.Name())
			Expect(err).To(BeNil())

			buf, err := ioutil.ReadFile(f.Name())
			Expect(err).To(BeNil())

			itBuildsImageSuccessfully(buildImageParams{
				filename: "sample.tar",
				contents: bytes.NewBuffer(buf),
				code:     "",
				remote:   "",
			})
		})

		Context("when a plain Dockerfile is given", func() {
			tmpdir := makeTempDir()
			defer os.RemoveAll(tmpdir)

			// create a sample Dockerfile
			dfPath := makeDockerfile(tmpdir)

			buf, err := ioutil.ReadFile(dfPath)
			Expect(err).To(BeNil())

			itBuildsImageSuccessfully(buildImageParams{
				filename: "Dockerfile",
				contents: bytes.NewBuffer(buf),
				code:     "",
				remote:   "",
			})
		})

		Context("when Dockerfile is given in code parameter", func() {
			tmpdir := makeTempDir()
			defer os.RemoveAll(tmpdir)

			// create a sample Dockerfile
			dfPath := makeDockerfile(tmpdir)

			buf, err := ioutil.ReadFile(dfPath)
			Expect(err).To(BeNil())

			itBuildsImageSuccessfully(buildImageParams{
				filename: "",
				contents: nil,
				code:     string(buf),
				remote:   "",
			})
		})

		Context("when a remote parameter is given", func() {
			itBuildsImageSuccessfully(buildImageParams{
				filename: "",
				contents: nil,
				code:     "",
				remote:   "git://github.com/foo/example.git",
			})
		})

		Context("when there's an exisiting image with the slug", func() {
			var i *image.Image

			BeforeEach(func() {
				i = &image.Image{
					UserID:      u.ID,
					HostID:      hs.H.ID,
					FreighterID: "cafebabe",
					Name:        "scratch v1",
					Slug:        "scratcher",
					Tag:         "mytag",
				}
				err := i.Insert(db)
				Expect(err).To(BeNil(), "failed to insert test image")
			})

			It("updates the existing image record with the new record", func() {
				responseBody := `
					{"stream":"Step 0 : FROM ubuntu:latest\n"}
					{"stream":" ---\u003e 4300eb9d3c8d\n"}
					{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
					{"stream":" ---\u003e Using cache\n"}
					{"stream":" ---\u003e 3a3ed758c370\n"}
					{"stream":"Step 2 : CMD /usr/bin/top\n"}
					{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
					{"stream":" ---\u003e 4b6188aebe39\n"}
					{"stream":"Removing intermediate container 36b1479cc2e4\n"}
					{"stream":"Successfully built 4b6188aebe39\n"}
			    `
				mockClient.OutputStream = bytes.NewBufferString(responseBody)
				mockClient.InspectImageReturn = &freighterclient.Image{
					ID:   "cafebabe2",
					Name: "scratcher",
					Tag:  "mytag",
				}

				params := map[string]string{
					"slug":   "scratcher:mytag",
					"name":   "scratcher v1 test",
					"code":   "",
					"remote": "git://github.com/foo/example.git",
				}
				res := doRequest("foo", params, nil)
				Expect(res.Code).To(Equal(http.StatusOK))

				i2, err := image.Find(db, u, hs.H, "scratcher:mytag")
				Expect(err).To(BeNil(), "failed to find stored image after build")
				Expect(i2.ID).To(Equal(i.ID))
				Expect(i2.FreighterID).To(Equal("cafebabe2"))
				Expect(i2.UpdatedAt).NotTo(Equal(i.UpdatedAt))
			})

			It("does not update the existing image if build fails", func() {
				mockClient.BuildImageError = &freighterclient.StatusError{Code: 418}
				params := map[string]string{
					"slug":   "scratcher:mytag",
					"name":   "scratcher v1 test",
					"code":   "",
					"remote": "git://github.com/foo/example.git",
				}
				res := doRequest("foo", params, nil)

				Expect(mockClient.BuildImageCalled).To(Equal(1))
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("internal_server_error"))

				i2, err := image.Find(db, u, hs.H, "scratcher:mytag")
				Expect(err).To(BeNil())
				Expect(i2.ID).To(Equal(i.ID))
				Expect(i2.FreighterID).To(Equal("cafebabe"))
				Expect(i2.UpdatedAt).To(Equal(i.UpdatedAt))
			})
		})
	})

	Describe("DELETE /api/v0/hosts/{slug}/image/{image}", func() {
		var i *image.Image
		BeforeEach(func() {
			i = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Slug:        "nitrousio/elixir",
				Tag:         "latest",
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(host, image string) *httptest.ResponseRecorder {
			req, err := NewAuthedRequest(t, "DELETE", fmt.Sprintf("/api/v0/hosts/%v/images/%v", host, image), nil)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("calls RemoveImage", func() {
			doRequest("foo", "nitrousio/elixir")
			Expect(mockClient.RemoveImageCalled).To(Equal(1))
		})

		It("deletes image record from the database", func() {
			_, err := image.Find(db, hs.U, hs.H, "nitrousio/elixir")
			Expect(err).To(BeNil())

			res := doRequest("foo", "nitrousio/elixir")
			Expect(res.Code).To(Equal(http.StatusOK))

			_, err = image.Find(db, hs.U, hs.H, "nitrousio/elixir")
			Expect(err).To(MatchError(sql.ErrNoRows))
		})

		It("publishes an image removed event with the serialized image", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			res := doRequest("foo", "nitrousio/elixir")
			Expect(res.Code).To(Equal(http.StatusOK))

			Expect(<-ch).To(Equal(&controllers.Event{
				Name: controllers.EventImageRemoved,
				Data: serializers.SerializeImage(i),
			}))
			close(done)
		})

		Context("when trying to delete an image that does not exist in the database", func() {
			It("returns HTTP 400", func() {
				res := doRequest("foo", "nonexistent")
				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("image does not exist"))
			})
		})

		Context("when image is in use", func() {
			BeforeEach(func() {
				mockClient.RemoveImageError = freighterclient.ErrImageInUse
			})

			It("returns HTTP 400", func() {
				res := doRequest("foo", "nitrousio/elixir")
				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(ContainSubstring("image is being used"))
			})
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/{image}/push", func() {
		var i *image.Image
		BeforeEach(func() {
			i = &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Slug:        "nitrousio/devenv",
				Tag:         "latest",
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(host, image string, params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/%s/push", host, image), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("calls PushImage", func() {
			res := doRequest("foo", "nitrousio/devenv:latest", &url.Values{"repository": []string{"myuser/scratch"}})
			Expect(mockClient.PushImageCalled).To(Equal(1))
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("publishes an image pushed event with the image slug and repository", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("foo", "nitrousio/devenv:latest", &url.Values{"repository": []string{"myuser/scratch"}})

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventImagePushed))
			Expect(evt.Data).To(BeEquivalentTo(struct {
				Image      string `json:"image"`
				Repository string `json:"repository"`
			}{
				Image:      "nitrousio/devenv:latest",
				Repository: "myuser/scratch",
			}))
			close(done)
		})

		It("publishes an image pushed event with the image ID and repository", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("foo", strconv.FormatInt(i.ID, 10), &url.Values{"repository": []string{"myuser/scratch"}})

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventImagePushed))
			Expect(evt.Data).To(BeEquivalentTo(struct {
				Image      string `json:"image"`
				Repository string `json:"repository"`
			}{
				Image:      "nitrousio/devenv:latest",
				Repository: "myuser/scratch",
			}))
			close(done)
		})

		It("requires repository param", func() {
			res := doRequest("foo", "devenv", nil)
			Expect(mockClient.PushImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("repository is required"))
		})

		It("returns 404 if the image could not be located", func() {
			mockClient.PushImageError = freighterclient.ErrImagePushAuthRequired
			res := doRequest("foo", "nitrousio/devenv:1203", &url.Values{"repository": []string{"scratch"}})
			Expect(res.Code).To(Equal(http.StatusNotFound))
			Expect(mockClient.PushImageCalled).To(Equal(0))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("not_found"))
		})

		It("returns HTTP 400 & registry_auth_failed if freighterclient returns an ErrImagePushAuthRequired", func() {
			mockClient.PushImageError = freighterclient.ErrImagePushAuthRequired
			res := doRequest("foo", "nitrousio/devenv:latest", &url.Values{"repository": []string{"scratch"}})
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			Expect(mockClient.PushImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("registry_auth_failed"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.PushImageError = &freighterclient.StatusError{Code: 418}
			res := doRequest("foo", "nitrousio/devenv:latest", &url.Values{"repository": []string{"scratch"}})
			Expect(res.Code).To(Equal(http.StatusInternalServerError))
			Expect(mockClient.PushImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})
	})

	Describe("POST /api/v0/hosts/{slug}/images/{image}/tag", func() {
		BeforeEach(func() {
			i := &image.Image{
				UserID:      hs.U.ID,
				HostID:      hs.H.ID,
				FreighterID: "cafebabe",
				Name:        "scratch v1",
				Slug:        "nitrousio/scratch",
				Tag:         "latest",
			}
			err := i.Insert(db)
			Expect(err).To(BeNil())
		})

		doRequest := func(host, image string, params *url.Values) *httptest.ResponseRecorder {
			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/hosts/%v/images/%s/tag", host, image), params)
			Expect(err).To(BeNil())
			res := httptest.NewRecorder()
			svr.Router().ServeHTTP(res, req)
			return res
		}

		It("creates an image record", func() {
			i, err := image.Find(db, u, hs.H, "staging")
			Expect(err).To(MatchError(sql.ErrNoRows))

			res := doRequest("foo", "nitrousio/scratch", &url.Values{
				"tag":  []string{"staging"},
				"name": []string{"staging v1 from scratch"},
			})
			Expect(res.Code).To(Equal(http.StatusOK))

			i, err = image.Find(db, u, hs.H, "staging")
			Expect(err).To(BeNil())
			Expect(i.FreighterID).To(Equal("cafebabe"))
			Expect(i.Name).To(Equal("staging v1 from scratch"))
			Expect(i.Slug).To(Equal("staging"))
			Expect(i.Tag).To(Equal("latest"))
		})

		Context("when image already exists", func() {
			BeforeEach(func() {
				i := &image.Image{
					UserID:      hs.U.ID,
					HostID:      hs.H.ID,
					FreighterID: "deadbeef",
					Name:        "staging image",
					Slug:        "staging", // We'll attempt to tag a new image with this image's repo.
					Tag:         "latest",
				}
				err := i.Insert(db)
				Expect(err).To(BeNil())
			})

			It("updates the existing image record with the source image", func() {
				res := doRequest("foo", "nitrousio/scratch", &url.Values{
					"tag":  []string{"staging"},
					"name": []string{"staging v1 from scratch"},
				})
				Expect(res.Code).To(Equal(http.StatusOK))

				i, err := image.Find(db, u, hs.H, "staging")
				Expect(err).To(BeNil())
				Expect(i.FreighterID).To(Equal("cafebabe"))
				Expect(i.Name).To(Equal("staging v1 from scratch"))
				Expect(i.Slug).To(Equal("staging"))
				Expect(i.Tag).To(Equal("latest"))
			})
		})

		It("calls TagImage", func() {
			res := doRequest("foo", "nitrousio/scratch", &url.Values{"tag": []string{"staging"}})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.ReceivedImage).To(Equal("nitrousio/scratch:latest"))
			Expect(mockClient.ReceivedTag).To(Equal("staging"))
			Expect(mockClient.TagImageForce).To(BeFalse())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("publishes an image tagged event with the image ID/name and tag", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("foo", "nitrousio/scratch:latest", &url.Values{"tag": []string{"staging"}})

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventImageTagged))
			img, ok := evt.Data.(*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(img.FreighterID).To(Equal("cafebabe"))
			Expect(img.Slug).To(Equal("staging"))
			Expect(img.Tag).To(Equal("latest"))
			close(done)
		})

		It("sends force param", func() {
			res := doRequest("foo", "nitrousio/scratch", &url.Values{"tag": []string{"staging"}, "force": []string{"1"}})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.TagImageForce).To(BeTrue())
			Expect(res.Code).To(Equal(http.StatusOK))
		})

		It("requires tag param", func() {
			res := doRequest("foo", "dontcare", nil)
			Expect(mockClient.TagImageCalled).To(Equal(0))
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("tag is required"))
		})

		It("converts the name portion of the 'tag' form value to lowercase", func() {
			params := &url.Values{}
			params.Add("tag", "NitrousIO/Staging:MyTag")
			doRequest("foo", "nitrousio/scratch", params)

			Expect(mockClient.TagImageCalled).To(Equal(1))
			Expect(mockClient.ReceivedTag).To(Equal("nitrousio/staging:MyTag"))
		})

		It("returns 500 on freighterclient error", func() {
			mockClient.TagImageError = &freighterclient.StatusError{Code: 418}
			res := doRequest("foo", "nitrousio/scratch", &url.Values{"tag": []string{"staging"}})
			Expect(mockClient.TagImageCalled).To(Equal(1))
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})

		Context("when trying to tag an image that does not exist in the database", func() {
			It("returns HTTP 400", func() {
				res := doRequest("foo", "nonexistent", &url.Values{"tag": []string{"staging"}})
				Expect(res.Code).To(Equal(http.StatusNotFound))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("not_found"))
				Expect(errDesc).To(Equal("image does not exist"))
			})
		})
	})
})
