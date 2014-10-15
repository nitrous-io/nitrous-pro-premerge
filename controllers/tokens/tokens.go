package tokens

import (
	"net/http"
	"time"

	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errGrantType   = httperror.New(http.StatusBadRequest, "unsupported_grant_type")
	errLogin       = httperror.New(http.StatusBadRequest, "invalid_request", "username or email is required")
	errPassword    = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errInvalidCred = httperror.New(http.StatusBadRequest, "invalid_grant", "incorrect username or password")
)

var (
	LoginAttemptsWaitTime = map[int]time.Duration{
		3: time.Duration(0),
		6: time.Second * 1,
		9: time.Second * 3,
	}
	LoginAttemptsFinalWaitTime = time.Second * 5
)

func Post(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	grantType := r.FormValue("grant_type")
	login := r.FormValue("username") // this can be either username or email
	password := r.FormValue("password")

	if grantType != "password" {
		return errGrantType
	}

	if login == "" {
		return errLogin
	}

	attemptCount, err := user.GetLoginAttempts(db, login)
	if err != nil {
		return err
	}
	sleepOnLoginAttempts(attemptCount)

	if password == "" {
		return errPassword
	}

	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		return err
	}
	if u == nil {
		return errInvalidCred
	}

	if !bcrypt.Verify(u.EncryptedPassword, password) {
		if err := user.IncrementLoginAttempts(db, login); err != nil {
			return err
		}
		return errInvalidCred
	}

	if err := user.ResetLoginAttempts(db, login); err != nil {
		return err
	}

	t := &token.Token{UserID: u.ID}

	err = t.Insert(db)
	if err != nil {
		return err
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	env.Status = http.StatusOK
	env.User = u
	env.Data = map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "Bearer",
	}
	env.Tracker.CreateProfile(u, env.Logger)
	return nil
}

func sleepOnLoginAttempts(loginAttempts int) {
	if loginAttempts == 0 {
		return
	}

	sleepTime := LoginAttemptsFinalWaitTime
	for attempt, waitTime := range LoginAttemptsWaitTime {
		if loginAttempts < attempt {
			if waitTime < sleepTime {
				sleepTime = waitTime
			}
		}
	}
	time.Sleep(sleepTime)
	return
}
