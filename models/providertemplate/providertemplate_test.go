package providertemplate_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/providertemplate")
}

var _ = Describe("models/providertemplate", func() {
	var (
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO provider_templates (name, provider, config) values ($1, $2, $3);", "do-512MB", "digitalocean", "")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO provider_templates (name, provider, config) values ($1, $2, $3);", "do-1GB", "digitalocean", "")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindByID()", func() {
		var ptID int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM provider_templates").Scan(&ptID)
			Expect(err).To(BeNil())
		})

		Context("when a provider template record with a given id exists", func() {
			It("returns the provider template record", func() {
				pt, err := providertemplate.FindByID(db, ptID)
				Expect(err).To(BeNil())
				Expect(pt).NotTo(BeNil())
				Expect(pt.ID).To(Equal(ptID))
			})
		})

		Context("when a provider template record with a given id does not exist", func() {
			It("returns nil", func() {
				pt, err := providertemplate.FindByID(db, 9001)
				Expect(err).To(BeNil())
				Expect(pt).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				pt, err := providertemplate.FindByID(db, ptID)
				Expect(err).NotTo(BeNil())
				Expect(pt).To(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var pt *providertemplate.ProviderTemplate

		BeforeEach(func() {
			var err error
			pt = &providertemplate.ProviderTemplate{
				Name:     "ec2-micro-apse1",
				Provider: "ec2",
			}
			err = pt.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the provider template in the database", func() {
			pt2, err := providertemplate.FindByID(db, pt.ID)
			Expect(err).To(BeNil())
			Expect(pt2).NotTo(BeNil())
			Expect(pt2.ID).NotTo(BeZero())
			Expect(pt2.Name).To(Equal("ec2-micro-apse1"))
			Expect(pt2.Provider).To(Equal("ec2"))
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(pt.ID).NotTo(BeZero())
			Expect(pt.CreatedAt).NotTo(BeNil())
			Expect(pt.UpdatedAt).NotTo(BeNil())
		})
	})
})
