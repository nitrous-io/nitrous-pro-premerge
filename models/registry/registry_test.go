package registry_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/registry"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/registry")
}

var _ = Describe("models/registry", func() {
	var (
		hs *HostStack
		db *db.Database
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAllByUserIDAndHostID()", func() {
		BeforeEach(func() {
			_, err := db.Exec(`INSERT INTO registries
				(user_id, host_id, url, username, email)
				VALUES ($1, $2, $3, $4, $5);`,
				hs.U.ID, hs.H.ID, "hub.example.com", "foo", "foo@example.com")
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO registries
				(user_id, host_id, url, username, email)
				VALUES ($1, $2, $3, $4, $5);`,
				hs.U.ID, hs.H.ID, "hub1.example.com", "foo1", "foo1@example.com")
			Expect(err).To(BeNil())
		})

		Context("when there's registry records for given user and host", func() {
			It("returns the registry records", func() {
				registries, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(registries).To(HaveLen(2))

				Expect(registries[0].ID).NotTo(BeZero())
				Expect(registries[0].UserID).To(Equal(hs.U.ID))
				Expect(registries[0].URL).To(Equal("hub.example.com"))
				Expect(registries[0].Username).To(Equal("foo"))
				Expect(registries[0].Email).To(Equal("foo@example.com"))

				Expect(registries[1].ID).NotTo(BeZero())
				Expect(registries[1].UserID).To(Equal(hs.U.ID))
				Expect(registries[1].URL).To(Equal("hub1.example.com"))
				Expect(registries[1].Username).To(Equal("foo1"))
				Expect(registries[1].Email).To(Equal("foo1@example.com"))
			})
		})

		Context("when there's no registry records for given user and host", func() {
			It("returns the registry records", func() {
				h2 := &host.Host{
					UserID:             hs.U.ID,
					ProviderTemplateID: hs.PT.ID,
				}
				err := h2.Insert(db)
				Expect(err).To(BeNil())

				registries, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, h2.ID)
				Expect(err).To(BeNil())
				Expect(registries).To(HaveLen(0))
			})
		})
	})

	Describe("FindByUserIDAndHostIDAndURL()", func() {
		Context("when a registry record exist for given user, host and registry URL", func() {
			BeforeEach(func() {
				_, err := db.Exec(`INSERT INTO registries
				(user_id, host_id, url, username, email)
				VALUES ($1, $2, $3, $4, $5);`,
					hs.U.ID, hs.H.ID, "hub.example.com", "foo", "foo@example.com")
				Expect(err).To(BeNil())
			})

			It("returns the registry record", func() {
				r, err := registry.FindByUserIDAndHostIDAndURL(db, hs.U.ID, hs.H.ID, "hub.example.com")
				Expect(err).To(BeNil())
				Expect(r).NotTo(BeNil())
				Expect(r.UserID).To(Equal(hs.U.ID))
				Expect(r.HostID).To(Equal(hs.H.ID))
				Expect(r.URL).To(Equal("hub.example.com"))
			})
		})

		Context("when a registry record does not exist for given user, host and registry URL", func() {
			It("returns nil", func() {
				r, err := registry.FindByUserIDAndHostIDAndURL(db, hs.U.ID, hs.H.ID, "hub.example.com")
				Expect(err).To(BeNil())
				Expect(r).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				r, err := registry.FindByUserIDAndHostIDAndURL(db, hs.U.ID, hs.H.ID, "hub.example.com")
				Expect(err).NotTo(BeNil())
				Expect(r).To(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var r *registry.Registry

		BeforeEach(func() {
			r = &registry.Registry{
				UserID:   hs.U.ID,
				HostID:   hs.H.ID,
				URL:      "hub.example.com",
				Username: "foo",
				Email:    "foo@example.com",
			}
		})

		Context("when no error happens", func() {
			It("persists the registry value in the database", func() {
				err := r.Insert(db)
				Expect(err).To(BeNil())

				rx, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(rx[0].ID).NotTo(BeZero())
				Expect(rx[0].UserID).To(Equal(hs.U.ID))
				Expect(rx[0].HostID).To(Equal(hs.H.ID))
				Expect(rx[0].URL).To(Equal("hub.example.com"))
				Expect(rx[0].Username).To(Equal("foo"))
				Expect(rx[0].Email).To(Equal("foo@example.com"))
				Expect(rx[0].CreatedAt).NotTo(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				err := r.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Delete()", func() {
		BeforeEach(func() {
			_, err := db.Exec(`INSERT INTO registries
				(user_id, host_id, url, username, email)
				VALUES ($1, $2, $3, $4, $5);`,
				hs.U.ID, hs.H.ID, "hub.example.com", "foo", "foo@example.com")
			Expect(err).To(BeNil())
		})

		Context("when theres a matching registry record", func() {
			It("deletes the matching record", func() {
				err := registry.Delete(db, hs.U.ID, hs.H.ID, "hub.example.com")
				Expect(err).To(BeNil())

				rx, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(rx).To(HaveLen(0))
			})
		})

		Context("when there's no matching registry record", func() {
			It("does not delete the matching record", func() {
				err := registry.Delete(db, hs.U.ID, hs.H.ID, "not-exist.example.com")
				Expect(err).To(BeNil())

				rx, err := registry.FindAllByUserIDAndHostID(db, hs.U.ID, hs.H.ID)
				Expect(err).To(BeNil())
				Expect(rx).To(HaveLen(1))
			})
		})
	})
})
