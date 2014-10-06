package user_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/models/userprofile"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
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
			exposed := []string{"id", "name", "username", "email", "created_at", "updated_at"}
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

		Context("when a user record with a given id does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByID(db, 9001)
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByID(db, fooID)
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

	Describe("FindByUsernameOrEmail()", func() {
		Context("when a user record with a given username or email exists", func() {
			It("returns the user record", func() {
				u, err := user.FindByUsernameOrEmail(db, "foo")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("foo"))
				Expect(u.Email).To(Equal("foo@example.com"))
				Expect(u.Name).To(Equal("foo bar"))
				Expect(u.EncryptedPassword).To(Equal("foofoo"))

				u, err = user.FindByUsernameOrEmail(db, "bar@example.com")
				Expect(err).To(BeNil())
				Expect(u).NotTo(BeNil())
				Expect(u.ID).NotTo(BeZero())
				Expect(u.Username).To(Equal("bar"))
				Expect(u.Email).To(Equal("bar@example.com"))
				Expect(u.Name).To(Equal("bar baz"))
				Expect(u.EncryptedPassword).To(Equal("barbar"))
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns nil", func() {
				u, err := user.FindByUsernameOrEmail(db, "bat")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())

				u, err = user.FindByUsernameOrEmail(db, "bat@example.com")
				Expect(u).To(BeNil())
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				u, err := user.FindByUsernameOrEmail(db, "foo")
				Expect(u).To(BeNil())
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("GetLoginAttemptsByUsername", func() {
		Context("when a user record with a given username or email exists", func() {
			BeforeEach(func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE users SET login_attempts = 93 WHERE email = $1", "bar@example.com")
				Expect(err).To(BeNil())
			})

			AfterEach(func() {
				_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
				Expect(err).To(BeNil())
				_, err = db.Exec("UPDATE users SET login_attempts = 0 WHERE email = $1", "bar@example.com")
				Expect(err).To(BeNil())
			})

			It("returns the initial value", func() {
				value, err := user.GetLoginAttempts(db, "foo")
				Expect(err).To(BeNil())
				Expect(value).To(Equal(0))

				value, err = user.GetLoginAttempts(db, "bar@example.com")
				Expect(err).To(BeNil())
				Expect(value).To(Equal(93))
			})
		})

		Context("when a user record with a given username does not exist", func() {
			It("returns nil", func() {
				value, err := user.GetLoginAttempts(db, "bat")
				Expect(value).To(Equal(0))
				Expect(err).To(BeNil())

				value, err = user.GetLoginAttempts(db, "bat@example.com")
				Expect(value).To(Equal(0))
				Expect(err).To(BeNil())
			})
		})

		Context("when some error happens", func() {
			It("returns error", func() {
				db.Close()
				value, err := user.GetLoginAttempts(db, "foo")
				Expect(value).To(Equal(0))
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("IncrementLoginAttempts", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 93 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 0 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		It("should allow for incrementing", func() {
			err := user.IncrementLoginAttempts(db, "foo")
			Expect(err).To(BeNil())

			err = user.IncrementLoginAttempts(db, "bar@example.com")
			Expect(err).To(BeNil())

			value, err := user.GetLoginAttempts(db, "foo")
			Expect(value).To(Equal(1))
			Expect(err).To(BeNil())

			value, err = user.GetLoginAttempts(db, "bar@example.com")
			Expect(value).To(Equal(94))
			Expect(err).To(BeNil())
		})

		It("should not allow for overflows on the smallint", func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 32767 WHERE username = $1", "foo")
			Expect(err).To(BeNil())

			err = user.IncrementLoginAttempts(db, "foo")
			Expect(err).To(BeNil())

			value, err := user.GetLoginAttempts(db, "foo")
			Expect(value).To(Equal(32767))
			Expect(err).To(BeNil())
		})
	})

	Describe("ResetLoginAttempts", func() {
		BeforeEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 92 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 93 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			_, err := db.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1", "foo")
			Expect(err).To(BeNil())
			_, err = db.Exec("UPDATE users SET login_attempts = 0 WHERE email = $1", "bar@example.com")
			Expect(err).To(BeNil())
		})

		It("should allow for resetting", func() {
			err := user.ResetLoginAttempts(db, "foo")
			Expect(err).To(BeNil())

			err = user.ResetLoginAttempts(db, "bar@example.com")
			Expect(err).To(BeNil())

			value, err := user.GetLoginAttempts(db, "foo")
			Expect(value).To(Equal(0))
			Expect(err).To(BeNil())

			value, err = user.GetLoginAttempts(db, "bar@example.com")
			Expect(value).To(Equal(0))
			Expect(err).To(BeNil())
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
				err := WrapInTransaction(db, u.Insert)
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

			It("creates a user profile record", func() {
				up := &userprofile.UserProfile{}
				err := db.Get(up, "SELECT * FROM user_profiles WHERE user_profiles.user_id = $1;", u.ID)
				Expect(err).To(BeNil())
				Expect(up.ID).NotTo(BeNil())
				Expect(up.UserID).To(Equal(u.ID))
				Expect(up.MaxHosts).To(Equal(int64(1)))
				Expect(up.CreatedAt).NotTo(BeNil())
				Expect(up.UpdatedAt).NotTo(BeNil())
			})
		})

		Context("when some error occurrs during the transaction", func() {
			It("returns an the error", func() {
				tx, err := db.Beginx()
				Expect(err).To(BeNil())

				err = tx.Rollback()
				Expect(err).To(BeNil())

				err = u.Insert(tx)
				Expect(err).NotTo(BeNil())
			})
		})
	})

	Describe("Update()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())
		})

		It("updates name, email, encrypted_password, updated_at", func() {
			u.Username = "nochange"
			u.Name = "qux jr"
			u.Password = "abcdef"
			u.Email = "quxjr@example.com"
			oldCreated, oldUpdated := u.CreatedAt, u.UpdatedAt
			t := time.Now()
			u.CreatedAt = &t

			Expect(u.Update(db)).To(BeNil())
			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(updated.Username).To(Equal("qux"))
			Expect(updated.Name).To(Equal("qux jr"))
			Expect(updated.Email).To(Equal("quxjr@example.com"))
			Expect(updated.Password).To(BeEmpty())
			Expect(bcrypt.Verify(updated.EncryptedPassword, "abcdef")).To(BeTrue())
			Expect(updated.CreatedAt).To(Equal(oldCreated))
			Expect(*updated.UpdatedAt).To(BeTemporally(">", *oldUpdated))
		})

		It("does not update encrypted password if password left blank", func() {
			u.Password = ""
			u.Email = "quxnopass@example.com"
			Expect(u.Update(db)).To(BeNil())
			updated, err := user.FindByID(db, u.ID)
			Expect(err).To(BeNil())
			Expect(bcrypt.Verify(updated.EncryptedPassword, "123456")).To(BeTrue())
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
				{"f", nil},
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

	Describe("Topic()", func() {
		It("returns a Topic based on the user's ID", func() {
			u := user.User{ID: 123}
			Expect(u.Topic()).To(Equal(pubsub.Topic("user-123")))
			Expect(u.Topic()).To(BeEquivalentTo("user-123"))
		})
	})

	Describe("CanCreateHosts()", func() {
		var u *user.User

		BeforeEach(func() {
			u = &user.User{
				Username: "qux",
				Name:     "quxboy",
				Password: "123456",
				Email:    "qux@example.com",
			}
			err := WrapInTransaction(db, u.Insert)
			Expect(err).To(BeNil())

			_, err = db.Exec(`UPDATE user_profiles SET max_hosts = 2 WHERE user_id = $1;`, u.ID)
			Expect(err).To(BeNil())
		})

		It("returns false, when owned hosts == max hosts", func() {
			hs, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())

			_, err = CreateHostStack(db, &HostStack{
				U:  u,
				R:  hs.R,
				P:  hs.P,
				PT: hs.PT,
			})
			Expect(err).To(BeNil())

			// max hosts for the user is set to 2
			// and user already has 2 hosts
			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeFalse())
		})

		It("returns true, when owned hosts < max hosts", func() {
			_, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())

			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeTrue())
		})

		It("returns true, when user has no hosts", func() {
			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeTrue())
		})

		It("ignores terminated hosts", func() {
			hs, err := CreateHostStack(db, &HostStack{U: u})
			Expect(err).To(BeNil())
			err = hs.H.Delete(db)
			Expect(err).To(BeNil())

			hs2, err := CreateHostStack(db, &HostStack{
				U:  u,
				R:  hs.R,
				P:  hs.P,
				PT: hs.PT,
			})
			Expect(err).To(BeNil())
			err = hs2.H.Delete(db)
			Expect(err).To(BeNil())

			b, err := u.CanCreateHosts(db)
			Expect(err).To(BeNil())
			Expect(b).To(BeTrue())
		})
	})
})
