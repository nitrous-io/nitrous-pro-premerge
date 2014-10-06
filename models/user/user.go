package user

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/userprofile"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
)

var (
	usernameRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
	emailRe    = regexp.MustCompile(`\A[^@\s]+@([^@\s]+\.)+[^@\s]+\z`)
)

var (
	ErrEmailBlank       = errors.New("email can't be blank")
	ErrEmailInvalid     = errors.New("email is not valid")
	ErrUsernameBlank    = errors.New("username can't be blank")
	ErrUsernameTooLong  = errors.New("username is too long (maximum is 30 characters)")
	ErrUsernameInvalid  = errors.New("username may only contain alphanumeric characters or dashes and cannot begin with a dash")
	ErrPasswordBlank    = errors.New("password can't be blank")
	ErrPasswordTooShort = errors.New("password is too short (minimum is 6 characters)")
	ErrPasswordTooLong  = errors.New("password is too long (maximum is 30 characters)")
)

type User struct {
	ID        int64      `db:"id" json:"id"`
	Name      string     `db:"name" json:"name"`
	Username  string     `db:"username" json:"username"`
	Email     string     `db:"email" json:"email"`
	CreatedAt *time.Time `db:"created_at" json:"created_at"`
	UpdatedAt *time.Time `db:"updated_at" json:"updated_at"`

	Password          string `json:"-"`
	EncryptedPassword string `db:"encrypted_password" json:"-"`

	LoginAttempts int `db:"login_attempts" json:"-"`
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

func FindByUsernameOrEmail(d db.QueryReceiver, login string) (*User, error) {
	user := &User{}
	err := d.Get(user, "SELECT * FROM users WHERE username = $1 OR email = $1", login)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func GetLoginAttempts(d db.QueryReceiver, login string) (int, error) {
	var loginAttempts int
	err := d.QueryRowx("SELECT login_attempts FROM users WHERE username = $1 OR email = $1", login).Scan(&loginAttempts)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return loginAttempts, nil
}

func IncrementLoginAttempts(d db.QueryReceiver, login string) error {
	_, err := d.Exec("UPDATE users SET login_attempts = least(login_attempts + 1, 32767) WHERE username = $1 OR email = $1", login)
	return err
}

func ResetLoginAttempts(d db.QueryReceiver, login string) error {
	_, err := d.Exec("UPDATE users SET login_attempts = 0 WHERE username = $1 OR email = $1", login)
	return err
}

func (u *User) Insert(tx *sqlx.Tx) error {
	if err := u.encryptPassword(); err != nil {
		return err
	}
	nstmt, err := tx.PrepareNamed(`INSERT INTO users
		(email, username, encrypted_password, name)
		VALUES (:email, :username, :encrypted_password, :name)
		RETURNING *;`)
	if err != nil {
		return err
	}

	if err = nstmt.QueryRow(u).StructScan(u); err != nil {
		return err
	}

	// create a user profile
	return userprofile.Create(tx, u.ID)
}

func (u *User) Update(d db.QueryReceiver) error {
	if err := u.encryptPassword(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`UPDATE users
		SET (name, email, encrypted_password, updated_at)
		= (:name, :email, :encrypted_password, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(u).StructScan(u)
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

func (u *User) Topic() pubsub.Topic {
	return pubsub.Topic(fmt.Sprintf("user-%d", u.ID))
}

func (u *User) CanCreateHosts(d db.QueryReceiver) (bool, error) {
	var b bool
	err := d.QueryRowx(`SELECT (count(h.id) < coalesce(min(up.max_hosts), 0))
		FROM users u
		JOIN user_profiles up ON (up.user_id = u.id)
		LEFT JOIN hosts h ON (h.user_id = u.id AND h.terminated_at IS NULL)
		WHERE u.id = $1;`,
		u.ID).Scan(&b)

	if err != nil {
		return false, err
	}

	return b, nil
}

func (u *User) encryptPassword() error {
	if u.Password == "" {
		return nil
	}
	encrypted, err := bcrypt.Hash(u.Password)
	if err != nil {
		return err
	}
	u.EncryptedPassword = encrypted
	u.Password = ""
	return nil
}
