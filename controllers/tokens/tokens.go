package tokens

import (
	"net/http"

	"github.com/nitrous-io/nitrous/models/token"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errGrantType   = httperror.New(http.StatusBadRequest, "unsupported_grant_type")
	errUsername    = httperror.New(http.StatusBadRequest, "invalid_request", "username is required")
	errPassword    = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errInvalidCred = httperror.New(http.StatusBadRequest, "invalid_grant", "incorrect username or password")
)

func Post(c controllers.Helper, w http.ResponseWriter, r *http.Request) (*controllers.JSONResponse, error) {
	db := c.DB()

	grantType := r.FormValue("grant_type")
	username := r.FormValue("username")
	password := r.FormValue("password")

	if grantType != "password" {
		return nil, errGrantType
	}

	if username == "" {
		return nil, errUsername
	}

	if password == "" {
		return nil, errPassword
	}

	u, err := user.FindByUsername(db, username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, errInvalidCred
	}

	if !bcrypt.Verify(u.EncryptedPassword, password) {
		return nil, errInvalidCred
	}

	t := &token.Token{UserID: u.ID}

	err = t.Insert(db)
	if err != nil {
		return nil, err
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	return &controllers.JSONResponse{
		Code: http.StatusOK,
		Data: map[string]interface{}{
			"access_token": t.Token,
			"token_type":   "Bearer",
		},
	}, nil
}
