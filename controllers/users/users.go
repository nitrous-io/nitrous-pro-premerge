package users

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	validations = map[string]error{
		"index_users_on_username": httperror.New(http.StatusBadRequest, "validation_failed", "username is taken"),
		"index_users_on_email":    httperror.New(http.StatusBadRequest, "validation_failed", "email is taken"),
	}
	SignUpCode string
)

func init() {
	SignUpCode = os.Getenv("SIGNUP_CODE")
	if SignUpCode == "" {
		SignUpCode = "emm@ w@tson luv"
	}
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	code := r.FormValue("code")
	if code != SignUpCode {
		var reason string
		if code == "" {
			reason = "code can't be blank"
		} else {
			reason = "code is incorrect"
		}
		return httperror.New(http.StatusBadRequest, "validation_failed", reason)
	}

	email := strings.ToLower(r.FormValue("email"))
	username := strings.ToLower(r.FormValue("username"))
	password := r.FormValue("password")
	name := r.FormValue("name")

	u := &user.User{
		Email:    email,
		Username: username,
		Name:     name,
		Password: password,
	}

	err := u.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = u.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	env.User = u

	env.Tracker.CreateProfile(u, env.Logger)
	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	email, password, name := r.PostFormValue("email"), r.PostFormValue("password"), r.PostFormValue("name")

	if email == "" && password == "" && name == "" {
		userResponse(env, u)
		return nil
	}

	if email != "" {
		u.Email = email
	}
	if password != "" {
		u.Password = password
	}
	if name != "" {
		u.Name = name
	}

	if err := u.Validate(); err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	if err := u.Update(db); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	userResponse(env, u)
	return nil
}

func GetAuthenticatedUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	if u == nil {
		return errors.New("user is nil")
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	return nil
}

func userResponse(env *middleware.Env, u *user.User) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	return
}
