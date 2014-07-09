package freighter_test

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/nitrous-io/nitrous/pkg/freighter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "freighter/client")
}

var _ = Describe("freighter/client", func() {
	Describe("NewClient()", func() {
		It("implements freighter.Client", func() {
			var _ freighter.Client = freighter.NewClient(nil)
		})
	})

	Describe("List()", func() {
		var (
			client freighter.Client
			host   *freighter.Host

			server       *ghttp.Server
			handlers     http.HandlerFunc
			statusCode   int
			responseBody string
		)

		IPAndPort := func(s *ghttp.Server) (string, int) {
			serverAddr := s.HTTPTestServer.Listener.Addr().String()
			ip, portStr, err := net.SplitHostPort(serverAddr)
			Expect(err).To(BeNil())
			port, err := strconv.Atoi(portStr)
			Expect(err).To(BeNil())
			return ip, port
		}

		BeforeEach(func() {
			handlers = ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/containers"),
				ghttp.VerifyHeaderKV("Content-Type", "application/json"),
				ghttp.RespondWithPtr(&statusCode, &responseBody),
			)

			server = ghttp.NewServer()
			server.AppendHandlers(handlers)

			ip, port := IPAndPort(server)
			host = &freighter.Host{IP: ip, Port: port, Secure: false}
			client = freighter.NewClient(nil)
		})

		AfterEach(func() {
			if server.HTTPTestServer != nil {
				server.Close()
			}
		})

		Context("when the response is HTTP 200 OK", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
			})

			Context("when the response has containers", func() {
				BeforeEach(func() {
					responseBody = `{
						"containers": [
							{
								"id":"cafebabe",
								"name":"yosemite",
								"image":"freighter.yosemite",
								"host":"172.17.0.1",
								"ssh_port":60001,
								"state":"running"
							},
							{
								"id":"deadbeef",
								"name":"mavericks",
								"image":"freighter.mavericks",
								"host":"172.17.0.2",
								"ssh_port":60002,
								"state":"running"
							}
						]
					}`
				})

				It("returns a slice of Containers", func() {
					containers, err := client.List(host)
					Expect(err).To(BeNil())
					Expect(server.ReceivedRequests()).To(HaveLen(1))
					Expect(containers).To(HaveLen(2))

					Expect(containers[0].ID).To(Equal("cafebabe"))
					Expect(containers[0].Name).To(Equal("yosemite"))
					Expect(containers[0].Image).To(Equal("freighter.yosemite"))
					Expect(containers[0].Host).To(Equal("172.17.0.1"))
					Expect(containers[0].SSHPort).To(Equal(60001))
					Expect(containers[0].State).To(Equal("running"))

					Expect(containers[1].ID).To(Equal("deadbeef"))
					Expect(containers[1].Name).To(Equal("mavericks"))
					Expect(containers[1].Image).To(Equal("freighter.mavericks"))
					Expect(containers[1].Host).To(Equal("172.17.0.2"))
					Expect(containers[1].SSHPort).To(Equal(60002))
					Expect(containers[1].State).To(Equal("running"))
				})
			})

			Context("when the response contains no containers", func() {
				BeforeEach(func() {
					responseBody = `{"containers": []}`
				})

				It("returns an empty slice", func() {
					containers, err := client.List(host)
					Expect(err).To(BeNil())
					Expect(server.ReceivedRequests()).To(HaveLen(1))
					Expect(containers).To(HaveLen(0))
				})
			})

			Context("when the response is empty", func() {
				BeforeEach(func() {
					responseBody = ``
				})

				It("returns ResponseError", func() {
					containers, err := client.List(host)
					Expect(err).NotTo(BeNil())
					Expect(err).To(Equal(freighter.ResponseError))
					Expect(server.ReceivedRequests()).To(HaveLen(1))
					Expect(containers).To(BeNil())
				})
			})
		})

		Context("when the response is not HTTP 200 OK", func() {
			BeforeEach(func() {
				statusCode = http.StatusInternalServerError
			})

			It("returns a StatusError", func() {
				containers, err := client.List(host)
				Expect(err).NotTo(BeNil())
				_, ok := err.(*freighter.StatusError)
				Expect(ok).To(BeTrue())
				Expect(server.ReceivedRequests()).To(HaveLen(1))
				Expect(containers).To(BeNil())
			})
		})

		Context("when the request fails", func() {
			BeforeEach(func() {
				// Close server to simulate a failed request.
				server.Close()
			})

			It("returns a RequestFailedError", func() {
				containers, err := client.List(host)
				Expect(err).NotTo(BeNil())
				_, ok := err.(*freighter.RequestFailedError)
				Expect(ok).To(BeTrue())
				Expect(server.ReceivedRequests()).To(HaveLen(0))
				Expect(containers).To(BeNil())
			})
		})

		Context("when the request times out", func() {
			BeforeEach(func() {
				tr := &http.Transport{
					Dial: (&net.Dialer{
						Timeout: 1 * time.Nanosecond,
					}).Dial,
				}
				client = freighter.NewClient(&http.Client{Transport: tr})
			})

			It("returns a RequestFailedError", func() {
				containers, err := client.List(host)
				Expect(err).NotTo(BeNil())
				_, ok := err.(*freighter.RequestFailedError)
				Expect(ok).To(BeTrue())
				Expect(server.ReceivedRequests()).To(HaveLen(0))
				Expect(containers).To(BeNil())
			})
		})

		Context("when connecting to a Freighter host over TLS", func() {
			BeforeEach(func() {
				server = ghttp.NewTLSServer()
				server.AppendHandlers(handlers)

				ip, port := IPAndPort(server)
				host = &freighter.Host{IP: ip, Port: port, Secure: true}

				tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
				c := &http.Client{Transport: tr}
				client = freighter.NewClient(c)
			})

			Context("when the response is HTTP 200 OK", func() {
				BeforeEach(func() {
					statusCode = http.StatusOK
					responseBody = `{
						"containers": [
							{
								"id":"cafebabe",
								"name":"yosemite",
								"image":"freighter.yosemite",
								"host":"172.17.0.1",
								"ssh_port":60001,
								"state":"running"
							}
						]
					}`
				})

				It("returns a slice of Containers", func() {
					containers, err := client.List(host)
					Expect(err).To(BeNil())
					Expect(server.ReceivedRequests()).To(HaveLen(1))
					Expect(containers).To(HaveLen(1))

					Expect(containers[0].ID).To(Equal("cafebabe"))
					Expect(containers[0].Name).To(Equal("yosemite"))
					Expect(containers[0].Image).To(Equal("freighter.yosemite"))
					Expect(containers[0].Host).To(Equal("172.17.0.1"))
					Expect(containers[0].SSHPort).To(Equal(60001))
					Expect(containers[0].State).To(Equal("running"))
				})
			})
		})
	})
})
