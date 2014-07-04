package user

import (
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
)

var (
	usernameRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
	emailRe    = regexp.MustCompile(`\A[^@\s]+@([^@\s]+\.)+[^@\s]+\z`)
)

var (
	ErrEmailBlank       = errors.New("email can't be blank")
	ErrEmailInvalid     = errors.New("email is not valid")
	ErrUsernameBlank    = errors.New("username can't be blank")
	ErrUsernameTooShort = errors.New("username is too short (minimum is 3 characters)")
	ErrUsernameTooLong  = errors.New("username is too long (maximum is 30 characters)")
	ErrUsernameInvalid  = errors.New("username may only contain alphanumeric characters or dashes and cannot begin with a dash")
	ErrPasswordBlank    = errors.New("password can't be blank")
	ErrPasswordTooShort = errors.New("password is too short (minimum is 6 characters)")
	ErrPasswordTooLong  = errors.New("password is too long (maximum is 30 characters)")
)

type User struct {
	ID                int64  `db:"id" json:"-"`
	Name              string `db:"name" json:"-"`
	EncryptedPassword string `db:"encrypted_password" json:"-"`
	Password          string `json:"-"`

	// Exposed attributes to API
	Username  string     `db:"username" json:"username"`
	Email     string     `db:"email" json:"email"`
	CreatedAt *time.Time `db:"created_at" json:"created_at"`
	UpdatedAt *time.Time `db:"updated_at" json:"updated_at"`
}

func FindByID(d db.QueryReceiver, id int64) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func FindByUsername(d db.QueryReceiver, username string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE username = $1", username)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (u *User) Insert(d db.QueryReceiver) error {
	if u.Password != "" {
		encrypted, err := bcrypt.Hash(u.Password)
		if err != nil {
			return err
		}
		u.EncryptedPassword = encrypted
		u.Password = ""
	}
	_, err := d.NamedExec("INSERT INTO users (email, username, encrypted_password, name) VALUES (:email, :username, :encrypted_password, :name);", u)
	if err != nil {
		return err
	}
	return d.Get(u, "SELECT * FROM users WHERE username = $1;", u.Username)
}

func (u *User) Validate() error {
	if u.Email == "" {
		return ErrEmailBlank
	}
	if !emailRe.MatchString(u.Email) {
		return ErrEmailInvalid
	}

	if u.Username == "" {
		return ErrUsernameBlank
	}
	usernameLen := len(u.Username)
	if usernameLen < 2 {
		return ErrUsernameTooShort
	}
	if usernameLen > 30 {
		return ErrUsernameTooLong
	}
	if !usernameRe.MatchString(u.Username) {
		return ErrUsernameInvalid
	}

	if u.Password == "" && u.EncryptedPassword == "" {
		return ErrPasswordBlank
	}
	if u.Password != "" {
		passwordLen := len(u.Password)
		if passwordLen < 6 {
			return ErrPasswordTooShort
		}
		if passwordLen > 30 {
			return ErrPasswordTooLong
		}
	}

	return nil
}
