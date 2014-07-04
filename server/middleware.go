package server

import (
	"crypto/sha1"
	"fmt"
	"net/http"
	"strings"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errInvalidRequest = httperror.New(http.StatusBadRequest, "invalid_request")
	errInvalidToken   = httperror.New(http.StatusUnauthorized, "invalid_token")
)

// This is called when the error is not a *httperror.Error*, i.e. there's
// something wrong with the controller. We transform that error into a
// httperror.Error and log it.
func (s *Server) serverError(err error) error {
	code := http.StatusInternalServerError
	errMsg := err.Error()
	errHash := fmt.Sprintf("%x", sha1.Sum([]byte(errMsg)))
	e := httperror.New(code, "internal_server_error", "", errHash)
	s.logger.Error(errHash, e.Error())
	return e
}

// process the request error. If the error is of type *httperror.Error, then we
// convert and serialize that to JSON, otherwise, the error will be
// `500 internal_server_error`
func (s *Server) handleError(w http.ResponseWriter, err error) {
	e, ok := err.(*httperror.Error)
	if !ok {
		e = s.serverError(e).(*httperror.Error)
	}
	j, err := e.JSON()
	if err != nil {
		j = []byte(`{"error":"internal_server_error"}`)
	}
	s.Respond(w, e.Code, j)
}

func (s *Server) action(fn func(controllers.Helper, http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(s, w, r)
		if err != nil {
			s.handleError(w, err)
		}
	}
}

func (s *Server) authedAction(fn func(controllers.Helper, *user.User, http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			s.handleError(w, errInvalidRequest)
			return
		}

		tok := authHeader[len(prefix):]
		t, err := token.FindByToken(s.db, tok)
		if err != nil {
			s.handleError(w, err)
			return
		}
		if t == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			s.handleError(w, errInvalidToken)
			return
		}

		u, err := user.FindByID(s.db, t.UserID)
		if err != nil {
			s.handleError(w, err)
			return
		}
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			s.handleError(w, errInvalidToken)
			return
		}

		err = fn(s, u, w, r)
		if err != nil {
			s.handleError(w, err)
		}
	}
}

type authJSONHandler func(controllers.Helper, *user.User, http.ResponseWriter, *http.Request) (*controllers.JSONResponse, error)
type publicJSONHandler func(controllers.Helper, http.ResponseWriter, *http.Request) (*controllers.JSONResponse, error)

// FIXME: fix duplication between this and the auth method above
func (s *Server) authJSON(f authJSONHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			s.handleError(w, errInvalidRequest)
			return
		}

		tok := authHeader[len(prefix):]
		t, err := token.FindByToken(s.db, tok)
		if err != nil {
			s.handleError(w, err)
			return
		}
		if t == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			s.handleError(w, errInvalidToken)
			return
		}

		u, err := user.FindByID(s.db, t.UserID)
		if err != nil {
			s.handleError(w, err)
			return
		}
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			s.handleError(w, errInvalidToken)
			return
		}

		resp, err := f(s, u, w, r)
		if err != nil {
			s.handleError(w, err)
			return
		}
		s.RespondEncodeJSON(w, resp.Code, resp.Data)
	}
}

func (s *Server) publicJSON(f publicJSONHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := f(s, w, r)
		if err != nil {
			s.handleError(w, err)
			return
		}
		s.RespondEncodeJSON(w, resp.Code, resp.Data)
	}
}
