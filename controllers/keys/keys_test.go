package keys_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"bitbucket.org/kardianos/osext"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers/keys"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/publickey"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "controllers/keys")
}

var _ = Describe("controllers/keys", func() {
	validKeys := []string{
		"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6YIJRY7GTMUOHjoRDP4IXa5LdmKVYpW0ORccuRztsbBVcZhkm2GVrvOPwnUc53k0zUe+q95im+Yb3d5pATX27KG0VheDx7mz2l2xUb44VzdT6aMpYEsQREw5RrUnpzFQgPh295ZbVPTIGzq0+7LALlDMP5YpY8oYrY1gAtTEmivcffD/fQMmcExlRbWRBvDc/nfSoN5wSpG+sW32gQ/l5IWtmPlnhb4ze/D32YDL6OVK75tlFNeMk2pmPWejnw9GFOBXkXEpYVfmd5AHpJqc1WskOdv1ahXwM2n8pghkjw3eh9YDmpiLkxeJEpA+pNJA69NdF4fYJhpm2Nz2dIvwn",
		"ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAEJfzWE4kBhqfyJV5PGUhCyaJ9GB0RRrc3RsCweBLgLmbagOy4IwRtd79/AgxlmoZ/7e8eTNfAhE4vV6MXJoltIkAFz+0+NsAopvQEWWDHfADilCuTltegQn6r1f6iA5XgSoLJQvnJVaneo4UzClCHIVDZWQL1XvNlKKtxRHU0q0niElA==",
	}

	var (
		db              *db.Database
		handler         *mux.Router
		res             *httptest.ResponseRecorder
		req             *http.Request
		svr             *server.Server
		params          *url.Values
		hs              *HostStack
		c               *container.Container
		authToken       *token.Token
		authTokenInputs AuthTokenInputs

		// Mock client that satisfies the freighterclient.Client interface.
		mockClient *freighterclient.MockClient
	)

	expectSuccessfulRequest := func(status string) map[string]interface{} {
		j := map[string]interface{}{}
		err := json.NewDecoder(res.Body).Decode(&j)
		Expect(err).To(BeNil())
		Expect(j["error_description"]).To(BeNil())
		Expect(j["error"]).To(BeNil())
		Expect(j["status"]).To(Equal(status))
		return j
	}

	expectBadRequest := func(errMsg string) map[string]interface{} {
		j := map[string]interface{}{}
		err := json.NewDecoder(res.Body).Decode(&j)
		Expect(err).To(BeNil())
		Expect(j["error_description"]).To(Equal(errMsg))
		return j
	}

	BeforeEach(func() {
		var err error
		mockClient = &freighterclient.MockClient{}
		keys.FreighterClient = mockClient

		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		root, err := osext.ExecutableFolder()
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer(root, db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()
		authTokenInputs = AuthTokenInputs{Handler: handler}

		hs, err = CreateHostStack(db, &HostStack{
			H: &host.Host{
				Slug:      "foo",
				IPAddress: sql.NullString{"127.0.0.1", true},
			},
		})
		Expect(err).To(BeNil())

		authToken = &token.Token{
			UserID: hs.U.ID,
			Token:  "valid-token",
		}
		err = authToken.Insert(db)
		Expect(err).To(BeNil())

		c = &container.Container{
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Slug:   "java-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/containers/{container}/keys", func() {
		doRequest := func() {
			var err error
			req, err = NewAuthedRequest(authToken, "GET", fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when a valid oauth token is given", func() {
			Context("when user has keys", func() {
				var pks []*publickey.PublicKey

				BeforeEach(func() {
					for _, k := range validKeys {
						pk := &publickey.PublicKey{
							UserID:      hs.U.ID,
							ContainerID: c.ID,
							Key:         k,
						}
						err := pk.Insert(db)
						Expect(err).To(BeNil())
						pks = append(pks, pk)
					}

					params = &url.Values{}
					doRequest()
				})

				It("responds with ok", func() {
					expectSuccessfulRequest("ok")
					Expect(res.Code).To(Equal(http.StatusOK))
				})

				It("responds with json keys", func() {
					j := struct {
						Status string                `json:"status"`
						Keys   []publickey.PublicKey `json:"keys"`
					}{}
					err := json.NewDecoder(res.Body).Decode(&j)
					Expect(err).To(BeNil())
					for i := 0; i < 2; i++ {
						Expect(j.Keys[i].ID).To(Equal(pks[i].ID))
						Expect(j.Keys[i].ContainerID).To(Equal(c.ID))
						Expect(j.Keys[i].Key).To(Equal(pks[i].Key))
						Expect(j.Keys[i].UserID).To(Equal(int64(0))) // UserID not marshalled
					}
				})
			})

			Context("when user has no keys", func() {
				BeforeEach(func() {
					params = &url.Values{}
					doRequest()
				})

				It("returns empty array", func() {
					j := expectSuccessfulRequest("ok")
					Expect(j["keys"]).To(BeEmpty())
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug)
				authTokenInputs.Method = "GET"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	for _, meth := range []string{"POST", "PUT"} {
		m := meth
		Describe(m+" /api/v0/containers/{container}/keys", func() {
			doRequest := func(slug string) {
				var err error
				req, err = NewAuthedRequest(authToken, m, fmt.Sprintf("/api/v0/containers/%s/keys", slug), params)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)
			}

			Context("when valid oauth token is present", func() {
				Context("when valid params are received", func() {
					BeforeEach(func() {
						params = &url.Values{}
						params.Set("keytext", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC0Ddg2BE+XhqJHHI164Sxi7euv5bTTxdW1D/mUTd1uQruFvyyMPAVjRhR1aSgvh8DGlaj/ndV5CViR6zU9upLf9XPjCg84/4HMBpbhSA3PWRb6YMwf85HEAfiXYLmw06PD40Ts6EHbvtWfQLo06bxsnGaRaP4dJNZRkDpEuQTZNoWh0SQzG3Ww5TA7V80WCAdBnIOXymjGDm+Ri24RibmO+mp4s2dnevZOPXAOSAn5axd5X6INg+TEYFaEvFT8JnBTBvCWTTeHj+Xti9wWBcGbdFI/vAoHNrDtCav0UBe5ueItXAUklm4lo7uG+CL/33fLQ+iEC+WTTivTt9U0IORB")
						doRequest(c.Slug)
						mockClient.AddPublicKeysError = nil
					})

					It("creates the key", func() {
						expectSuccessfulRequest("created")
						pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(pks).To(HaveLen(1))
						Expect(mockClient.AddPublicKeysCalled).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal(c.CID))
					})
				})

				Context("when key already exists", func() {
					BeforeEach(func() {
						pk := &publickey.PublicKey{
							UserID:      hs.U.ID,
							ContainerID: c.ID,
							Key:         validKeys[0],
						}
						err := pk.Insert(db)
						Expect(err).To(BeNil())

						params = &url.Values{}
						params.Set("keytext", validKeys[0])
						doRequest(c.Slug)
					})

					It("responds with bad request", func() {
						expectBadRequest("key already exists")
					})
				})

				Context("when invalid key is received", func() {
					BeforeEach(func() {
						params = &url.Values{}
						params.Set("keytext", "ssh-rsa bad key!!")
						doRequest(c.Slug)
					})

					It("responds with bad request", func() {
						expectBadRequest("invalid key")
					})
				})

				Context("when invalid container id is received", func() {
					BeforeEach(func() {
						params = &url.Values{}
						params.Set("keytext", validKeys[0])
						doRequest("container-that-does-not-exist")
					})

					It("responds with bad request", func() {
						expectBadRequest("container does not exist")
					})
				})

				Context("when the call to freighter fails for some reason", func() {
					BeforeEach(func() {
						mockClient.AddPublicKeysError = freighterclient.ErrPubKeyOpsNotSupported
						params = &url.Values{}
						params.Set("keytext", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC0Ddg2BE+XhqJHHI164Sxi7euv5bTTxdW1D/mUTd1uQruFvyyMPAVjRhR1aSgvh8DGlaj/ndV5CViR6zU9upLf9XPjCg84/4HMBpbhSA3PWRb6YMwf85HEAfiXYLmw06PD40Ts6EHbvtWfQLo06bxsnGaRaP4dJNZRkDpEuQTZNoWh0SQzG3Ww5TA7V80WCAdBnIOXymjGDm+Ri24RibmO+mp4s2dnevZOPXAOSAn5axd5X6INg+TEYFaEvFT8JnBTBvCWTTeHj+Xti9wWBcGbdFI/vAoHNrDtCav0UBe5ueItXAUklm4lo7uG+CL/33fLQ+iEC+WTTivTt9U0IORB")
						doRequest(c.Slug)
					})

					It("responds with bad request", func() {
						expectBadRequest("public key operations not supported on this container")
					})

					It("should not persist the key in the database", func() {
						pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(pks).To(HaveLen(0))
					})
				})
			})

			Context("oauth token failure modes", func() {
				BeforeEach(func() {
					authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug)
					authTokenInputs.Method = m
				})

				ItRequiresAuthToken(&authTokenInputs)
			})
		})
	}

	Describe("DELETE /api/v0/containers/{container}/keys", func() {
		doRequest := func(slug string) {
			var err error
			req, err = NewAuthedRequest(authToken, "DELETE", fmt.Sprintf("/api/v0/containers/%s/keys", slug), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("with valid oauth token", func() {
			Context("when user has multiple keys", func() {
				BeforeEach(func() {
					for i := 0; i < 2; i++ {
						pk := &publickey.PublicKey{
							UserID:      hs.U.ID,
							ContainerID: c.ID,
							Key:         validKeys[i],
						}
						err := pk.Insert(db)
						Expect(err).To(BeNil())
					}

					pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
					Expect(err).To(BeNil())
					Expect(pks).To(HaveLen(2))

					params = &url.Values{}
				})

				Context("when call to freighterclient succeeds", func() {
					BeforeEach(func() {
						mockClient.ClearPublicKeysError = nil
						doRequest(c.Slug)
					})

					It("removes all of them", func() {
						expectSuccessfulRequest("ok")

						pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(pks).To(BeEmpty())
						Expect(mockClient.ClearPublicKeysCalled).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal(c.CID))
					})
				})

				Context("when the call to freighter fails for some reason", func() {
					BeforeEach(func() {
						mockClient.ClearPublicKeysError = freighterclient.ErrPubKeyOpsNotSupported
						doRequest(c.Slug)
					})

					It("returns bad request", func() {
						expectBadRequest("public key operations not supported on this container")
						Expect(mockClient.ClearPublicKeysCalled).To(Equal(1))
						Expect(mockClient.ReceivedID).To(Equal(c.CID))
					})

					It("should not destroy the keys from the database", func() {
						pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
						Expect(err).To(BeNil())
						Expect(len(pks)).To(Equal(2))
					})
				})
			})

			Context("when user has no keys", func() {
				BeforeEach(func() {
					params = &url.Values{}
					doRequest(c.Slug)
					mockClient.ClearPublicKeysError = nil
				})

				It("returns success", func() {
					expectSuccessfulRequest("ok")
					Expect(mockClient.ClearPublicKeysCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))
				})
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys", c.Slug)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})

	Describe("DELETE /api/v0/keys/<md5_fingerprint>", func() {
		var pks []*publickey.PublicKey
		var c2 *container.Container
		var fprint string

		doRequest := func(slug, fingerprint string) {
			var err error
			req, err = NewAuthedRequest(authToken, "DELETE",
				fmt.Sprintf("/api/v0/containers/%s/keys/%s", slug, fingerprint), params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		BeforeEach(func() {
			pks = []*publickey.PublicKey{}
			for i := 0; i < 2; i++ {
				pk := &publickey.PublicKey{
					UserID:      1,
					ContainerID: c.ID,
					Key:         validKeys[i],
				}
				err := pk.Insert(db)
				Expect(err).To(BeNil())
				pks = append(pks, pk)
			}

			c2 = &container.Container{
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "go-benchmark",
			}
			err := c2.Insert(db)
			Expect(err).To(BeNil())
			fprint, err = pks[1].Md5Fingerprint()
			Expect(err).To(BeNil())
			mockClient.RemovePublicKeyCalled = 0
		})

		Context("with valid oauth token", func() {
			Context("when call to freighter is successful", func() {
				BeforeEach(func() {
					mockClient.RemovePublicKeyError = nil
				})

				It("is successful when key is present in container", func() {
					doRequest(c.Slug, fprint)
					expectSuccessfulRequest("ok")
					Expect(mockClient.RemovePublicKeyCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))
				})
			})

			Context("when call to freighter is unsuccessful", func() {
				BeforeEach(func() {
					mockClient.RemovePublicKeyError = freighterclient.ErrPubKeyOpsNotSupported
				})

				It("should respond with bad request", func() {
					doRequest(c.Slug, fprint)
					expectBadRequest("public key operations not supported on this container")
					Expect(mockClient.RemovePublicKeyCalled).To(Equal(1))
					Expect(mockClient.ReceivedID).To(Equal(c.CID))
				})

				It("should not destroy the keys from the database", func() {
					doRequest(c.Slug, fprint)
					pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
					Expect(err).To(BeNil())
					Expect(len(pks)).To(Equal(2))
				})
			})

			It("is unsuccessful when key is not in container", func() {
				doRequest(c2.Slug, fprint)
				expectBadRequest("fingerprint not found")
				Expect(mockClient.RemovePublicKeyCalled).To(Equal(0))

				pks, err := publickey.FindAllByUserIDAndContainerID(db, hs.U.ID, c.ID)
				Expect(err).To(BeNil())
				Expect(len(pks)).To(Equal(2))
			})
		})

		Context("oauth token failure modes", func() {
			BeforeEach(func() {
				authTokenInputs.Path = fmt.Sprintf("/api/v0/containers/%s/keys/%s", c.Slug, fprint)
				authTokenInputs.Method = "DELETE"
			})

			ItRequiresAuthToken(&authTokenInputs)
		})
	})
})
