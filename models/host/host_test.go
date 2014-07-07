package host_test

import (
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/host")
}

var _ = Describe("models/host", func() {
	var (
		db *db.Database
		r  *region.Region
		p  *plan.Plan
		pt *providertemplate.ProviderTemplate
	)

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		r = &region.Region{Name: "apse1", Description: "Southeast Asia"}
		err = r.Insert(db)
		Expect(err).To(BeNil())

		p = &plan.Plan{Name: "micro", MaxContainers: 1, Price: 500}
		err = p.Insert(db)
		Expect(err).To(BeNil())

		pt = &providertemplate.ProviderTemplate{
			RegionID: r.ID,
			PlanID:   p.ID,
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
		err = pt.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAllByUser()", func() {
		var (
			u1 *user.User
			u2 *user.User
		)

		BeforeEach(func() {
			var err error
			u1 = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u1.Insert(db)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = u2.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
				(user_id, name, region_id, plan_id, provider_template_id)
				VALUES ($1, $2, $3, $4, $5);`,
				u1.ID, "my-host-1", r.ID, p.ID, pt.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts (user_id, name, region_id, plan_id, provider_template_id)
				VALUES ($1, $2, $3, $4, $5);`,
				u1.ID, "my-host-2", r.ID, p.ID, pt.ID)
			Expect(err).To(BeNil())
		})

		Context("when the given user has host records", func() {
			It("returns the host records", func() {
				hosts, err := host.FindAllByUser(db, u1)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].ID).NotTo(BeZero())
				Expect(hosts[0].UserID).To(Equal(u1.ID))
				Expect(hosts[0].Name).To(Equal("my-host-1"))

				Expect(hosts[1].ID).NotTo(BeZero())
				Expect(hosts[1].UserID).To(Equal(u1.ID))
				Expect(hosts[1].Name).To(Equal("my-host-2"))
			})
		})

		Context("when the given user does not have any host records", func() {
			It("returns an empty slice", func() {
				hosts, err := host.FindAllByUser(db, u2)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(0))
			})
		})
	})

	Describe("FindByUserAndName()", func() {
		var (
			u   *user.User
			err error
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u.Insert(db)
			Expect(err).To(BeNil())
		})

		Context("when there are multiple hosts for the given user and name", func() {
			var (
				h1 *host.Host
				h2 *host.Host
			)

			BeforeEach(func() {
				h1 = &host.Host{
					Name:               "foo",
					UserID:             u.ID,
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
					State:              "running",
				}

				err = h1.Insert(db)
				Expect(err).To(BeNil())

				h2 = &host.Host{
					Name:               "bar",
					UserID:             u.ID,
					RegionID:           r.ID,
					PlanID:             p.ID,
					ProviderTemplateID: pt.ID,
					State:              "stopped",
				}
				err = h2.Insert(db)
				Expect(err).To(BeNil())
			})

			It("returns the host record mathces the name", func() {
				h, err := host.FindByUserAndName(db, u, "foo")
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(h1.ID))
				Expect(h.UserID).To(Equal(u.ID))
				Expect(h.Name).To(Equal("foo"))
				Expect(h.RegionID).To(Equal(r.ID))
				Expect(h.State).To(Equal("running"))
			})
		})

		Context("when a host record for the given user and name does not exist", func() {
			It("returns nil without an error", func() {
				h, err := host.FindByUserAndName(db, u, "foo")
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				h, err := host.FindByUserAndName(db, u, "foo")
				Expect(h).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var (
			h *host.Host
			u *user.User
		)

		BeforeEach(func() {
			var err error
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u.Insert(db)
			Expect(err).To(BeNil())

			h = &host.Host{
				UserID:             u.ID,
				Name:               "my-host-1",
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("persists the host in the database", func() {
			hosts, err := host.FindAllByUser(db, u)
			Expect(err).To(BeNil())
			Expect(hosts).To(HaveLen(1))
			Expect(hosts[0].ID).NotTo(BeZero())
			Expect(hosts[0].UserID).To(Equal(u.ID))
			Expect(hosts[0].Name).To(Equal("my-host-1"))
			Expect(hosts[0].CreatedAt).NotTo(BeNil())
			Expect(hosts[0].UpdatedAt).NotTo(BeNil())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(h.ID).NotTo(BeZero())
			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("UpdateColumn()", func() {
		var (
			u   *user.User
			h   *host.Host
			err error
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u.Insert(db)
			Expect(err).To(BeNil())

			h = &host.Host{
				Name:               "foo",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
				State:              "running",
			}

			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the given column in host record", func() {
			h.Name = "bar"
			err := h.UpdateColumn(db, "name")

			h1, err := host.FindByUserAndName(db, u, "bar")
			Expect(err).To(BeNil())
			Expect(h1.ID).To(Equal(h.ID))
		})
	})

	Describe("Delete()", func() {
		var (
			u   *user.User
			h   *host.Host
			err error
		)

		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u.Insert(db)
			Expect(err).To(BeNil())

			h = &host.Host{
				Name:               "foo",
				UserID:             u.ID,
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
				State:              "running",
			}

			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("deletes the host record", func() {
			err := h.Delete(db)

			h, err = host.FindByUserAndName(db, u, "foo")
			Expect(err).To(BeNil())
			Expect(h).To(BeNil())
		})
	})

	Describe("Validate()", func() {
		var (
			h *host.Host
			u *user.User
		)

		BeforeEach(func() {
			var err error
			u = &user.User{
				Username: "foo",
				Email:    "foo@example.com",
				Password: "123456",
			}
			err = u.Insert(db)
			Expect(err).To(BeNil())

			h = &host.Host{
				UserID:             u.ID,
				Name:               "my-host-1",
				RegionID:           r.ID,
				PlanID:             p.ID,
				ProviderTemplateID: pt.ID,
			}
			Expect(h.Validate(db)).To(BeNil())
		})

		It("validates name", func() {
			buf := make([]byte, 64)
			for i := range buf {
				buf[i] = 'f'
			}
			s := string(buf) // "f" * 64.

			tests := []struct {
				input string
				err   error
			}{
				{"", host.ErrNameBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", host.ErrNameTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, host.ErrNameTooLong},
				{s[:63], nil},
				{"f00", nil},
				{"f-o-0-", nil},
				{"-foo", host.ErrNameInvalid},
				{"FOO", host.ErrNameInvalid},
			}

			for _, t := range tests {
				h.Name = t.input
				Expect(h.Validate(db) == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})
})
