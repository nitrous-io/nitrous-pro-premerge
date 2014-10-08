package volume_test

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/cvolume"
	"github.com/nitrous-io/nitrous/models/volume"

	ndb "github.com/nitrous-io/nitrous/db"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/volume")
}

var _ = Describe("models/volume", func() {
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

	newVolume := func(slug string) *volume.Volume {
		return &volume.Volume{
			HostID: hs.H.ID,
			Slug:   slug,
		}
	}

	newHostVolume := func(slug string) (*HostStack, *volume.Volume) {
		hs2, err := CreateHostStack(db, nil)
		Expect(err).To(BeNil())
		v := newVolume("data") // same slug
		v.HostID = hs2.H.ID
		err = v.Insert(db)
		Expect(err).To(BeNil())
		Expect(v.ID).NotTo(BeZero())
		return hs2, v
	}

	Describe("Insert()", func() {
		It("persists the volume in the database", func() {
			v := &volume.Volume{
				HostID: hs.H.ID,
				Slug:   "docs",
				Name:   "documents",
				System: true,
			}
			err := v.Insert(db)
			Expect(err).To(BeNil())
			Expect(v.ID).NotTo(BeZero())
			Expect(v.HostID).To(Equal(hs.H.ID))
			Expect(v.Slug).To(Equal("docs"))
			Expect(v.Name).To(Equal("documents"))
			Expect(v.System).To(Equal(true))
			Expect(v.CreatedAt).NotTo(BeNil())
			Expect(v.UpdatedAt).NotTo(BeNil())
		})
	})

	Describe("Delete()", func() {
		var v *volume.Volume

		BeforeEach(func() {
			v = newVolume("data")
			err := v.Insert(db)
			Expect(err).To(BeNil())
			Expect(v.ID).NotTo(BeZero())
		})

		It("removes the volume", func() {
			id := v.ID
			err := v.Delete(db)
			Expect(err).To(BeNil())

			err = db.Get(v, `SELECT * FROM volumes WHERE id = $1`, id)
			Expect(err).To(MatchError(sql.ErrNoRows))
		})

		It("removes cvolume association if present", func() {
			c := &container.Container{
				CID:    "cafebabe",
				UserID: hs.U.ID,
				HostID: hs.H.ID,
				Image:  "nitrous-base",
				Slug:   "java-test-container",
			}
			err := c.Insert(db)
			Expect(err).To(BeNil())
			cv := &cvolume.ContainerVolume{ContainerID: c.ID, VolumeID: v.ID}
			err = cv.Insert(db)
			Expect(err).To(BeNil())

			vid := v.ID
			err = v.Delete(db)
			Expect(err).To(BeNil())

			err = db.Get(v, `SELECT * FROM volumes WHERE id = $1`, vid)
			Expect(err).To(MatchError(sql.ErrNoRows))

			cv = &cvolume.ContainerVolume{}
			err = db.Get(cv, `SELECT * FROM container_volumes WHERE volume_id = $1`, vid)
			Expect(err).To(MatchError(sql.ErrNoRows))
		})
	})

	Describe("Find()", func() {
		Context("when an ID is given", func() {
			It("returns the matching record scoped by host", func() {
				v := newVolume("data")
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())
				hs2, v2 := newHostVolume("data")

				foundV, err := volume.Find(db, hs.H.ID, strconv.FormatInt(v.ID, 10))
				Expect(err).To(BeNil())
				Expect(foundV.ID).To(Equal(v.ID))

				foundV2, err := volume.Find(db, hs2.H.ID, strconv.FormatInt(v2.ID, 10))
				Expect(err).To(BeNil())
				Expect(foundV2.ID).To(Equal(v2.ID))
			})

			It("returns (nil, nil) on not found", func() {
				v, err := volume.Find(db, 12345, "10001")
				Expect(err).To(BeNil())
				Expect(v).To(BeNil())
			})
		})

		Context("when a slug is given", func() {
			It("returns the matching record scoped by host", func() {
				v := newVolume("data")
				err := v.Insert(db)
				Expect(err).To(BeNil())
				Expect(v.ID).NotTo(BeZero())
				hs2, v2 := newHostVolume("data")

				foundV, err := volume.Find(db, hs.H.ID, "data")
				Expect(err).To(BeNil())
				Expect(foundV.ID).To(Equal(v.ID))

				foundV2, err := volume.Find(db, hs2.H.ID, "data")
				Expect(err).To(BeNil())
				Expect(foundV2.ID).To(Equal(v2.ID))
			})

			It("returns (nil, nil) on not found", func() {
				v, err := volume.Find(db, 12345, "nope")
				Expect(err).To(BeNil())
				Expect(v).To(BeNil())
			})
		})
	})

	Describe("FindAll()", func() {
		It("finds all volumes scoped by host", func() {
			v := newVolume("data")
			err := v.Insert(db)
			Expect(err).To(BeNil())
			Expect(v.ID).NotTo(BeZero())
			hs2, v2 := newHostVolume("data")

			foundV, err := volume.FindAll(db, hs.H.ID)
			Expect(err).To(BeNil())
			Expect(foundV).To(HaveLen(1))
			Expect(foundV[0].ID).To(Equal(v.ID))
			Expect(foundV[0].HostID).To(Equal(hs.H.ID))

			foundV2, err := volume.FindAll(db, hs2.H.ID)
			Expect(err).To(BeNil())
			Expect(foundV2).To(HaveLen(1))
			Expect(foundV2[0].ID).To(Equal(v2.ID))
			Expect(foundV2[0].HostID).To(Equal(hs2.H.ID))
		})

		It("returns empty slice on not found", func() {
			v, err := volume.FindAll(db, 12345)
			Expect(err).To(BeNil())
			Expect(v).To(BeEmpty())
		})
	})

	Describe("Validate()", func() {
		It("validates slug", func() {
			s := strings.Repeat("a", 128)

			tests := []struct {
				input string
				err   error
			}{
				{"", volume.ErrSlugBlank},
				{"f", volume.ErrSlugTooShort},
				{s, volume.ErrSlugTooLong},
				{s[:127], nil},
				{"f00", nil},
				{"f-o-0-", nil},
				{"-foo", volume.ErrSlugInvalid},
				{"foo/bar", volume.ErrSlugInvalid},
			}

			for _, t := range tests {
				v := &volume.Volume{Slug: t.input}
				Expect(v.Validate() == t.err).To(BeTrue(),
					"Expected '%s' to return error '%v'", t.input, t.err)
			}
		})
	})
})
