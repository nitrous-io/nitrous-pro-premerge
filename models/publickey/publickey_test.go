package publickey_test

import (
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/publickey"
	"github.com/nitrous-io/nitrous/models/user"

	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestPublicKey(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/publickey")
}

var _ = Describe("models/publickey", func() {
	var (
		db *db.Database
		hs *HostStack
		c  *container.Container
	)

	validRSA := "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC6YIJRY7GTMUOHjoRDP4IXa5LdmKVYpW0ORccuRztsbBVcZhkm2GVrvOPwnUc53k0zUe+q95im+Yb3d5pATX27KG0VheDx7mz2l2xUb44VzdT6aMpYEsQREw5RrUnpzFQgPh295ZbVPTIGzq0+7LALlDMP5YpY8oYrY1gAtTEmivcffD/fQMmcExlRbWRBvDc/nfSoN5wSpG+sW32gQ/l5IWtmPlnhb4ze/D32YDL6OVK75tlFNeMk2pmPWejnw9GFOBXkXEpYVfmd5AHpJqc1WskOdv1ahXwM2n8pghkjw3eh9YDmpiLkxeJEpA+pNJA69NdF4fYJhpm2Nz2dIvwn"
	validECDSA := "ecdsa-sha2-nistp521 AAAAE2VjZHNhLXNoYTItbmlzdHA1MjEAAAAIbmlzdHA1MjEAAACFBAEJfzWE4kBhqfyJV5PGUhCyaJ9GB0RRrc3RsCweBLgLmbagOy4IwRtd79/AgxlmoZ/7e8eTNfAhE4vV6MXJoltIkAFz+0+NsAopvQEWWDHfADilCuTltegQn6r1f6iA5XgSoLJQvnJVaneo4UzClCHIVDZWQL1XvNlKKtxRHU0q0niElA=="

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		hs, err = CreateHostStack(db, nil)
		Expect(err).To(BeNil())

		c = &container.Container{
			ID:     "1234abc",
			UserID: hs.U.ID,
			HostID: hs.H.ID,
			Image:  "nitrous-base",
			Name:   "java-test-container",
		}
		err = c.Insert(db)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("FindAllByUser()", func() {
		var u2 *user.User

		BeforeEach(func() {

			_, err := db.Exec("INSERT INTO public_keys (user_id, container_id, key, fingerprint) VALUES ($1, $2, $3, $4);", hs.U.ID, c.ID, validRSA, "fingerprint")
			Expect(err).To(BeNil())

			u2 = &user.User{
				Username: "bar",
				Password: "123456",
				Email:    "bar@example.com",
			}
			err = u2.Insert(db)
			Expect(err).To(BeNil())

			_, err = db.Exec("INSERT INTO public_keys (user_id, container_id, key, fingerprint) VALUES ($1, $2, $3, $4);", u2.ID, c.ID, validECDSA, "fingerprint")
			Expect(err).To(BeNil())
		})

		Context("when public_key records with a given user_id exists", func() {
			It("finds public_keys by a given user", func() {
				keys, err := publickey.FindAllByUser(db, hs.U)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))

				Expect(keys[0].ID).To(Equal(hs.U.ID))
				Expect(keys[0].Key).To(Equal(validRSA))
			})
		})

		Context("when no public_key records with a given user_id exist", func() {
			It("returns empty slice", func() {
				keys, err := publickey.FindAllByUser(db, &user.User{ID: 999})
				Expect(err).To(BeNil())
				Expect(keys).To(BeEmpty())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				keys, err := publickey.FindAllByUser(db, &user.User{})
				Expect(keys).To(BeEmpty())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var pk *publickey.PublicKey

		BeforeEach(func() {
			pk = &publickey.PublicKey{
				UserID:      hs.U.ID,
				ContainerID: c.ID,
				Key:         validRSA,
			}
		})

		Context("when given params are valid", func() {
			It("creates a public key", func() {
				err := pk.Insert(db)
				Expect(err).To(BeNil())

				keys, err := publickey.FindAllByUserID(db, hs.U.ID)
				Expect(err).To(BeNil())
				Expect(keys).To(HaveLen(1))
				Expect(keys[0].UserID).To(Equal(hs.U.ID))
				Expect(keys[0].ContainerID).To(Equal(c.ID))
				Expect(keys[0].ID).NotTo(BeZero())
			})
		})

		Context("when given key is invalid", func() {
			It("returns error", func() {
				pk.Key = "ssh-rsa Not a key!!!"
				err := pk.Insert(db)
				Expect(err).To(Equal(publickey.ErrInvalidKey))
			})
		})
	})

	Describe("Destroy methods", func() {
		var (
			u, u2 *user.User
			// rsa1 and rsa2 belong to u, ecdsa belongs to u2
			rsa1, rsa2, ecdsa *publickey.PublicKey
		)
		BeforeEach(func() {
			u = &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			u2 = &user.User{
				Username: "bar",
				Password: "123456",
				Email:    "bar@example.com",
			}
			err = u2.Insert(db)
			Expect(err).To(BeNil())
			rsa1 = &publickey.PublicKey{
				UserID:      u.ID,
				ContainerID: c.ID,
				Key:         validRSA,
			}
			rsa2 = &publickey.PublicKey{
				UserID:      u.ID,
				ContainerID: c.ID,
				Key:         "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDFbXOqLy3jtavYmnuqrsjzf3Pf0Vsn77co4HC/Xvn7l0/JVpQ5YQBkcegGBZ3bVkg/5G8c6In1bPHiZ7EqKCJyml3dwrHuJn5webZ5ZZmmoLi/4COajym5nUDnYbCBpmocc4ydmZi8r5GNLGAPGYte4zWXapuzNjPzRCE2AP7kIEgbLfl6dzIrVeYwwiIChjY+6G2suRQNuTlHqGkIlnG1/2/Dp6pQYde2Itst3IfOip6syR4SBgke/t8Ys82v+LDtNadmPGXQjY1myZe6/uMPe0rI5SwOFvbm2Woruq/BndQIZKvcPRPfCglEOF4t2+iVjoQ7vBWwGRKapzu2qWKP someuser@somehost",
			}
			ecdsa = &publickey.PublicKey{
				UserID:      u2.ID,
				ContainerID: c.ID,
				Key:         validECDSA,
			}
			for _, k := range []*publickey.PublicKey{rsa1, rsa2, ecdsa} {
				err := k.Insert(db)
				Expect(err).To(BeNil())
			}
		})

		Describe("DestroyAllByUserID", func() {
			It("removes a user's keys", func() {
				pks, err := publickey.FindAllByUserID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(2))
				err = publickey.DestroyAllByUserID(db, u.ID)
				Expect(err).To(BeNil())
				pks, err = publickey.FindAllByUserID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(0))
			})

			It("does not remove other users' keys", func() {
				pks, err := publickey.FindAllByUserID(db, u2.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(1))
				err = publickey.DestroyAllByUserID(db, u.ID)
				Expect(err).To(BeNil())
				pks, err = publickey.FindAllByUserID(db, u2.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(1))
			})
		})

		Describe("DestroyByUserIDAndFingerprint", func() {
			It("removes a key with the right fingerprint", func() {
				rsa1print, err := rsa1.Md5Fingerprint()
				Expect(err).To(BeNil())
				err = publickey.DestroyByUserIDAndFingerprint(db, u.ID, rsa1print)
				Expect(err).To(BeNil())

				pks, err := publickey.FindAllByUserID(db, u.ID)
				Expect(err).To(BeNil())
				Expect(pks).To(HaveLen(1))
				Expect(pks[0].Key).To(Equal(rsa2.Key))
			})
		})
	})

	Describe("Validate()", func() {
		var pk *publickey.PublicKey

		BeforeEach(func() {
			u := &user.User{
				Username: "foo",
				Password: "123456",
				Email:    "foo@example.com",
			}
			err := u.Insert(db)
			Expect(err).To(BeNil())
			pk = &publickey.PublicKey{
				UserID: u.ID,
				Key:    validRSA,
			}
			Expect(pk.Validate()).To(BeNil())
		})

		It("validates key", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", publickey.ErrKeyBlank},
			}
			for _, t := range tests {
				pk.Key = t.input
				Expect(pk.Validate()).To(Equal(t.err))
			}
		})
	})
})
