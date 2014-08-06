package messageverifier_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/pkg/messageverifier"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "messageverifier")
}

var _ = Describe("messageverifier", func() {
	Describe("Generate", func() {
		verifier := messageverifier.New("secret")
		out, err := verifier.Generate(1, "host_created", map[string]interface{}{
			"id":   123,
			"slug": "foo",
		})
		Expect(err).To(BeNil())
		Expect(out).To(Equal("WzEsImhvc3RfY3JlYXRlZCIseyJpZCI6MTIzLCJzbHVnIjoiZm9vIn1d--67e1af939b5525c8815b3d7dbc22a4d281ea63cc"))
	})
})
