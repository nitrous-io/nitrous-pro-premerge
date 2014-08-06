package shover_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/nitrous-io/nitrous/pkg/shover"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "shover")
}

// Implements shover.MessageSigner interface.
type fakeMessageSigner struct{}

func (s *fakeMessageSigner) Generate(data ...interface{}) (string, error) {
	hash, ok := data[len(data)-1].(map[string]interface{})
	Expect(ok).To(BeTrue(), `Expected last arg to be a map[string]interface{}`)
	ts, ok := hash["_t"]
	Expect(ok).To(BeTrue(), `Expected data hash to include a "_t" key`)
	Expect(ts).NotTo(BeZero())
	return fmt.Sprintf("message-length-%d", len(data)), nil
}

var _ = Describe("shover", func() {
	var (
		srv          *ghttp.Server
		statusCode   int
		responseBody string
		signer       = &fakeMessageSigner{}
	)

	Describe("Publish", func() {
		BeforeEach(func() {
			srv = ghttp.NewServer()
			srv.AppendHandlers(ghttp.CombineHandlers(
				func(w http.ResponseWriter, req *http.Request) {
					err := req.ParseForm()
					Expect(err).To(BeNil())

					Expect(req.PostFormValue("p")).To(Equal("message-length-3"))
				},
				ghttp.VerifyRequest("POST", "/publish"),
				ghttp.RespondWithPtr(&statusCode, &responseBody),
			))
		})

		AfterEach(func() {
			srv.Close()
		})

		It("posts to a Shover server without error", func() {
			statusCode = 200
			responseBody = `{"status": "ok"}`

			client, err := shover.NewClient(srv.URL(), signer, nil)
			Expect(err).To(BeNil())
			err = client.Publish(1, "host_created", map[string]interface{}{"id": 123})
			Expect(err).To(BeNil())
		})

		It("returns ErrInvalidPayload when Shover server responds with HTTP 400 Bad Request", func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"status": "invalid payload"}`

			client, err := shover.NewClient(srv.URL(), signer, nil)
			Expect(err).To(BeNil())
			err = client.Publish(1, "host_created", map[string]interface{}{"id": 123})
			Expect(err).To(MatchError(shover.ErrInvalidPayload))
		})
	})

	Describe("Broadcast", func() {
		BeforeEach(func() {
			srv = ghttp.NewServer()
			srv.AppendHandlers(ghttp.CombineHandlers(
				func(w http.ResponseWriter, req *http.Request) {
					err := req.ParseForm()
					Expect(err).To(BeNil())

					Expect(req.PostFormValue("p")).To(Equal("message-length-2"))
				},
				ghttp.VerifyRequest("POST", "/broadcast"),
				ghttp.RespondWithPtr(&statusCode, &responseBody),
			))
		})

		AfterEach(func() {
			srv.Close()
		})

		It("posts to a Shover server without error", func() {
			statusCode = 200
			responseBody = `{"status": "ok"}`

			client, err := shover.NewClient(srv.URL(), signer, nil)
			Expect(err).To(BeNil())
			err = client.Broadcast("broadcast_event", map[string]interface{}{"foo": "bar"})
			Expect(err).To(BeNil())
		})

		It("returns ErrInvalidPayload when Shover server responds with HTTP 400 Bad Request", func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"status": "invalid payload"}`

			client, err := shover.NewClient(srv.URL(), signer, nil)
			Expect(err).To(BeNil())
			err = client.Broadcast("broadcast_event", map[string]interface{}{"foo": "bar"})
			Expect(err).To(MatchError(shover.ErrInvalidPayload))
		})
	})
})
