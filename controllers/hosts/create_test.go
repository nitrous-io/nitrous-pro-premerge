package hosts_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/pkg/analytics"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
	"github.com/nitrous-io/nitrous/serializers"
	"github.com/nitrous-io/nitrous/server"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("POST /api/v0/hosts", func() {
	var (
		db              *db.Database
		svr             *server.Server
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		authTokenInputs AuthTokenInputs
		params          *url.Values

		fakeAnalyticsClient     *FakeAnalytics
		mockClient              *freighterclient.MockClient
		unstubAnalyticsClientFn func()

		u  *user.User
		t  *token.Token
		hs *HostStack
	)

	BeforeEach(func() {
		var err error

		mockClient = &freighterclient.MockClient{}
		StubDefaultFreighterFn(mockClient)

		fakeAnalyticsClient, unstubAnalyticsClientFn = StubAnalyticsClient()

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
		Expect(u.ID).NotTo(BeZero())

		t = &token.Token{
			UserID: u.ID,
			Token:  "valid-token",
		}
		err = t.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		UnstubFreighterFn()
		unstubAnalyticsClientFn()
	})

	BeforeEach(func() {
		var err error
		hs, err = CreateHostStack(db, &HostStack{
			R: &region.Region{Slug: "use"},
			P: &plan.Plan{Slug: "large"},
		})
		Expect(err).To(BeNil())

		params = &url.Values{}
		params.Set("slug", "honeybadger")
		params.Set("size", hs.P.Slug)
		params.Set("region", hs.R.Slug)
		params.Set("name", "honeybadger don't care")
	})

	type overrides map[string]string
	doRequest := func(p *url.Values) *httptest.ResponseRecorder {
		req, err := NewRequest("POST", "/api/v0/hosts", p)
		Expect(err).To(BeNil())
		req.Header.Set("Authorization", "Bearer valid-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}

	// tests for required params
	for _, param := range []string{"size", "region", "slug"} {
		p := param

		It("responds with "+p+" required when request does not specify a "+p, func() {
			params.Del(p)
			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(Equal(p + " is required"))
		})
	}

	It("responds with invalid slug when an invalid slug is specified", func() {
		params.Set("slug", "foo_bar")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("slug may only contain alphanumeric characters or dashes and cannot begin or end with a dash"))
	})

	It("should not send any analytics events if there are errors", func() {
		params.Set("slug", "foo_bar")
		doRequest(params)

		Expect(fakeAnalyticsClient.TrackEventCalled).To(Equal(0))
		Expect(fakeAnalyticsClient.Events).To(BeEmpty())
	})

	It("responds with plan not found when a matching plan cannot be found for the specified size", func() {
		params.Set("size", "doubledouble")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching plan found for the given size"))
	})

	It("responds with region not found when an invalid region is specified", func() {
		params.Set("region", "antarctica")
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching region found for the given name"))
	})

	It("responds with provider template not found error when no provider template found for the given plan and region", func() {
		var r2 *region.Region

		r2 = &region.Region{Slug: "usw", Name: "US West"}
		err := r2.Insert(db)
		Expect(err).To(BeNil())

		params.Set("region", r2.Slug)
		res := doRequest(params)

		Expect(res.Code).To(Equal(http.StatusBadRequest))

		errType, errDesc := ErrorSlug(res.Body)
		Expect(errType).To(Equal("invalid_request"))
		Expect(errDesc).To(Equal("no matching templates found for the given plan and regions combination"))
	})

	It("sends the host configs as json params", func() {
		hb := hostbuilderclient.NewHostbuilder("www.example.com")
		rt := &FakeRoundTripper{
			message: fmt.Sprintf(`
			{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
			`, hs.PT.Name),
			status: http.StatusOK,
		}
		replaceTransport(hb, rt)
		hosts.HostbuilderClient = hb
		doRequest(params)

		req := rt.requests[0]
		var config hostbuilderclient.Config
		json.Unmarshal([]byte(req.FormValue("config")), &config)
		Expect(config["name"]).To(Equal(params.Get("slug")))
		Expect(config["name"]).To(Equal(params.Get("slug")), "invalid slug set in name")
		Expect(config["apt_password"]).To(Equal(t.Token), "invalid token set in apt_password")
	})

	Context("when hostbuilder successfully creates a host", func() {
		var rt *FakeRoundTripper

		BeforeEach(func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			rt = &FakeRoundTripper{
				message: fmt.Sprintf(`
				{"stream":"starting: create the droplet"}
				{"stream":"finished: create the droplet"}
				{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}
				`, hs.PT.Name),
				status: http.StatusOK,
			}
			replaceTransport(hb, rt)
			hosts.HostbuilderClient = hb
		})

		It("creates a host record for the user", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]
			Expect(h.RegionID).To(Equal(hs.R.ID))
			Expect(h.PlanID).To(Equal(hs.P.ID))
			Expect(h.ProviderTemplateID).To(Equal(hs.PT.ID))

			Expect(h.Slug).To(Equal(params.Get("slug")))
			Expect(h.Name.String).To(Equal(params.Get("name")))

			Expect(h.IPAddress.String).To(Equal("0.0.0.0"))
			Expect(h.State).To(Equal(host.StateRunning))
			Expect(h.MaxContainers.Int64).To(Equal(hs.P.MaxContainers))
			Expect(h.Size.String).To(Equal(hs.P.Slug))
			Expect(h.ProviderIdentifier.String).To(Equal("abcd1234"))
			Expect(h.FreighterdPort.Int64).To(Equal(int64(65333)))
		})

		It("tracks the 'Host Created' event", func() {
			doRequest(params)
			Expect(fakeAnalyticsClient.TrackEventCalled).To(Equal(1))
			Expect(fakeAnalyticsClient.Events).To(Equal([]string{analytics.HostCreated}))
			Expect(fakeAnalyticsClient.User.ID).To(Equal(u.ID))
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			h := hosts[0]

			eventJSON, err := json.Marshal(fakeAnalyticsClient.TrackEventProperties)
			Expect(err).To(BeNil())
			Expect(eventJSON).To(MatchJSON(fmt.Sprintf(`[{"slug":"%s","host_id":%d,"plan":"%s","region":"%s"}]`, h.Slug, h.ID, hs.P.Name, hs.R.Name)))
		})

		It("creates a keypair associated with that host", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			kp, err := keypairs.Get(db, hosts[0].ID)
			Expect(err).To(BeNil())
			Expect(kp.HostID).To(Equal(hosts[0].ID))

			req := rt.requests[0]
			var config hostbuilderclient.Config
			json.Unmarshal([]byte(req.FormValue("config")), &config)
			public_keys := config["public_keys"].([]interface{})
			Expect(public_keys[0].(string)).To(Equal(kp.PublicKey))
		})

		It("creates a shared secret for the host", func() {
			doRequest(params)
			hosts, _ := host.FindAllByUser(db, u)

			Expect(hosts[0].Secret).NotTo(BeEmpty())
			req := rt.requests[0]
			var config hostbuilderclient.Config
			json.Unmarshal([]byte(req.FormValue("config")), &config)
			Expect(config["secret"]).To(Equal(hosts[0].Secret))
		})

		It("creates a cert associated with that host", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			crt, err := cert.Find(db, hosts[0].ID)
			Expect(err).To(BeNil())
			Expect(crt.HostID).To(Equal(hosts[0].ID))
			Expect(crt.ServerCert).To(ContainSubstring("BEGIN CERTIFICATE"))
			Expect(crt.ServerKey).To(ContainSubstring("BEGIN RSA PRIVATE KEY"))
		})

		It("creates a shared volume and publishes a volume created event", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest(params)

			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			vol, err := volume.Find(db, hosts[0].ID, "shared")
			Expect(err).To(BeNil())
			Expect(vol).NotTo(BeNil())
			Expect(vol.System).To(BeTrue())

			evtCh := make(chan *controllers.Event)
			go func() {
				var evt *controllers.Event
				for msg := range ch { // Ignore events we don't care about.
					evt = msg.(*controllers.Event)
					if evt.Name == controllers.EventVolumeCreated {
						evtCh <- evt
						break
					}
				}
			}()

			select {
			case <-time.After(50 * time.Millisecond):
				Fail("Timeout while getting shared volume created event")
			case evt := <-evtCh:
				Expect(evt.Name).To(Equal(controllers.EventVolumeCreated))
				evtData, ok := evt.Data.(map[string]*serializers.Volume)
				Expect(ok).To(BeTrue())
				Expect(evtData["volume"]).To(Equal(serializers.SerializeVolume(vol)))
			}
		})

		It("responds with the progress data and created host", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusOK))
			buf := bytes.NewBuffer(nil)
			io.Copy(buf, res.Body)

			Expect(res.Header().Get("X-Accel-Buffering")).To(Equal("no"), "failed to disable buffering in nginx")
			Expect(buf.String()).To(ContainSubstring("starting: create the droplet"), "expect response to include hostbuilder progress")

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			decoder := json.NewDecoder(buf)
			for {
				err := decoder.Decode(&j)
				if j.Host != nil {
					break
				}
				if err == io.EOF && j.Host == nil {
					Fail("invalid response, missing host info")
				}
			}

			hosts, _ := host.FindAllByUser(db, u)
			Expect(j.Host.ID).To(Equal(hosts[0].ID))
			Expect(j.Host.UserID).To(Equal(u.ID))
			Expect(j.Host.RegionID).To(Equal(hs.R.ID))
			Expect(j.Host.PlanID).To(Equal(hs.P.ID))
			Expect(j.Host.Slug).To(Equal(params.Get("slug")))
			Expect(j.Host.IPAddress).To(Equal("0.0.0.0"))
			Expect(j.Host.FreighterdPort).To(Equal(int64(65333)))
			Expect(j.Host.Size).To(Equal(hs.P.Slug))
			Expect(j.Host.State).To(Equal(host.StateRunning))
			Expect(j.Host.Hostname).To(Equal(hostname("honeybadger")))
			Expect(j.Host.CreatedAt).NotTo(BeNil())
			Expect(j.Host.UpdatedAt).NotTo(BeNil())
			Expect(j.Host.TerminatedAt).To(BeNil())
		})

		It("publishes events for the created host", func() {
			ch := svr.Events.Subscribe(u.Topic())
			defer svr.Events.Unsubscribe(ch, u.Topic())

			doRequest(params)

			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			h := hosts[0]

			msgs := []pubsub.Message{}
			finished := make(chan struct{})
			go func() {
				msgs = append(msgs, <-ch, <-ch, <-ch, <-ch)
				close(finished)
			}()

			select {
			case <-time.After(100 * time.Millisecond):
				Fail("timedout while waiting for events")
			case <-finished:
				Expect(msgs[0]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreating,
					Data: &controllers.ProgressEvent{ID: h.ID, Slug: h.Slug,
						Progress: `{"stream":"starting: create the droplet"}`},
				}), "unexpected event, wanted EventHostCreating")

				Expect(msgs[1]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreating,
					Data: &controllers.ProgressEvent{ID: h.ID, Slug: h.Slug,
						Progress: `{"stream":"finished: create the droplet"}`},
				}), "unexpected event, wanted EventHostCreating")

				Expect(msgs[2]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreating,
					Data: &controllers.ProgressEvent{ID: h.ID, Slug: h.Slug,
						Progress: fmt.Sprintf(
							`{"id":"abcd1234", "ip_address":"0.0.0.0", "provider":"%s", "freighterd_port":65333}`,
							hs.PT.Name,
						)},
				}), "unexpected event, wanted EventHostCreating")

				Expect(msgs[3]).To(Equal(&controllers.Event{
					Name: controllers.EventHostCreated,
					Data: map[string]*serializers.Host{
						"host": serializers.SerializeHost(&h),
					},
				}), "unexpected event, wanted EventHostCreated")
			}
		})

		It("converts the 'slug' form value to lowercase", func() {
			params.Set("slug", "HoneyBADGER")
			doRequest(params)

			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].Slug).To(Equal("honeybadger"))
		})

		Context("when trying to create multiple hosts concurrently", func() {
			It("only creates one host", func() {
				var wg sync.WaitGroup
				var resCodes []int

				for i := 0; i < 2; i++ {
					wg.Add(1)
					hID := i
					go func() {
						params.Set("slug", fmt.Sprintf("host-%d", hID))
						res := doRequest(params)
						resCodes = append(resCodes, res.Code)
						defer wg.Done()
					}()
				}

				wg.Wait()

				hosts, err := host.FindAllByUser(db, u)
				Expect(err).To(BeNil())
				Expect(len(hosts)).To(Equal(1))

				Expect(resCodes).To(ConsistOf(http.StatusOK, http.StatusConflict))
			})
		})
	})

	Context("when trying to create a host with an existing slug", func() {
		BeforeEach(func() {
			_, err := CreateHostStack(db, &HostStack{
				H: &host.Host{Slug: "honeybadger"},
				U: u,
			})
			Expect(err).To(BeNil())

			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))

			// increase user's default hosts limit to hit the actual error
			_, err = db.Exec(`UPDATE user_profiles SET max_hosts = 10 WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("does not create a host", func() {
			doRequest(params)
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
		})

		It("responds with HTTP 400 Bad Request", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusBadRequest))
			j := map[string]interface{}{}
			err := json.Unmarshal(res.Body.Bytes(), &j)
			Expect(err).To(BeNil())
			errType, _ := j["error"].(string)
			Expect(errType).To(Equal("validation_failed"))
			errDesc, _ := j["error_description"].(string)
			Expect(errDesc).To(Equal("slug is taken"))
		})
	})

	Context("when hostbuilder fails to create the host", func() {
		BeforeEach(func() {
			hb := hostbuilderclient.NewHostbuilder("www.example.com")
			replaceTransport(hb, &FakeRoundTripper{
				message: errorResponse,
				status:  http.StatusBadRequest,
			})

			hosts.HostbuilderClient = hb
		})

		It("creates a host in the orphan state", func() {
			doRequest(params)
			h, err := host.Find(db, u, params.Get("slug"))
			Expect(err).To(BeNil())
			Expect(h.Slug).To(Equal(params.Get("slug")))
			Expect(h.State).To(Equal(host.StateOrphan))
		})
	})

	Context("when async param is true", func() {
		BeforeEach(func() {
			params.Set("async", "true")
		})

		It("returns 202 accepted and creates host asynchronously", func() {
			res := doRequest(params)
			Expect(res.Code).To(Equal(http.StatusAccepted))

			var j struct {
				Host *serializers.Host `json:"host"`
			}
			err := json.NewDecoder(res.Body).Decode(&j)
			Expect(err).To(BeNil())

			Expect(j.Host.State).To(Equal(host.StateBorn))

			// These fields are not yet set.
			Expect(j.Host.IPAddress).To(BeEmpty())
			Expect(j.Host.FreighterdPort).To(BeZero())

			h, err := host.Find(db, u, params.Get("slug"))
			Expect(err).To(BeNil())
			Expect(h.Slug).To(Equal(params.Get("slug")))
			Expect(h.State).To(Equal(host.StateBorn))
		})

		Context("when hostbuilder successfully creates a host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				rt := &FakeRoundTripper{
					message: fmt.Sprintf(`{"id":"abcd1234","ip_address":"0.0.0.0","provider":"%s","freighterd_port":65333}`, hs.PT.Name),
					status:  http.StatusOK,
				}
				replaceTransport(hb, rt)

				hosts.HostbuilderClient = hb
			})

			It("updates the host record with the info from Hostbuilder", func() {
				doRequest(params)

				Eventually(func() (string, error) {
					h, err := host.Find(db, u, params.Get("slug"))
					return h.State, err
				}).Should(Equal(host.StateRunning))

				Eventually(func() (string, error) {
					h, err := host.Find(db, u, params.Get("slug"))
					return h.ProviderIdentifier.String, err
				}).Should(Equal("abcd1234"))
			})

			It("publishes a host events with the created host", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(params)

				msgs := []pubsub.Message{}
				finished := make(chan struct{})
				go func() {
					msgs = append(msgs, <-ch, <-ch)
					close(finished)
				}()

				select {
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventHostCreated")
				case <-finished:
					h, err := host.Find(db, u, params.Get("slug"))
					Expect(err).To(BeNil())

					Expect(msgs[0]).To(Equal(&controllers.Event{
						Name: controllers.EventHostCreating,
						Data: &controllers.ProgressEvent{ID: h.ID, Slug: h.Slug,
							Progress: fmt.Sprintf(
								`{"id":"abcd1234","ip_address":"0.0.0.0","provider":"%s","freighterd_port":65333}`, hs.PT.Name),
						},
					}), "unexpected event, wanted EventHostCreating")

					Expect(msgs[1]).To(Equal(&controllers.Event{
						Name: controllers.EventHostCreated,
						Data: map[string]*serializers.Host{
							"host": serializers.SerializeHost(h),
						},
					}), "unexpected event, wanted EventHostCreated")
				}
			})
		})

		Context("when hostbuilder fails to create the host", func() {
			BeforeEach(func() {
				hb := hostbuilderclient.NewHostbuilder("www.example.com")
				replaceTransport(hb, &FakeRoundTripper{
					message: `{"error":"some error occurred"}`,
					status:  http.StatusBadRequest,
				})

				hosts.HostbuilderClient = hb
			})

			It("updates the host's state to orphan", func() {
				doRequest(params)

				Eventually(func() (string, error) {
					h, err := host.Find(db, u, params.Get("slug"))
					return h.State, err
				}).Should(Equal(host.StateOrphan))
			})

			It("publishes a host create failed event with the host record and error", func() {
				ch := svr.Events.Subscribe(u.Topic())
				defer svr.Events.Unsubscribe(ch, u.Topic())

				doRequest(params)

				msgs := []pubsub.Message{}
				finished := make(chan struct{})
				go func() {
					msgs = append(msgs, <-ch, <-ch)
					close(finished)
				}()

				select {
				case <-finished:
					h, err := host.Find(db, u, params.Get("slug"))
					Expect(err).To(BeNil())

					Expect(msgs[0]).To(Equal(&controllers.Event{
						Name: controllers.EventHostCreating,
						Data: &controllers.ProgressEvent{ID: h.ID, Slug: h.Slug,
							Progress: `{"error":"some error occurred"}`},
					}))

					evt, ok := msgs[1].(*controllers.Event)
					Expect(ok).To(BeTrue())
					Expect(evt.Name).To(Equal(controllers.EventHostCreateFailed))
					evtData, ok := evt.Data.(map[string]interface{})
					Expect(ok).To(BeTrue())
					Expect(evtData["host"]).To(Equal(serializers.SerializeHost(h)))
					Expect(evtData["error"]).To(ContainSubstring("some error occurred"))
				case <-time.After(100 * time.Millisecond):
					Fail("timeout while waiting for EventHostCreateFailed")
				}
			})
		})
	})

	Context("when user has reached the max hosts limit", func() {
		It("responds with hosts limit reached", func() {
			_, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())

			res := doRequest(params)

			Expect(res.Code).To(Equal(http.StatusBadRequest))

			errType, errDesc := ErrorSlug(res.Body)
			Expect(errType).To(Equal("invalid_request"))
			Expect(errDesc).To(ContainSubstring("not allowed"))
		})
	})

	Context("oauth token failure modes", func() {
		BeforeEach(func() {
			authTokenInputs.Path = "/api/v0/hosts"
			authTokenInputs.Method = "POST"
		})

		ItRequiresAuthToken(&authTokenInputs)
	})
})
