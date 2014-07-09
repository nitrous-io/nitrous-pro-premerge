package keys_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"bitbucket.org/kardianos/osext"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
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
		db        *db.Database
		handler   *mux.Router
		res       *httptest.ResponseRecorder
		req       *http.Request
		svr       *server.Server
		params    *url.Values
		hs        *HostStack
		c         *container.Container
		authToken *token.Token
	)

	expectSuccessfulRequest := func(status string) map[string]interface{} {
		j := map[string]interface{}{}
		err := json.Unmarshal(res.Body.Bytes(), &j)
		Expect(err).To(BeNil())
		Expect(j["error"]).To(BeNil())
		Expect(j["status"]).To(Equal(status))
		return j
	}

	expectBadRequest := func(errMsg string) map[string]interface{} {
		j := map[string]interface{}{}
		err := json.Unmarshal(res.Body.Bytes(), &j)
		Expect(err).To(BeNil())
		Expect(j["error_description"]).To(Equal(errMsg))
		return j
	}

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())
		root, err := osext.ExecutableFolder()
		Expect(err).To(BeNil())
		logger := &logger.Logger{Level: logger.LogLevelQuiet}
		svr = server.NewServer(root, db, logger)
		handler = svr.Router()
		res = httptest.NewRecorder()

		hs, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		authToken = &token.Token{
			UserID: hs.U.ID,
			Token:  "foobarbazqux",
		}
		err = authToken.Insert(db)
		Expect(err).To(BeNil())

		c = &container.Container{
			ID:     "1234abc",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Name:   "java-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("GET /api/v0/keys", func() {
		doRequest := func() {
			var err error
			req, err = NewApiRequest(authToken, "GET", "/api/v0/keys", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

		Context("when valid params are received", func() {
			BeforeEach(func() {
				params = &url.Values{}
				doRequest()
			})

			It("responds with ok", func() {
				Expect(res.Code).To(Equal(http.StatusOK))
				expectSuccessfulRequest("ok")
			})

			Context("when no keys are present for the user", func() {
				It("returns empty array", func() {
					j := expectSuccessfulRequest("ok")
					Expect(j["keys"]).To(BeEmpty())
				})
			})

		})
	})

	for _, meth := range []string{"POST", "PUT"} {
		m := meth
		Describe(m+" /api/v0/keys", func() {
			doRequest := func() {
				var err error
				req, err = NewApiRequest(authToken, m, "/api/v0/keys", params)
				Expect(err).To(BeNil())
				handler.ServeHTTP(res, req)
			}

			Context("when valid params are received", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("container_id", c.ID)
					params.Set("keytext", "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC0Ddg2BE+XhqJHHI164Sxi7euv5bTTxdW1D/mUTd1uQruFvyyMPAVjRhR1aSgvh8DGlaj/ndV5CViR6zU9upLf9XPjCg84/4HMBpbhSA3PWRb6YMwf85HEAfiXYLmw06PD40Ts6EHbvtWfQLo06bxsnGaRaP4dJNZRkDpEuQTZNoWh0SQzG3Ww5TA7V80WCAdBnIOXymjGDm+Ri24RibmO+mp4s2dnevZOPXAOSAn5axd5X6INg+TEYFaEvFT8JnBTBvCWTTeHj+Xti9wWBcGbdFI/vAoHNrDtCav0UBe5ueItXAUklm4lo7uG+CL/33fLQ+iEC+WTTivTt9U0IORB")
					doRequest()
				})

				It("creates the key", func() {
					expectSuccessfulRequest("created")
					pks, err := publickey.FindAllByUserID(db, 1)
					Expect(err).To(BeNil())
					Expect(pks).To(HaveLen(1))
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
					params.Set("container_id", c.ID)
					params.Set("keytext", validKeys[0])
					doRequest()
				})

				It("responds with bad request", func() {
					expectBadRequest("key already exists")
				})
			})

			Context("when invalid key is received", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("container_id", c.ID)
					params.Set("keytext", "ssh-rsa bad key!!")
					doRequest()
				})

				It("responds with bad request", func() {
					expectBadRequest("invalid key")
				})
			})

			Context("when invalid container id is received", func() {
				BeforeEach(func() {
					params = &url.Values{}
					params.Set("container_id", "bad id")
					params.Set("keytext", validKeys[0])
					doRequest()
				})

				It("responds with bad request", func() {
					expectBadRequest("container not found")
				})
			})
		})
	}

	Describe("DELETE /api/v0/keys", func() {
		doRequest := func() {
			var err error
			req, err = NewApiRequest(authToken, "DELETE", "/api/v0/keys", params)
			Expect(err).To(BeNil())
			handler.ServeHTTP(res, req)
		}

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

				pks, err := publickey.FindAllByUserID(db, 1)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))

				params = &url.Values{}
				doRequest()
			})

			It("removes all of them", func() {
				expectSuccessfulRequest("ok")

				pks, err := publickey.FindAllByUserID(db, 1)
				Expect(err).To(BeNil())
				Expect(pks).To(BeEmpty())

			})
		})

		Context("when user has no keys", func() {
			BeforeEach(func() {
				params = &url.Values{}
				doRequest()
			})

			It("returns success", func() {
				expectSuccessfulRequest("ok")
			})
		})
	})

	Describe("DELETE /api/v0/keys/<md5_fingerprint>", func() {
		var pks []*publickey.PublicKey

		doRequest := func(fingerprint string) {
			var err error
			req, err = NewApiRequest(authToken, "DELETE",
				fmt.Sprintf("/api/v0/keys/%s", fingerprint), params)
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
		})

		Context("when key exists for user", func() {
			BeforeEach(func() {
				fprint, err := pks[1].Md5Fingerprint()
				Expect(err).To(BeNil())
				doRequest(fprint)
			})

			It("removes key", func() {
				expectSuccessfulRequest("ok")

				upks, err := publickey.FindAllByUserID(db, 1)
				Expect(err).To(BeNil())
				Expect(upks).To(HaveLen(1))
				Expect(upks[0].Key).To(Equal(pks[0].Key))
			})
		})

		Context("when key does not exist for user", func() {
			BeforeEach(func() {
				params = &url.Values{}
				doRequest("nosuchfingerprint")
			})

			It("returns 404", func() {
				Expect(res.Code).To(Equal(http.StatusNotFound))
			})

			It("does not remove keys", func() {
				upks, err := publickey.FindAllByUserID(db, hs.U.ID)
				Expect(err).To(BeNil())
				Expect(upks).To(HaveLen(len(pks)))
			})
		})

		Context("when container is specified", func() {
			var c2 *container.Container
			var fprint string

			BeforeEach(func() {
				c2 = &container.Container{
					ID:     "a1b2",
					UserID: hs.U.ID,
					HostID: hs.H.ID,
					Image:  "nitrous-base",
					Name:   "go-benchmark",
				}
				err := c2.Insert(db)
				Expect(err).To(BeNil())
				fprint, err = pks[1].Md5Fingerprint()
				Expect(err).To(BeNil())
			})

			It("is successful when key is present in container", func() {
				params = &url.Values{"container_id": {c.ID}}
				doRequest(fprint)
				expectSuccessfulRequest("ok")
			})

			It("is unsuccessful when key is not in container", func() {
				params = &url.Values{"container_id": {c2.ID}}
				doRequest(fprint)
				expectBadRequest("fingerprint not found")
			})
		})
	})
})
