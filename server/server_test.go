package server_test

import (
	"net/http"
	"testing"

	"github.com/nitrous-io/nitrous/server"
	. "github.com/nitrous-io/nitrous/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "middleware")
}

var _ = Describe("server", func() {
	var r *http.Request
	var err error

	BeforeEach(func() {
		r, err = NewRequest("GET", "/test", nil)
		Expect(err).To(BeNil())
	})

	Describe("getRemoteAddr", func() {
		Context("with RemoteAddr set to IP:PORT", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10:63333"
			})

			It("should return the IP", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr set to multiple IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10:63333 202.157.130.9 202.157.130.202:60000"
			})

			It("should return the first IP address", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr set to multiple IP addresses some of which are 'proxy' IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000"
			})

			It("should return the IP address which is not a proxy", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.202"))
			})
		})

		Context("with RemoteAddr is not set, but X-Forwarded-For is set", func() {
			BeforeEach(func() {
				r.RemoteAddr = ""
				r.Header.Set("X-Forwarded-For", "202.157.130.10")
			})

			It("should return the IP address in the X-Forwarded-For header", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})

		Context("with RemoteAddr is not set, but X-Forwarded-For is set to multiple IP addresses, some of which are 'trusted' IP addresses", func() {
			BeforeEach(func() {
				r.RemoteAddr = ""
				r.Header.Set("X-Forwarded-For", "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000")
			})

			It("should return the IP address in the X-Forwarded-For header", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.202"))
			})
		})

		Context("with both RemoteAddr and X-Forwarded-For are set", func() {
			BeforeEach(func() {
				r.RemoteAddr = "202.157.130.10"
				r.Header.Set("X-Forwarded-For", "127.0.0.1:63333 192.168.212.122\t202.157.130.202:60000")
			})

			It("should return the IP address stored in RemoteAddr", func() {
				Expect(server.GetRemoteAddr(r)).To(Equal("202.157.130.10"))
			})
		})
	})
})
