package host_test

import (
	"database/sql"
	"strconv"
	"testing"

	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
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
		db *ndb.Database
		hs *HostStack
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
			err = WrapInTransaction(db, u1.Insert)
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Email:    "bar@example.com",
				Password: "123456",
			}
			err = WrapInTransaction(db, u2.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`INSERT INTO hosts
				(user_id, slug, region_id, plan_id, provider_template_id)
				VALUES ($1, $2, $3, $4, $5);`,
				u1.ID, "my-host-1", hs.R.ID, hs.P.ID, hs.PT.ID)
			Expect(err).To(BeNil())
			_, err = db.Exec(`INSERT INTO hosts (user_id, slug, region_id, plan_id, provider_template_id)
				VALUES ($1, $2, $3, $4, $5);`,
				u1.ID, "my-host-2", hs.R.ID, hs.P.ID, hs.PT.ID)
			Expect(err).To(BeNil())
		})

		Context("when the given user has host records", func() {
			It("returns the host records", func() {
				hosts, err := host.FindAllByUser(db, u1)
				Expect(err).To(BeNil())
				Expect(hosts).To(HaveLen(2))

				Expect(hosts[0].ID).NotTo(BeZero())
				Expect(hosts[0].UserID).To(Equal(u1.ID))
				Expect(hosts[0].Slug).To(Equal("my-host-1"))

				Expect(hosts[1].ID).NotTo(BeZero())
				Expect(hosts[1].UserID).To(Equal(u1.ID))
				Expect(hosts[1].Slug).To(Equal("my-host-2"))
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

	Describe("Find()", func() {
		Context("when given param is an ID", func() {
			It("returns the matching record belongs to the user", func() {
				h, err := host.Find(db, hs.U, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
			})

			It("returns nil without an error when the matching record does not belong to the user", func() {
				u := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				h, err := host.Find(db, u, strconv.FormatInt(hs.H.ID, 10))
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})

			It("returns nil without an error when there's no matching record", func() {
				h, err := host.Find(db, hs.U, "10001")
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})
		})

		Context("when given param is a slug", func() {
			It("returns the mathching host record belongs to the user", func() {
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
				Expect(h.UserID).To(Equal(hs.U.ID))
				Expect(h.Slug).To(Equal(hs.H.Slug))
				Expect(h.Provider).To(Equal(hs.PT.Provider))
			})

			It("returns nil without an error when matching record does not belong to the user", func() {
				u := &user.User{
					Username: "barbaz",
					Email:    "barbaz@example.com",
					Password: "123456",
				}
				err := WrapInTransaction(db, u.Insert)
				Expect(err).To(BeNil())

				h, err := host.Find(db, u, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})

			It("returns nil without an error when there's no matching record'", func() {
				h, err := host.Find(db, hs.U, "vivabrazil")
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				h, err := host.Find(db, hs.U, hs.H.Slug)
				Expect(h).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByID()", func() {
		var id int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM hosts WHERE slug = $1;", hs.H.Slug).Scan(&id)
			Expect(err).To(BeNil())
			Expect(id).NotTo(BeZero())
		})

		Context("when a host record with a given id exists", func() {
			It("returns the host record", func() {
				h, err := host.FindByID(db, id)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(id))
			})
		})

		Context("when a host record with a given id does not exist", func() {
			It("returns nil", func() {
				h, err := host.FindByID(db, 9001)
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
			})
		})
	})

	Describe("FindBySlug()", func() {
		Context("when a host record with a given slug exists", func() {
			It("returns the host record", func() {
				h, err := host.FindBySlug(db, hs.H.Slug)
				Expect(err).To(BeNil())
				Expect(h).NotTo(BeNil())
				Expect(h.ID).To(Equal(hs.H.ID))
			})
		})

		Context("when a host record with a given slug oes not exist", func() {
			It("returns nil", func() {
				h, err := host.FindBySlug(db, "deadbeefhost")
				Expect(err).To(BeNil())
				Expect(h).To(BeNil())
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
			err = WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			h = &host.Host{
				UserID:             u.ID,
				Slug:               "my-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
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
			Expect(hosts[0].Slug).To(Equal("my-host-1"))
			Expect(hosts[0].CreatedAt).NotTo(BeNil())
			Expect(hosts[0].UpdatedAt).NotTo(BeNil())
		})

		It("fills up ID and other fields in original struct", func() {
			Expect(h.ID).NotTo(BeZero())
			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})

		It("sets state to a default of 'born'", func() {
			Expect(h.State).To(Equal("born"))
		})

		It("generates a new secret", func() {
			Expect(h.Secret).NotTo(BeEmpty())
			Expect(h.Secret).To(HaveLen(255))

			h2 := &host.Host{
				UserID:             u.ID,
				Slug:               "my-host-2",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			err := h2.Insert(db)
			Expect(err).To(BeNil())
			Expect(h2.Secret).NotTo(BeEmpty())
			Expect(h2.Secret).To(HaveLen(255))
			Expect(h2.Secret).NotTo(Equal(h.Secret))
		})
	})

	Describe("Update()", func() {
		var (
			h    *host.Host
			slug = "foo"
		)

		BeforeEach(func() {
			var err error
			h = &host.Host{
				Slug:               slug,
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				TerminatedAt:       nil,
			}
			err = h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("updates the host in the database", func() {
			h.Name = sql.NullString{"foo host", true}
			h.State = host.StateRestarting
			h.IPAddress = sql.NullString{"1.2.3.4", true}
			h.ProviderIdentifier = sql.NullString{"digitalocean", true}
			h.MaxContainers = sql.NullInt64{2, true}
			h.FreighterdPort = sql.NullInt64{60000, true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			h2, err := host.Find(db, hs.U, slug)
			Expect(err).To(BeNil())
			Expect(h2).NotTo(BeNil())
			Expect(h2.Slug).To(Equal(slug))

			// Updated fields.
			Expect(h2.Name.String).To(Equal("foo host"))
			Expect(h2.State).To(Equal(host.StateRestarting))
			Expect(h2.IPAddress.String).To(Equal("1.2.3.4"))
			Expect(h2.ProviderIdentifier.String).To(Equal("digitalocean"))
			Expect(h2.MaxContainers.Int64).To(Equal(int64(2)))
			Expect(h2.FreighterdPort.Int64).To(Equal(int64(60000)))

			// Unchanged fields.
			Expect(h2.UserID).To(Equal(hs.U.ID))
			Expect(h2.RegionID).To(Equal(hs.R.ID))
			Expect(h2.PlanID).To(Equal(hs.P.ID))
			Expect(h2.ProviderTemplateID).To(Equal(hs.PT.ID))
		})

		It("fills up the fields in original struct", func() {
			h.Name = sql.NullString{"foo host", true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			Expect(h.CreatedAt).NotTo(BeNil())
			Expect(h.UpdatedAt).NotTo(BeNil())
		})

		It("updates UpdatedAt", func() {
			origUpdatedAt := h.UpdatedAt

			h.Name = sql.NullString{"foo host", true}
			err := h.Update(db)
			Expect(err).To(BeNil())

			Expect(h.UpdatedAt).NotTo(Equal(origUpdatedAt))
		})
	})

	Describe("Delete()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			h = &host.Host{
				Slug:               "foo",
				UserID:             hs.U.ID,
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
				State:              host.StateRunning,
				TerminatedAt:       nil,
			}
			err := h.Insert(db)
			Expect(err).To(BeNil())
		})

		It("sets terminated_at instead of removing the host from the database", func() {
			err := h.Delete(db)
			Expect(err).To(BeNil())
			Expect(h.TerminatedAt).NotTo(BeNil())
			Expect(h.State).To(Equal(host.StateTerminated))

			h2 := &host.Host{}
			err = db.QueryRowx("SELECT * FROM hosts WHERE id = $1", h.ID).StructScan(h2)
			Expect(err).To(BeNil())
			Expect(h2).To(Equal(h))
		})
	})

	Describe("Validate()", func() {
		var (
			h *host.Host
		)

		BeforeEach(func() {
			h = &host.Host{
				UserID:             hs.U.ID,
				Slug:               "my-host-1",
				RegionID:           hs.R.ID,
				PlanID:             hs.P.ID,
				ProviderTemplateID: hs.PT.ID,
			}
			Expect(h.Validate()).To(BeNil())
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
				{"", host.ErrSlugBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", host.ErrSlugTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{s, host.ErrSlugTooLong},
				{s[:63], nil},
				{"f00", nil},
				{"f-o-0", nil},
				{"f-o-0-", host.ErrSlugInvalid},
				{"-foo", host.ErrSlugInvalid},
				{"foo_bar", host.ErrSlugInvalid},
				{"FOO", host.ErrSlugInvalid},
			}

			for _, t := range tests {
				h.Slug = t.input
				Expect(h.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})

	Describe(".Hostname", func() {
		It("return the host DNS name", func() {
			host.SetHostnames("n2otest.com", "n2otestproxy.com")
			h := &host.Host{Slug: "hello"}
			Expect(h.Hostname()).To(Equal("hello.n2otest.com"))
		})
	})

	Describe(".ProxyHostname", func() {
		It("return the host DNS name", func() {
			host.SetHostnames("n2otest.com", "n2otestproxy.com")
			h := &host.Host{Slug: "world"}
			Expect(h.ProxyHostname()).To(Equal("world.n2otestproxy.com"))
		})
	})
})
