package user_test

import (
	"encoding/json"
	"testing"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	. "github.com/nitrous-io/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "models/user")
}

var _ = Describe("models/user", func() {
	var db *db.Database

	BeforeEach(func() {
		var err error
		db, err = TestDatabase("nitrous_test")
		Expect(err).To(BeNil())

		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "foo", "foo@example.com", "foo bar", "foofoo")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "bar", "bar@example.com", "bar baz", "barbar")
		Expect(err).To(BeNil())
		_, err = db.Exec("INSERT INTO users (username, email, name, encrypted_password) values ($1, $2, $3, $4);", "baz", "baz@example.com", "baz qux", "bazbaz")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Encode as JSON", func() {
		It("returns the JSON record", func() {
			u, _ := user.FindByUsername(db, "foo")
			d, _ := json.Marshal(u)
			r := make(map[string]interface{})
			json.Unmarshal(d, &r)
			exposed := []string{"username", "email", "created_at", "updated_at"}
			for _, p := range exposed {
				Expect(r[p]).NotTo(BeNil(), "User JSON should contain %s", p)
				delete(r, p)
			}
			Expect(r).To(BeEmpty(), "Exposed more than what we need %v", r)
		})
	})

	Describe("FindByID()", func() {
		var fooID, barID, bazID int64

		BeforeEach(func() {
			err := db.QueryRow("SELECT id FROM users WHERE username = $1", "foo").Scan(&fooID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "bar").Scan(&barID)
			Expect(err).To(BeNil())
			err = db.QueryRow("SELECT id FROM users WHERE username = $1", "baz").Scan(&bazID)
			Expect(err).To(BeNil())
		})

		Context("when a user record with a given id exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByID(db, fooID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(fooID))
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))

				u, err = user.FindByID(db, barID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(barID))
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))

				u, err = user.FindByID(db, bazID)
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).To(Equal(bazID))
				Expect(u.Username).To(Equal("baz"))
				Expect(u.Email).To(Equal("baz@example.com"))
				Expect(u.Name).To(Equal("baz qux"))
				Expect(u.EncryptedPassword).To(Equal("bazbaz"))
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByUsername(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByUsername(db, "foo")
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("FindByUsername()", func() {
		Context("when a user record with a given username exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByUsername(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))

				u, err = user.FindByUsername(db, "bar")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))

				u, err = user.FindByUsername(db, "baz")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("baz"))
				Expect(u.Email).To(Equal("baz@example.com"))
				Expect(u.Name).To(Equal("baz qux"))
				Expect(u.EncryptedPassword).To(Equal("bazbaz"))
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByUsername(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByUsername(db, "foo")
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Insert()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
		})

		Context("when no error happens", func() {
			var u2 *user.User

			BeforeEach(func() {
				err := u.Insert(db)
				Expect(err).To(BeNil())
				u2, err = user.FindByUsername(db, "qux")
				Expect(err).To(BeNil())
				Expect(u2).NotTo(BeNil())
			})

			It("persists the user in the database", func() {
				Expect(u2.Username).To(Equal("qux"))
				Expect(u2.Name).To(Equal("quxboy"))
				Expect(u2.Email).To(Equal("qux@example.com"))
				Expect(u2.CreatedAt).NotTo(BeNil())
				Expect(u2.UpdatedAt).NotTo(BeNil())
			})

			It("encrypts the password", func() {
				Expect(u2.EncryptedPassword).NotTo(Equal("123456"))
				Expect(bcrypt.Verify(u2.EncryptedPassword, "123456")).To(BeTrue())
			})

			It("fills up ID and other fields in original struct", func() {
				Expect(u.ID).NotTo(BeZero())
				Expect(u.CreatedAt).NotTo(BeNil())
				Expect(u.UpdatedAt).NotTo(BeNil())
				Expect(u.Password).To(BeEmpty()) // clears unencrypted password
				Expect(u.EncryptedPassword).NotTo(BeEmpty())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				err := u.Insert(db)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Validate()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Email:    "foo@gmail.com",
				Username: "foo",
				Password: "123456",
			}
			Expect(u.Validate()).To(BeNil())
		})

		It("validates email", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", user.ErrEmailBlank},
				{"foo", user.ErrEmailInvalid},
				{"foo@gmail.com", nil},
				{"foo+bar@gmail.com", nil},
				{"foo+bar@hello.com.sg", nil},
			}

			for _, t := range tests {
				u.Email = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates username", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", user.ErrUsernameBlank},
				{"foo", nil},
				{"fo", nil},
				{"f", user.ErrUsernameTooShort},
				{"ffffffffffffffffffffffffffffff", nil},
				{"fffffffffffffffffffffffffffffff", user.ErrUsernameTooLong},
				{"f00", nil},
				{"f-o-0-", nil},
				{"-foo", user.ErrUsernameInvalid},
				{"FOO", user.ErrUsernameInvalid},
			}

			for _, t := range tests {
				u.Username = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}
		})

		It("validates password", func() {
			tests := []struct {
				input string
				err   error
			}{
				{"", user.ErrPasswordBlank},
				{"12345", user.ErrPasswordTooShort},
				{"123456", nil},
				{"fQ@*($&($()ZXv897ua]oo", nil},
				{"ffffffffffffffffffffffffffffff", nil},
				{"fffffffffffffffffffffffffffffff", user.ErrPasswordTooLong},
			}

			for _, t := range tests {
				u.Password = t.input
				Expect(u.Validate() == t.err).To(BeTrue())
			}

			u.Password = ""
			u.EncryptedPassword = "something"
			Expect(u.Validate()).To(BeNil())
		})
	})
})
