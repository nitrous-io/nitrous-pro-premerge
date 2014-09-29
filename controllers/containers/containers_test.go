package containers_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/hmactoken"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/controllers/containers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/cvolume"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/containers")
}

var _ = Describe("controllers/containers", func() {
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

	Describe("WithContainerAndHost", func() {
		var (
			env           *middleware.Env
			handler       *mux.Router
			c             *container.Container
			handlerCalled bool
		)

		BeforeEach(func() {
			env = &middleware.Env{
				DB:   db,
				User: u,
			}

			h := func(w http.ResponseWriter, r *http.Request) {
				mw := containers.WithContainerAndHost(func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
					Expect(cn).NotTo(BeNil())
					Expect(h).NotTo(BeNil())
					Expect(h.State).To(Equal(host.StateRunning))
					handlerCalled = true
					return nil
				})
				err := mw(env, w, r)
				if err != nil {
					svr.HandleError(w, err)
				}
			}
			handler = mux.NewRouter()
			handler.HandleFunc("/{slug}/action", h).Methods("GET")
		})

		Context("when there is no container with the given slug", func() {
			It("returns a not found error", func() {
				req, err := NewRequest("GET", "/missing-container/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusNotFound))
				Expect(handlerCalled).To(BeFalse())

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("not_found"))
				errDesc, _ := j["error_description"].(string)
				Expect(errDesc).To(Equal("container does not exist"))
			})
		})

		Context("when a container with the given slug exists", func() {
			BeforeEach(func() {
				c = &container.Container{
					CID:    "beefface",
					Slug:   "rails-box",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())

				handlerCalled = false
			})

			It("runs the wrapped handler", func() {
				req, err := NewRequest("GET", "/rails-box/action", nil)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)

				Expect(res.Code).To(Equal(http.StatusOK))
				Expect(handlerCalled).To(BeTrue())
			})
		})
	})

	Describe("GET /api/v0/containers", func() {
		var (
			c         *container.Container
			fc        *freighterclient.Container
			touchedAt time.Time

			startedAt = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func(token, host string) {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/containers?host=%s", host), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   "dominant-intercooler-42",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Rails app server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			touchedAt = time.Date(2013, time.November, 10, 0, 0, 0, 0, time.UTC)
			c.TouchedAt = &touchedAt
			err = c.Update(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       "dominant-intercooler-42",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		It("calls the Freighter client to list containers", func() {
			doRequest("valid-token", "foo")
			Expect(mockClient.ListCalled).To(Equal(1))
		})

		It("responds with the containers from Freighter and augments them with data from the database", func() {
			doRequest("valid-token", "foo")

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Containers []*serializers.Container `json:"containers"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Containers).To(HaveLen(1))
			Expect(j.Containers[0].ID).To(Equal(c.ID))
			Expect(j.Containers[0].CID).To(Equal("deadbeef"))
			Expect(j.Containers[0].UserID).To(Equal(u.ID))
			Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
			Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
			Expect(j.Containers[0].IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Containers[0].GatewayIP).To(Equal("1.2.3.0"))
			Expect(j.Containers[0].State).To(Equal("running"))
			Expect(j.Containers[0].Name).To(Equal("Rails app server"))
			Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
			Expect(j.Containers[0].Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(j.Containers[0].Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(j.Containers[0].ExitCode).To(Equal(0))
			Expect(j.Containers[0].StartedAt).To(Equal(&startedAt))
			Expect(j.Containers[0].FinishedAt).To(BeNil())
			Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
			Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
			Expect(j.Containers[0].TerminatedAt).To(BeNil())
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.ListError = errors.New("you are not Chuck Norris")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("internal_server_error"))
			})
		})

		Context("when containers from Freighter differs from data in the database", func() {
			BeforeEach(func() {
				fc.Image = "new-image"
				fc.Name = "new-name-42"
				fc.Host = "2.4.6.8"
				fc.State = "stopped"
				mockClient.Containers = []*freighterclient.Container{fc}
			})

			It("prefers the data in the Freighter response", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].Image).To(Equal("new-image"))
				Expect(j.Containers[0].Slug).To(Equal("new-name-42"))
				Expect(j.Containers[0].IPAddress).To(Equal("2.4.6.8"))
				Expect(j.Containers[0].State).To(Equal("stopped"))
			})
		})

		Context("when there are more containers from Freighter than in the database", func() {
			BeforeEach(func() {
				fc2 := &freighterclient.Container{
					ID:         "cafebabe",
					Name:       "wicked-baby-69",
					Host:       "1.2.3.5",
					Gateway:    "1.2.3.0",
					State:      "running",
					Image:      "nitrous-base",
					Cmd:        []string{"/sbin/init"},
					Entrypoint: []string{"/bin/blah"},
					ExitCode:   0,
					StartedAt:  startedAt,
					FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
				}
				mockClient.Containers = []*freighterclient.Container{fc, fc2}
			})

			It("responds with all the containers from Freighter", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(2))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
				Expect(j.Containers[0].IPAddress).To(Equal("1.2.3.4"))
				Expect(j.Containers[0].GatewayIP).To(Equal("1.2.3.0"))
				Expect(j.Containers[0].State).To(Equal("running"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].Cmd).To(Equal([]string{"/sbin/init"}))
				Expect(j.Containers[0].Entrypoint).To(Equal([]string{"/bin/blah"}))
				Expect(j.Containers[0].ExitCode).To(Equal(0))
				Expect(j.Containers[0].StartedAt).To(Equal(&startedAt))
				Expect(j.Containers[0].FinishedAt).To(BeNil())
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())

				Expect(j.Containers[1].ID).To(Equal(int64(0))) // Container exists in Freighter but not in DB.
				Expect(j.Containers[1].CID).To(Equal("cafebabe"))
				Expect(j.Containers[1].UserID).To(Equal(u.ID))
				Expect(j.Containers[1].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[1].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[1].Slug).To(Equal("wicked-baby-69"))
				Expect(j.Containers[1].IPAddress).To(Equal("1.2.3.5"))
				Expect(j.Containers[1].GatewayIP).To(Equal("1.2.3.0"))
				Expect(j.Containers[1].State).To(Equal("running"))
				Expect(j.Containers[1].Name).To(Equal(""))
				Expect(j.Containers[1].Cmd).To(Equal([]string{"/sbin/init"}))
				Expect(j.Containers[1].Entrypoint).To(Equal([]string{"/bin/blah"}))
				Expect(j.Containers[1].ExitCode).To(Equal(0))
				Expect(j.Containers[1].StartedAt).To(Equal(&startedAt))
				Expect(j.Containers[1].FinishedAt).To(BeNil())
				Expect(j.Containers[1].CreatedAt).To(BeNil())
				Expect(j.Containers[1].UpdatedAt).To(BeNil())
				Expect(j.Containers[1].TerminatedAt).To(BeNil())
			})
		})

		Context("when host has no containers", func() {
			BeforeEach(func() {
				mockClient.Containers = []*freighterclient.Container{}
			})

			It("responds with an empty array", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))
				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())
				containers, ok := j["containers"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(containers).To(BeEmpty())
			})
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("should return containers belonging to the host (without the freighter-hydrated details)", func() {
				doRequest("valid-token", "foo")

				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())
				// ip address, gateway will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateHostStopped.String()))

				Expect(j.Containers[0].Cmd).To(BeNil())
				Expect(j.Containers[0].Entrypoint).To(BeNil())
				Expect(j.Containers[0].ExitCode).To(Equal(0))
				Expect(j.Containers[0].StartedAt).To(BeNil())
				Expect(j.Containers[0].FinishedAt).To(BeNil())
			})
		})

		Context("when host IP address is not available", func() {
			var c1 *container.Container
			var hs1 *HostStack
			var err error

			BeforeEach(func() {
				hs1, err = CreateHostStack(db, &HostStack{
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

				c1 = &container.Container{
					CID:    "cafedead",
					Slug:   "forever-container-21",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs1.H.ID,
					Name:   sql.NullString{"Rails app server", true},
				}
				err = c1.Insert(db)
				Expect(err).To(BeNil())

				// test the actual client
				UnstubFreighterFn()
			})

			It("should return all containers belonging to the host (without the freighter-hydrated details)", func() {
				doRequest("valid-token", "bar")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(1))

				Expect(j.Containers[0].ID).To(Equal(c1.ID))
				Expect(j.Containers[0].CID).To(Equal("cafedead"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs1.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("forever-container-21"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())
				// ip address, gateway will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateHostStopped.String()))

				Expect(j.Containers[0].Cmd).To(BeNil())
				Expect(j.Containers[0].Entrypoint).To(BeNil())
				Expect(j.Containers[0].ExitCode).To(Equal(0))
				Expect(j.Containers[0].StartedAt).To(BeNil())
				Expect(j.Containers[0].FinishedAt).To(BeNil())
			})
		})

		Context("when host param is not provided", func() {
			var c1 *container.Container
			var hs1 *HostStack
			var err error

			BeforeEach(func() {
				hs1, err = CreateHostStack(db, &HostStack{
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

				c1 = &container.Container{
					CID:    "cafedead",
					Slug:   "forever-container-21",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs1.H.ID,
					Name:   sql.NullString{"Rails app server", true},
				}
				err = c1.Insert(db)
				Expect(err).To(BeNil())

				// We are updating the state here to be stopped
				// so that there are no Freighter API calls
				// the way we stub the Freighter API right now
				// doesnt' allow us to get different responses
				// for different API requests (similar to stub chain)
				hs.H.State = host.StateStopped
				err = hs.H.Update(db)
				Expect(err).To(BeNil())

				hs1.H.State = host.StateStopped
				err = hs1.H.Update(db)
				Expect(err).To(BeNil())

				UnstubFreighterFn()
			})

			It("should return all containers belonging to the users' hosts", func() {
				doRequest("valid-token", "")
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Containers []*serializers.Container `json:"containers"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Containers).To(HaveLen(2))

				Expect(j.Containers[0].ID).To(Equal(c.ID))
				Expect(j.Containers[0].CID).To(Equal("deadbeef"))
				Expect(j.Containers[0].UserID).To(Equal(u.ID))
				Expect(j.Containers[0].HostID).To(Equal(hs.H.ID))
				Expect(j.Containers[0].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[0].Slug).To(Equal("dominant-intercooler-42"))
				Expect(j.Containers[0].Name).To(Equal("Rails app server"))
				Expect(j.Containers[0].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[0].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[0].TerminatedAt).To(BeNil())
				// ip address, gateway will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[0].IPAddress).To(Equal(""))
				Expect(j.Containers[0].GatewayIP).To(Equal(""))
				Expect(j.Containers[0].State).To(Equal(container.StateHostStopped.String()))

				Expect(j.Containers[1].ID).To(Equal(c1.ID))
				Expect(j.Containers[1].CID).To(Equal("cafedead"))
				Expect(j.Containers[1].UserID).To(Equal(u.ID))
				Expect(j.Containers[1].HostID).To(Equal(hs1.H.ID))
				Expect(j.Containers[1].Image).To(Equal("nitrous-base"))
				Expect(j.Containers[1].Slug).To(Equal("forever-container-21"))
				Expect(j.Containers[1].Name).To(Equal("Rails app server"))
				Expect(j.Containers[1].CreatedAt).NotTo(BeNil())
				Expect(j.Containers[1].UpdatedAt).NotTo(BeNil())
				Expect(j.Containers[1].TerminatedAt).To(BeNil())
				// ip address, gateway state will be empty since the host is stopped,
				// so the freighter API call didn't happen
				Expect(j.Containers[1].IPAddress).To(Equal(""))
				Expect(j.Containers[1].GatewayIP).To(Equal(""))
				Expect(j.Containers[1].State).To(Equal(container.StateHostStopped.String()))
			})
		})

		Context("when host does not exist for user", func() {
			It("returns HTTP 404 Not Found", func() {
				doRequest("valid-token", "no-such-host")

				Expect(res.Code).To(Equal(http.StatusNotFound))

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
				authTokenInputs.Path = "/api/v0/containers"
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /api/v0/containers", func() {
		var params *url.Values

		BeforeEach(func() {
			params = &url.Values{}
			params.Set("slug", "elixir-web-1")
			params.Set("name", "Elixir Web Server 1")
			params.Set("image", "elixir")
			params.Add("env[]", "LANG=C")
			params.Add("env[]", "NODE=nope")
			params.Add("port[]", "3000:8080")
			params.Add("port[]", "0.0.0.0::4552")
			params.Add("port[]", "0.0.0.0:8081:11211")

			mockClient.CreatedContainer = &freighterclient.Container{
				ID:      "deadbeef",
				Name:    "elixir-web-1",
				Host:    "",
				Gateway: "",
				State:   "running",
				Image:   "elixir",
				ImageID: "abc123",
			}
		})

		doRequest := func(token, host string, params *url.Values) {
			if params == nil {
				params = &url.Values{}
			}
			params.Set("host", host)
			req, err := NewFormRequest("POST", "/api/v0/containers", params)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		expectSerializedContainer := func(cn *serializers.Container, c *container.Container) {
			Expect(cn.ID).To(Equal(c.ID))
			Expect(cn.CID).To(Equal("deadbeef"))
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.HostID).To(Equal(hs.H.ID))
			Expect(cn.Image).To(Equal("elixir"))
			Expect(cn.Slug).To(Equal("elixir-web-1"))
			Expect(cn.IPAddress).To(Equal(""))
			Expect(cn.GatewayIP).To(Equal(""))
			Expect(cn.State).To(Equal(container.StateRunning.String()))
			Expect(cn.Name).To(Equal("Elixir Web Server 1"))
		}

		It("calls the Freighter client to create a container", func() {
			mockClient.CreateError = errors.New("create failed")

			doRequest("valid-token", "foo", params)

			Expect(mockClient.CreateCalled).To(Equal(1))
			Expect(mockClient.ReceivedName).To(Equal("elixir-web-1"))
			Expect(mockClient.ReceivedImage).To(Equal("elixir"))
			conf := mockClient.ReceivedConfig
			Expect(conf.Env).To(ConsistOf("LANG=C", "NODE=nope"))
			Expect(conf.PortBindings).To(ConsistOf("3000:8080", "0.0.0.0::4552", "0.0.0.0:8081:11211"))
		})

		It("creates a new container record", func() {
			doRequest("valid-token", "foo", params)
			c, err := container.Find(db, u, "elixir-web-1")
			Expect(err).To(BeNil())
			Expect(c.Slug).To(Equal("elixir-web-1"))
			Expect(c.Image).To(Equal("elixir"))
			Expect(c.Name.String).To(Equal("Elixir Web Server 1"))
			Expect(mockClient.ReceivedSecret).To(Equal(c.Secret))
		})

		It("converts the 'slug' form value to lowercase", func() {
			params.Del("slug")
			params.Set("slug", "Elixir-WEB-1")

			doRequest("valid-token", "foo", params)
			Expect(mockClient.CreateCalled).To(Equal(1))
			Expect(mockClient.ReceivedName).To(Equal("elixir-web-1"))

			c, err := container.Find(db, u, "elixir-web-1")
			Expect(err).To(BeNil())
			Expect(c.Slug).To(Equal("elixir-web-1"))
		})

		It("responds with HTTP 201 Created and the container", func() {
			doRequest("valid-token", "foo", params)

			Expect(res.Code).To(Equal(http.StatusCreated))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			c, err := container.Find(db, u, "elixir-web-1")
			Expect(err).To(BeNil())
			expectSerializedContainer(j.Container, c)
		})

		It("publishes a container created event with the created container", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("valid-token", "foo", params)

			var evt *controllers.Event
			for msg := range ch { // Ignore all events except container created.
				e, ok := msg.(*controllers.Event)
				if ok && e.Name == controllers.EventContainerCreated {
					evt = e
					break
				}
			}
			Expect(evt.Name).To(Equal(controllers.EventContainerCreated))
			cn, ok := evt.Data.(*serializers.Container)
			Expect(ok).To(BeTrue())
			c, err := container.Find(db, u, "elixir-web-1")
			Expect(err).To(BeNil())
			expectSerializedContainer(cn, c)

			close(done)
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.CreateError = errors.New("only Chuck Norris is allowed")
			})

			It("does not create a new container record", func() {
				doRequest("valid-token", "foo", params)
				c, err := container.Find(db, u, "elixir-web-1")
				Expect(err).To(BeNil())
				Expect(c).To(BeNil())
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusInternalServerError))
				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("internal_server_error"))
			})

			Context("when the error is a StartFailedError", func() {
				BeforeEach(func() {
					mockClient.CreateError = &freighterclient.StartFailedError{"start failed: could not bind port 22"}
				})

				It("responds with HTTP 201 Created with both the container and the error", func() {
					doRequest("valid-token", "foo", params)

					Expect(res.Code).To(Equal(http.StatusCreated))

					var j struct {
						Container        *serializers.Container `json:"container"`
						Error            string                 `json:"error"`
						ErrorDescription string                 `json:"error_description"`
					}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())

					Expect(j.Error).To(Equal("start_failed"))
					Expect(j.ErrorDescription).To(ContainSubstring("start failed: could not bind port 22"))

					c, err := container.Find(db, u, "elixir-web-1")
					Expect(err).To(BeNil())
					expectSerializedContainer(j.Container, c)
				})
			})

			Context("when the error is an InvalidParamError", func() {
				BeforeEach(func() {
					mockClient.CreateError = &freighterclient.InvalidParamError{"invalid name"}
				})

				It("responds with HTTP 400 Bad Request", func() {
					doRequest("valid-token", "foo", params)

					Expect(res.Code).To(Equal(http.StatusBadRequest))
					errType, _ := ErrorSlug(res.Body)
					Expect(errType).To(Equal("validation_failed"))
				})
			})

			Context("when the error is an ErrImageNotFound", func() {
				BeforeEach(func() {
					mockClient.CreateError = freighterclient.ErrImageNotFound
				})

				It("responds with HTTP 404 Not Found", func() {
					doRequest("valid-token", "foo", params)

					Expect(res.Code).To(Equal(http.StatusNotFound))
					errType, errDesc := ErrorSlug(res.Body)
					Expect(errType).To(Equal("not_found"))
					Expect(errDesc).To(Equal("image does not exist"))
				})
			})
		})

		Context("when name is already taken", func() {
			BeforeEach(func() {
				c := &container.Container{
					CID:    "facecafe",
					Slug:   "elixir-web-1",
					Image:  "nitrous-base",
					UserID: u.ID,
					HostID: hs.H.ID,
				}
				err := c.Insert(db)
				Expect(err).To(BeNil())
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("validation_failed"))
				Expect(errDesc).To(Equal("slug is taken"))
			})
		})

		Context("when slug param is not given", func() {
			BeforeEach(func() {
				params.Del("slug")
			})

			It("responds with HTTP 400 Bad Request", func() {
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("invalid_request"))
				Expect(errDesc).To(Equal("slug is required"))
			})
		})

		Context("when image param is not given", func() {
			BeforeEach(func() {
				params.Del("image")

				mockClient.CreatedContainer = &freighterclient.Container{
					ID:    "deadbeef",
					Name:  "freighter.elixir-web-1",
					Host:  "1.2.3.4",
					State: "running",
					Image: controllers.DefaultImage,
				}
			})

			It("uses the default image", func() {
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusCreated))

				var j struct {
					Container *serializers.Container `json:"container"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				Expect(j.Container.Image).To(Equal(controllers.DefaultImage))
			})
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
			})

			It("returns HTTP 503 Service Unavailable", func() {
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusServiceUnavailable))
				errType, errDesc := ErrorSlug(res.Body)
				Expect(errType).To(Equal("service_unavailable"))
				Expect(errDesc).To(Equal("host is not running"))
			})
		})

		Context("with volumes param", func() {
			BeforeEach(func() {
				params.Add("volume[]", "music:/mpd/albums")
				params.Add("volume[]", "backups")
			})

			It("returns success if volume does exist", func() {
				v := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
				err := v.Insert(db)
				Expect(err).To(BeNil())
				v2 := &volume.Volume{HostID: hs.H.ID, Slug: "backups"}
				err = v2.Insert(db)
				Expect(err).To(BeNil())
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusCreated))
				var j struct {
					Status    string                 `json:"status"`
					Container *serializers.Container `json:"container"`
				}
				err = json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())
				conf := mockClient.ReceivedConfig
				Expect(conf.Mounts).To(HaveLen(2))
				Expect(conf.Mounts).To(ContainElement("music:/mpd/albums"))
				Expect(conf.Mounts).To(ContainElement("backups"))
				cv, err := cvolume.Find(db, &container.Container{ID: j.Container.ID}, v)
				Expect(err).To(BeNil())
				Expect(cv.VolumeID).To(Equal(v.ID))
				cv2, err := cvolume.Find(db, &container.Container{ID: j.Container.ID}, v2)
				Expect(err).To(BeNil())
				Expect(cv2.VolumeID).To(Equal(v2.ID))
			})

			It("returns error if volume does not exist", func() {
				doRequest("valid-token", "foo", params)

				Expect(res.Code).To(Equal(http.StatusBadRequest))
				_, errDesc := ErrorSlug(res.Body)
				Expect(errDesc).To(ContainSubstring("volume not found"))
			})

		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/containers"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /containers/{slug}/commit", func() {
		doRequest := func(name string) {
			params := &url.Values{}
			if name != "" {
				params.Set("name", name)
			}
			req, err := NewAuthedFormRequest(t, "POST",
				"/api/v0/containers/elixir-web-1/commit",
				params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c := &container.Container{
				CID:    "deadbeef",
				Slug:   "elixir-web-1",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			mockClient.CommitImage = &freighterclient.Image{
				ID:          "1a",
				Name:        "elixir-staging",
				Tag:         "latest",
				CreatedAt:   1903,
				ActualSize:  13,
				VirtualSize: 29,
			}

			fc := &freighterclient.Container{
				ID:         "deadbeef",
				Name:       "elixir-web-1",
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				StartedAt:  time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC),
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		It("creates a new image record", func() {
			doRequest("elixir-staging")

			i, err := image.Find(db, u, hs.H, "elixir-staging:latest")
			Expect(err).To(BeNil())
			Expect(i.UserID).To(Equal(u.ID))
			Expect(i.HostID).To(Equal(hs.H.ID))
			Expect(i.FreighterID).To(Equal("1a"))
			Expect(i.Slug).To(Equal("elixir-staging"))
			Expect(i.Tag).To(Equal("latest"))
		})

		It("returns image and container on success", func() {
			doRequest("elixir-staging")
			Expect(res.Code).To(Equal(http.StatusOK))
			Expect(mockClient.CommitCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
			Expect(mockClient.ReceivedName).To(Equal("elixir-staging"))
			var j struct {
				Image     *serializers.Image     `json:"image"`
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			Expect(j.Image).NotTo(BeNil())
			Expect(j.Image.Slug).To(Equal("elixir-staging"))
			Expect(j.Image.Tag).To(Equal("latest"))
			Expect(j.Container).NotTo(BeNil())
			Expect(j.Container.Slug).To(Equal("elixir-web-1"))
			Expect(j.Container.State).To(Equal("running"))
		})

		It("returns a server error when freighter fails", func() {
			mockClient.InfoError = errors.New("you are not Chuck Norris")
			doRequest("elixir-staging")
			errType, _ := ErrorSlug(res.Body)
			Expect(errType).To(Equal("internal_server_error"))
		})

		It("requires name parameter", func() {
			doRequest("")
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("image name is required"))
		})

		It("handles image already exists error", func() {
			mockClient.CommitError = freighterclient.ErrImageExists
			doRequest("taken")
			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal("image already exists"))
		})

		It("publishes an image created event with the committed image", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("elixir-staging")

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventImageCreated))

			img, ok := evt.Data.(*serializers.Image)
			Expect(ok).To(BeTrue())
			Expect(img).NotTo(BeNil())
			Expect(img.Slug).To(Equal("elixir-staging"))
			Expect(img.Tag).To(Equal("latest"))

			close(done)
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/containers/elixir-web-1/commit"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("GET /containers/{slug}", func() {
		var (
			c         *container.Container
			fc        *freighterclient.Container
			slug      = "elixir-web-1"
			token     = ""
			startedAt = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func() {
			req, err := NewRequest("GET", fmt.Sprintf("/api/v0/containers/%s", slug), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			token = "valid-token"

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   slug,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       slug,
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "running",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   0,
				StartedAt:  startedAt,
				FinishedAt: time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC),
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		It("calls the Freighter client to get the container's info", func() {
			doRequest()
			Expect(mockClient.InfoCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Container.ID).To(Equal(c.ID))
			Expect(j.Container.CID).To(Equal("deadbeef"))
			Expect(j.Container.UserID).To(Equal(u.ID))
			Expect(j.Container.HostID).To(Equal(hs.H.ID))
			Expect(j.Container.Image).To(Equal("nitrous-base"))
			Expect(j.Container.Slug).To(Equal(slug))
			Expect(j.Container.Name).To(Equal("Elixir server"))
			Expect(j.Container.IPAddress).To(Equal("1.2.3.4"))
			Expect(j.Container.GatewayIP).To(Equal("1.2.3.0"))
			Expect(j.Container.State).To(Equal("running"))
			Expect(j.Container.Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(j.Container.Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(j.Container.ExitCode).To(Equal(0))
			Expect(j.Container.StartedAt).To(Equal(&startedAt))
			Expect(j.Container.FinishedAt).To(BeNil())
			Expect(j.Container.CreatedAt).NotTo(BeNil())
			Expect(j.Container.UpdatedAt).NotTo(BeNil())
			Expect(j.Container.TerminatedAt).To(BeNil())
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.InfoError = errors.New("you are not Chuck Norris")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("internal_server_error"))
			})
		})

		Context("when host is not running", func() {
			BeforeEach(func() {
				hs.H.State = host.StateStopped
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with the container with its state set to host_stopped", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Container *serializers.Container `json:"container"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.ID).To(Equal(c.ID))
				Expect(j.Container.CID).To(Equal("deadbeef"))
				Expect(j.Container.UserID).To(Equal(u.ID))
				Expect(j.Container.HostID).To(Equal(hs.H.ID))
				Expect(j.Container.Image).To(Equal("nitrous-base"))
				Expect(j.Container.Slug).To(Equal(slug))
				Expect(j.Container.Name).To(Equal("Elixir server"))
				Expect(j.Container.IPAddress).To(Equal(""))
				Expect(j.Container.GatewayIP).To(Equal(""))
				Expect(j.Container.State).To(Equal(container.StateHostStopped.String()))
				Expect(j.Container.ExitCode).To(Equal(0))
				Expect(j.Container.StartedAt).To(BeNil())
				Expect(j.Container.FinishedAt).To(BeNil())
				Expect(j.Container.CreatedAt).NotTo(BeNil())
				Expect(j.Container.UpdatedAt).NotTo(BeNil())
				Expect(j.Container.TerminatedAt).To(BeNil())
			})
		})

		Context("when host IP address is not available", func() {
			BeforeEach(func() {
				hs.H.IPAddress = sql.NullString{}
				err := hs.H.Update(db)
				Expect(err).To(BeNil())
				UnstubFreighterFn()
			})

			It("responds with the container with its state set to host_stopped", func() {
				doRequest()
				Expect(res.Code).To(Equal(http.StatusOK))

				var j struct {
					Container *serializers.Container `json:"container"`
				}
				err := json.NewDecoder(res.Body).Decode(&j)
				Expect(err).To(BeNil())

				Expect(j.Container.ID).To(Equal(c.ID))
				Expect(j.Container.CID).To(Equal("deadbeef"))
				Expect(j.Container.UserID).To(Equal(u.ID))
				Expect(j.Container.HostID).To(Equal(hs.H.ID))
				Expect(j.Container.Image).To(Equal("nitrous-base"))
				Expect(j.Container.Slug).To(Equal(slug))
				Expect(j.Container.Name).To(Equal("Elixir server"))
				Expect(j.Container.IPAddress).To(Equal(""))
				Expect(j.Container.GatewayIP).To(Equal(""))
				Expect(j.Container.State).To(Equal(container.StateHostStopped.String()))
				Expect(j.Container.ExitCode).To(Equal(0))
				Expect(j.Container.StartedAt).To(BeNil())
				Expect(j.Container.FinishedAt).To(BeNil())
				Expect(j.Container.CreatedAt).NotTo(BeNil())
				Expect(j.Container.UpdatedAt).NotTo(BeNil())
				Expect(j.Container.TerminatedAt).To(BeNil())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s", slug)
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	type controlTestInputs struct {
		action            string
		actionCounter     *int
		actionError       *error
		expectedState     string
		expectedEventName controllers.EventName
	}

	itBehavesLikeAControlAction := func(inputs *controlTestInputs) {
		var (
			c  *container.Container
			fc *freighterclient.Container

			slug       = "elixir-web-1"
			startedAt  = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			finishedAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
		)

		doRequest := func(slug string) {
			req, err := NewRequest("POST", fmt.Sprintf("/api/v0/containers/%s/%s", slug, inputs.action), nil)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c = &container.Container{
				CID:    "deadbeef",
				Slug:   slug,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       slug,
				Host:       "1.2.3.4",
				Gateway:    "1.2.3.0",
				State:      "stopped",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   126,
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		expectSerializedContainer := func(cn *serializers.Container) {
			Expect(cn.ID).To(Equal(c.ID))
			Expect(cn.CID).To(Equal("deadbeef"))
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.Image).To(Equal("nitrous-base"))
			Expect(cn.Slug).To(Equal(slug))
			Expect(cn.IPAddress).To(Equal("1.2.3.4"))
			Expect(cn.GatewayIP).To(Equal("1.2.3.0"))
			Expect(cn.State).To(Equal(inputs.expectedState))
			Expect(cn.Name).To(Equal("Elixir server"))
			Expect(cn.HostID).To(Equal(hs.H.ID))
			Expect(cn.Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(cn.Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(cn.ExitCode).To(Equal(126))
			Expect(cn.StartedAt).To(Equal(&startedAt))
			Expect(cn.FinishedAt).To(Equal(&finishedAt))
			Expect(cn.CreatedAt).NotTo(BeNil())
			Expect(cn.UpdatedAt).NotTo(BeNil())
			Expect(cn.TerminatedAt).To(BeNil())
		}

		It("calls the Freighter client to perform the control action", func() {
			doRequest("elixir-web-1")
			Expect(*inputs.actionCounter).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest("elixir-web-1")

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			expectSerializedContainer(j.Container)
		})

		It("publishes a container.{action} event with the container", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest("elixir-web-1")

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(inputs.expectedEventName))

			cn, ok := evt.Data.(*serializers.Container)
			Expect(ok).To(BeTrue())
			expectSerializedContainer(cn)

			close(done)
		})

		It("unlocks the container after the action", func() {
			doRequest("elixir-web-1")

			// check if the container is locakable
			expiresIn := time.Duration(5) * time.Second
			err := c.Lock(expiresIn, db)
			Expect(err).To(BeNil())
		})

		Context("when container is already locked", func() {
			It("responds with HTTP 409 Conflict", func() {
				expiresIn := time.Duration(5) * time.Second
				err := c.Lock(expiresIn, db)
				Expect(err).To(BeNil())

				doRequest("elixir-web-1")

				Expect(res.Code).To(Equal(http.StatusConflict))

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("container_locked"))
			})
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				*inputs.actionError = errors.New("control action failed")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest("elixir-web-1")

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("internal_server_error"))
			})

			It("unlocks the container", func() {
				doRequest("elixir-web-1")

				cntnr, err := container.FindByCID(db, "deadbeef")
				Expect(err).To(BeNil())
				Expect(cntnr.LockedAt).To(BeNil())
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/%s", slug, inputs.action)
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	}

	Describe("POST /api/v0/containers/{slug}/start", func() {
		var i = &controlTestInputs{}

		BeforeEach(func() {
			i.action = "start"
			i.actionCounter = &mockClient.StartCalled
			i.actionError = &mockClient.StartError
			i.expectedState = "running"
			i.expectedEventName = controllers.EventContainerStarted
		})

		itBehavesLikeAControlAction(i)

		It("calls the Freighter client to start the container", func() {
			c := &container.Container{
				CID:    "deadbead",
				Slug:   "foo",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
			mockClient.StartError = errors.New("start failed")

			params := &url.Values{}
			params.Add("env[]", "LANG=C")
			params.Add("env[]", "NODE=nope")
			params.Add("port[]", "3000:8080")
			params.Add("port[]", "0.0.0.0::4552")

			req, err := NewFormRequest("POST", "/api/v0/containers/foo/start", params)
			Expect(err).To(BeNil())
			req.Header.Set("Authorization", "Bearer valid-token")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			Expect(mockClient.StartCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbead"))
			conf := mockClient.ReceivedConfig
			Expect(conf.Env).To(ConsistOf("LANG=C", "NODE=nope"))
			Expect(conf.PortBindings).To(ConsistOf("3000:8080", "0.0.0.0::4552"))
		})
	})

	Describe("POST /api/v0/containers/{slug}/stop", func() {
		var i = &controlTestInputs{}

		BeforeEach(func() {
			i.action = "stop"
			i.actionCounter = &mockClient.StopCalled
			i.actionError = &mockClient.StopError
			i.expectedState = "stopped"
			i.expectedEventName = controllers.EventContainerStopped
		})

		itBehavesLikeAControlAction(i)
	})

	Describe("POST /api/v0/containers/{name}/restart", func() {
		var i = &controlTestInputs{}

		BeforeEach(func() {
			i.action = "restart"
			i.actionCounter = &mockClient.RestartCalled
			i.actionError = &mockClient.RestartError
			i.expectedState = "running"
			i.expectedEventName = controllers.EventContainerRestarted
		})

		itBehavesLikeAControlAction(i)
	})

	Describe("DELETE /containers/{slug}", func() {
		var (
			c          *container.Container
			fc         *freighterclient.Container
			slug       = "elixir-web-1"
			token      = ""
			startedAt  = time.Date(2014, time.August, 13, 0, 0, 0, 0, time.UTC)
			finishedAt = time.Date(2014, time.August, 14, 0, 0, 0, 0, time.UTC)
		)

		BeforeEach(func() {
			token = "valid-token"

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   slug,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())

			fc = &freighterclient.Container{
				ID:         "deadbeef",
				Name:       slug,
				Host:       "",
				Gateway:    "",
				State:      "",
				Image:      "nitrous-base",
				Cmd:        []string{"/sbin/init"},
				Entrypoint: []string{"/bin/blah"},
				ExitCode:   126,
				StartedAt:  startedAt,
				FinishedAt: finishedAt,
			}
			mockClient.Containers = []*freighterclient.Container{fc}
		})

		doRequest := func() {
			req, err := NewRequest("DELETE", fmt.Sprintf("/api/v0/containers/%s", slug), nil)
			Expect(err).To(BeNil())
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			handler.ServeHTTP(res, req)
		}

		expectSerializedContainer := func(cn *serializers.Container) {
			Expect(cn.ID).To(Equal(c.ID))
			Expect(cn.CID).To(Equal("deadbeef"))
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.Image).To(Equal("nitrous-base"))
			Expect(cn.Slug).To(Equal(slug))
			Expect(cn.Name).To(Equal("Elixir server"))
			Expect(cn.IPAddress).To(Equal(""))
			Expect(cn.GatewayIP).To(Equal(""))
			Expect(cn.State).To(Equal(container.StateTerminated.String()))
			Expect(cn.Cmd).To(Equal([]string{"/sbin/init"}))
			Expect(cn.Entrypoint).To(Equal([]string{"/bin/blah"}))
			Expect(cn.ExitCode).To(Equal(126))
			Expect(cn.StartedAt).To(Equal(&startedAt))
			Expect(cn.FinishedAt).To(Equal(&finishedAt))
			Expect(cn.CreatedAt).NotTo(BeNil())
			Expect(cn.UpdatedAt).NotTo(BeNil())
			Expect(cn.TerminatedAt).NotTo(BeNil())
		}

		It("calls the Freighter client to Terminate the container", func() {
			doRequest()
			Expect(mockClient.TerminateCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			expectSerializedContainer(j.Container)
		})

		It("deletes the container from the database", func() {
			doRequest()

			c, err := container.Find(db, hs.U, slug)
			Expect(err).To(BeNil())
			Expect(c).To(BeNil())
		})

		It("publishes a container destroyed event with the destroyed container", func(done Done) {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest()

			msg := <-ch
			evt, ok := msg.(*controllers.Event)
			Expect(ok).To(BeTrue())
			Expect(evt.Name).To(Equal(controllers.EventContainerDestroyed))

			cn, ok := evt.Data.(*serializers.Container)
			Expect(ok).To(BeTrue())
			expectSerializedContainer(cn)

			close(done)
		})

		Context("when container is locked", func() {
			It("responds with HTTP 409 Conflict", func() {
				expiresIn := time.Duration(5) * time.Second
				err := c.Lock(expiresIn, db)
				Expect(err).To(BeNil())

				doRequest()

				Expect(res.Code).To(Equal(http.StatusConflict))

				j := map[string]interface{}{}
				err = json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("container_locked"))
			})
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.TerminateError = errors.New("only Chuck Norris can terminate containers")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				j := map[string]interface{}{}
				err := json.Unmarshal(res.Body.Bytes(), &j)
				Expect(err).To(BeNil())

				errType, _ := j["error"].(string)
				Expect(errType).To(Equal("internal_server_error"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s", slug)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("POST /containers/{slug}/token", func() {
		secret := "s3cr3t"

		doRequest := func(permissions ...string) {
			p := &url.Values{}
			for _, perm := range permissions {
				p.Add("permission[]", perm)
			}
			req, err := NewAuthedFormRequest(t, "POST", "/api/v0/containers/elixir-web-1/token", p)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			c := &container.Container{
				CID:    "deadbeef",
				Slug:   "elixir-web-1",
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
				Secret: secret,
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
		})

		It("returns an HMAC token signed with the shared secret", func() {
			doRequest()
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string]string
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			err = hmactoken.Verify(nil, j["token"], secret)
			Expect(err).To(BeNil())
		})

		It("returns an HMAC token with only the requested permissions", func() {
			doRequest("attach", "logs")
			Expect(res.Code).To(Equal(http.StatusOK))

			var j map[string]string
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			err = hmactoken.Verify([]string{"attach", "logs"}, j["token"], secret)
			Expect(err).To(BeNil())
			err = hmactoken.Verify([]string{"shell"}, j["token"], secret)
			Expect(err).To(HaveOccurred())
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = "/api/v0/containers/elixir-web-1/token"
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("prepareConfig()", func() {
		var (
			form url.Values
			hv   map[string]*volume.Volume
		)

		BeforeEach(func() {
			form = url.Values{}
			hv = map[string]*volume.Volume{}
		})

		getConfs := func() *freighterclient.ContainerConfig {
			fcc, err := containers.PrepareConfig(form, hv)
			Expect(err).To(BeNil())
			return fcc
		}

		It("is empty when nothing relevant is provided", func() {
			fcc := getConfs()
			Expect(fcc.Env).To(BeNil())
			Expect(fcc.Mounts).To(BeNil())
			Expect(fcc.PortBindings).To(BeNil())
		})

		It("populates Env", func() {
			form.Add("env[]", "PAGER=less")
			fcc := getConfs()
			Expect(fcc.Env).To(Equal([]string{"PAGER=less"}))
		})

		Context("populates Mounts", func() {
			BeforeEach(func() {
				form.Add("volume[]", "music:/mpd/music")
				form.Add("volume[]", "backups")
				hv["music"] = &volume.Volume{}
				hv["backups"] = &volume.Volume{}
			})

			It("if host volumes are present", func() {
				vols := getConfs().Mounts
				Expect(vols).To(HaveLen(2))
				Expect(vols).To(ContainElement("music:/mpd/music"))
				Expect(vols).To(ContainElement("backups"))
			})

			It("returns error if host volume requested is absent", func() {
				delete(hv, "backups")
				fcc, err := containers.PrepareConfig(form, hv)
				Expect(err).To(Equal(containers.ErrVolumeNotFound))
				Expect(fcc).To(BeNil())
			})
		})

		It("populates PortBindings", func() {
			form.Add("port[]", "3030:4040")
			form.Add("port[]", "5050:6060")
			pbs := getConfs().PortBindings
			Expect(pbs).To(ConsistOf("3030:4040", "5050:6060"))
		})

		Context("publish all option", func() {
			It("defaults to false", func() {
				publishAll := getConfs().PublishAll
				Expect(publishAll).To(BeFalse())
			})

			It("can be set to true", func() {
				form.Add("publish_all", "1")
				publishAll := getConfs().PublishAll
				Expect(publishAll).To(BeTrue())
			})
		})

		Context("cmd option", func() {
			It("defaults to nil", func() {
				cmd := getConfs().Cmd
				Expect(cmd).To(BeNil())
			})

			cases := []struct {
				input    string
				expected []string
				errMsg   string
			}{
				{"/bin/bash", []string{"/bin/bash"}, ""},
				{"./foo `echo $SHELL`", []string{"./foo", "`echo $SHELL`"}, ""},
				{"/bin/sh -c echo 'test this please' ",
					[]string{"/bin/sh", "-c", "echo", "test this please"}, ""},
				{"echo $(rm /tmp/ohnoes) ohno",
					[]string{"echo", "$(rm", "/tmp/ohnoes)", "ohno"}, ""},
				{"/bin/cmd\\ with\\ space arg1 arg2",
					[]string{"/bin/cmd with space", "arg1", "arg2"}, ""},
				{"unclosed quote '", []string{}, "container cmd invalid"},
			}

			for _, c2 := range cases {
				c := c2
				It(fmt.Sprintf("handles input: %s", c.input), func() {
					form.Add("cmd", c.input)
					fcc, err := containers.PrepareConfig(form, hv)
					if c.errMsg != "" {
						Expect(err).NotTo(BeNil())
						Expect(err.Error()).To(ContainSubstring(c.errMsg))
					} else {
						Expect(err).To(BeNil())
						Expect(fcc.Cmd).To(Equal(c.expected))
					}
				})
			}
		})
	})

	Describe("POST /containers/{name}/reconfig", func() {
		var (
			containerName = "elixir-web-1"
			c             *container.Container
		)

		doRequest := func() {
			params := &url.Values{}
			params.Add("env[]", "LANG=C")
			params.Add("env[]", "NODE=nope")
			params.Add("volume[]", "music")
			params.Add("port[]", "3000:8080")
			params.Add("port[]", "0.0.0.0::4552")
			params.Add("port[]", "0.0.0.0:8081:11211")

			req, err := NewAuthedFormRequest(t, "POST", fmt.Sprintf("/api/v0/containers/%s/reconfig", containerName), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			v := &volume.Volume{HostID: hs.H.ID, Slug: "music"}
			err := v.Insert(db)
			Expect(err).To(BeNil())

			c = &container.Container{
				CID:    "deadbeef",
				Slug:   containerName,
				Image:  "nitrous-base",
				UserID: u.ID,
				HostID: hs.H.ID,
				Name:   sql.NullString{"Elixir server", true},
			}
			err = c.Insert(db)
			Expect(err).To(BeNil())

			mockClient.ReconfigContainer = &freighterclient.Container{
				ID:    "livebeef",
				Name:  containerName,
				Host:  "1.2.3.4",
				State: "running",
				Image: "deadbeef-reconfig-image",
			}
		})

		It("calls the Freighter client with config from params", func() {
			doRequest()
			Expect(mockClient.ReconfigCalled).To(Equal(1))
			Expect(mockClient.ReceivedID).To(Equal("deadbeef"))
			conf := mockClient.ReceivedConfig
			Expect(conf.Env).To(Equal([]string{"LANG=C", "NODE=nope"}))
			Expect(conf.Mounts).To(Equal([]string{"music"}))
			Expect(conf.PortBindings).To(HaveLen(3))
			Expect(conf.PortBindings).To(ContainElement("3000:8080"))
			Expect(conf.PortBindings).To(ContainElement("0.0.0.0::4552"))
			Expect(conf.PortBindings).To(ContainElement("0.0.0.0:8081:11211"))
		})

		It("responds with the container from Freighter and augments it with data from the database", func() {
			doRequest()

			Expect(res.Code).To(Equal(http.StatusOK))

			var j struct {
				Container *serializers.Container `json:"container"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())
			cn := j.Container

			Expect(cn.CID).To(Equal("livebeef"))
			Expect(cn.Image).To(Equal("deadbeef-reconfig-image"))
			Expect(cn.Slug).To(Equal(containerName))
			Expect(cn.IPAddress).To(Equal("1.2.3.4"))
			Expect(cn.State).To(Equal("running"))
			Expect(cn.Name).To(Equal("Elixir server"))
			Expect(cn.UpdatedAt).NotTo(BeNil())
			Expect(cn.UserID).To(Equal(u.ID))
			Expect(cn.HostID).To(Equal(hs.H.ID))
		})

		It("updates container record", func() {
			doRequest()

			c, err := container.Find(db, hs.U, containerName)
			Expect(err).To(BeNil())
			Expect(c.CID).To(Equal("livebeef"))
			Expect(c.Image).To(Equal("deadbeef-reconfig-image"))
		})

		It("updates volumes", func() {
			// create previous volume association 'dotfiles', expect it to be
			// replaced with 'music' as provided in params
			v := &volume.Volume{HostID: hs.H.ID, Slug: "dotfiles"}
			err := v.Insert(db)
			Expect(err).To(BeNil())
			cv := &cvolume.ContainerVolume{ContainerID: c.ID, VolumeID: v.ID}
			err = cv.Insert(db)
			Expect(err).To(BeNil())
			doRequest()
			cvs := []*cvolume.ContainerVolume{}
			err = db.Select(&cvs, `SELECT cv.id, container_id, volume_id, cv.created_at
				FROM container_volumes cv
				JOIN containers ON containers.id = container_id
				WHERE container_id = $1 AND terminated_at IS NULL`,
				c.ID)
			Expect(err).To(BeNil())
			music, err := volume.Find(db, hs.H.ID, "music")
			Expect(err).To(BeNil())
			Expect(cvs).To(HaveLen(1))
			Expect(cvs[0].VolumeID).To(Equal(music.ID))
		})

		Context("when the Freighter client returns an error", func() {
			BeforeEach(func() {
				mockClient.ReconfigError = errors.New("failed to reconfigure container")
			})

			It("responds with HTTP 500 Internal Server Error", func() {
				doRequest()

				Expect(res.Code).To(Equal(http.StatusInternalServerError))

				errType, _ := ErrorSlug(res.Body)
				Expect(errType).To(Equal("internal_server_error"))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/reconfig", containerName)
				authTokenInputs.Method = "POST"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
