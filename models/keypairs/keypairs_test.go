package keypairs_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/nitrous/models/keypairs"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/keypairs")
}

var _ = Describe("models/keypairs", func() {
	Describe(".Create", func() {
		It("creates a keypair for the specified host", func() {
			db, _ := TestDatabase("nitrous_test")
			priv, _ := keypairs.GenerateKey()

			tx, _ := db.Beginx()
			kp1, err := keypairs.Create(tx, priv, "aes_key", 1)
			Expect(err).To(BeNil(), "should be able to create new keypair")
			tx.Commit()

			kp2, err := keypairs.Get(db, 1)
			Expect(err).To(BeNil(), "should be able to get an existing keypair")

			Expect(kp1).To(Equal(kp2), "should be the same keypair")
		})
	})

	Describe(".Get", func() {
		It("fails to get non-existing keypair", func() {
			db, _ := TestDatabase("nitrous_test")
			_, err := keypairs.Get(db, 1)
			Expect(err).NotTo(BeNil(), "should fail to find invalid keypair")
		})
	})

	Describe(".Delete", func() {
		It("deletes the keypairs belongs to a host", func() {
			db, _ := TestDatabase("nitrous_test")
			priv, _ := keypairs.GenerateKey()

			tx, _ := db.Beginx()
			kp, _ := keypairs.Create(tx, priv, "aes_key", 1)
			tx.Commit()

			tx, _ = db.Beginx()
			err := keypairs.Delete(tx, 1)
			Expect(err).To(BeNil(), "should delete the host's keypairs")
			tx.Commit()

			_, err = keypairs.Get(db, 1)
			Expect(err).ToNot(BeNil(), "should not find deleted keypairs")

			var terminated time.Time
			db.QueryRow("SELECT terminated_at, created_at FROM key_pairs WHERE id = ?", kp.ID).
				Scan(&terminated)
			Expect(terminated).ToNot(BeNil(), "should update terminated time")
		})
	})
})
