package freighter_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/pkg/freighter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestHost(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "freighter/host")
}

var _ = Describe("freighter/host", func() {
	Describe("NewHost()", func() {
		It("sets port to 65343 by default", func() {
			h := freighter.NewHost("127.0.0.1", 0, false)
			Expect(h.Port).To(Equal(65343))
		})
	})

	Describe("BaseURL()", func() {
		It("returns a URL based on its IP, port and whether it's secure", func() {
			tests := []struct {
				ip       string
				port     int
				secure   bool
				expected string
			}{
				{"127.0.0.1", 8888, false, "http://127.0.0.1:8888"},
				{"127.0.0.1", 8888, true, "https://127.0.0.1:8888"},
				{"127.0.0.1", 0, false, "http://127.0.0.1:65343"},
				{"1.2.3.4", 60000, true, "https://1.2.3.4:60000"},
				{"hostname", 60000, true, "https://hostname:60000"},
			}

			var h *freighter.Host
			for _, t := range tests {
				h = freighter.NewHost(t.ip, t.port, t.secure)
				Expect(h.BaseURL()).To(Equal(t.expected))
			}
		})
	})
})
