package middlewares

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errInvalidRequest = httperror.New(http.StatusBadRequest, "invalid_request", "auth token is required")
	errInvalidToken   = httperror.New(http.StatusUnauthorized, "invalid_token", "auth token is invalid")
	ErrHostNotFound   = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
)

type Env struct {
	Status int
	User   *user.User
	Data   map[string]interface{}
	Vars   map[string]string
}

type Middleware func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error

func Auth(mw Middleware) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		authHeader := r.Header.Get("Authorization")
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			return errInvalidRequest
		}

		tok := authHeader[len(prefix):]
		t, err := token.FindByToken(db, tok)
		if err != nil {
			return err
		}
		if t == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			return errInvalidToken
		}

		u, err := user.FindByID(db, t.UserID)
		if err != nil {
			return err
		}
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			return errInvalidToken
		}

		env.User = u
		return mw(env, c, w, r)
	}
}

func Vars(mw Middleware) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		env.Vars = mux.Vars(r)
		return mw(env, c, w, r)
	}
}

func JSON(mw Middleware) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		err := mw(env, c, w, r)
		if err != nil {
			return err
		}
		if env.Status == 0 || env.Data == nil {
			return errors.New("Status and/or Data not set")
		}
		c.RespondEncodeJSON(w, env.Status, env.Data)
		return nil
	}
}

type hostHandlerFn func(h *host.Host, env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error

func HostViaForm(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("form", srcVar, fn)
}

func HostViaPath(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("path", srcVar, fn)
}

// withHost takes a src ("form" or "path") and the formvalue/pattern to look
// for to retrieve host slug.
func withHost(src string, srcVar string, fn hostHandlerFn) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		var hSlug string
		switch src {
		case "form":
			hSlug = r.FormValue(srcVar)
		case "path":
			hSlug = env.Vars[srcVar]
		default:
			return errors.New("Invalid host source: " + src)
		}

		h, err := host.FindByUserAndSlug(c.DB(), env.User, hSlug)
		if err != nil {
			return err
		}
		if h == nil {
			return ErrHostNotFound
		}

		return fn(h, env, c, w, r)
	}
}
