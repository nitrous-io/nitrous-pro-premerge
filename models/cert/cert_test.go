package cert_test

import (
	"testing"
	"time"

	"github.com/nitrous-io/nitrous/models/cert"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func mkcert() *cert.Cert {
	return &cert.Cert{
		CA:         "ca",
		ServerCert: "servercert",
		ServerKey:  "serverkey",
	}
}

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/cert")
}

var _ = Describe("models/cert", func() {
	Describe(".Insert", func() {
		It("creates certificates for a host", func() {
			db, _ := TestDatabase("nitrous_test")
			cert1 := mkcert()

			tx, _ := db.Beginx()
			err := cert1.Insert(tx, 1)
			Expect(err).To(BeNil(), "failed to create new cert")
			tx.Commit()

			cert2, err := cert.Find(db, 1)
			Expect(err).To(BeNil(), "failed to find an existing cert")

			Expect(cert2).To(Equal(cert1), "created cert is not the same as stored cert")
		})
	})

	Describe(".Find", func() {
		It("fails to get non-existing cert", func() {
			db, _ := TestDatabase("nitrous_test")
			_, err := cert.Find(db, 1)
			Expect(err).NotTo(BeNil(), "should fail to find invalid cert")
		})
	})

	Describe(".Delete", func() {
		It("removes the host certificates", func() {
			db, _ := TestDatabase("nitrous_test")
			crt := mkcert()

			tx, _ := db.Beginx()
			crt.Insert(tx, 1)
			tx.Commit()

			tx, _ = db.Beginx()
			err := cert.Delete(tx, 1)
			Expect(err).To(BeNil(), "failed to delete the host's certs")
			tx.Commit()

			_, err = cert.Find(db, 1)
			Expect(err).ToNot(BeNil(), "should not find deleted certs")

			var terminated time.Time
			db.QueryRow("SELECT terminated_at FROM certs WHERE id = ?", crt.ID).
				Scan(&terminated)
			Expect(terminated).ToNot(BeNil(), "failed to update terminated time")
		})
	})
})
