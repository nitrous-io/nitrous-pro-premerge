package mergemap_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/pkg/mergemap"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "pkg/mergemap")
}

var _ = Describe("pkg/mergemap", func() {
	It("should merge two maps", func() {
		to := map[string]interface{}{"a": "1", "b": "2"}
		from := map[string]interface{}{"c": "3", "b": "2.1"}

		mergemap.Merge(from, to)
		Expect(to).To(Equal(map[string]interface{}{"a": "1", "b": "2.1", "c": "3"}))
	})
})
