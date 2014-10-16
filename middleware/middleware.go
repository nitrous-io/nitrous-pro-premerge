package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/analytics"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/pkg/mergemap"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
)

var (
	errInvalidRequest = httperror.New(http.StatusBadRequest, "invalid_request", "auth token is required")
	errInvalidToken   = httperror.New(http.StatusUnauthorized, "invalid_token", "auth token is invalid")
	ErrHostNotFound   = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
)

type Env struct {
	Root           string
	Logger         *logger.Logger
	DB             *db.Database
	Events         *pubsub.Pubsub
	Status         int
	User           *user.User
	Data           map[string]interface{}
	TrackEventName string
	trackEventData map[string]interface{}
	RemoteAddr     string
	Tracker        analytics.Tracker
}

func (env *Env) AddEventData(data map[string]interface{}) {
	if env.trackEventData == nil {
		env.trackEventData = map[string]interface{}{}
	}

	mergemap.Merge(data, env.trackEventData)
}

type Middleware func(env *Env, w http.ResponseWriter, r *http.Request) error

func authenticate(tok string, mw Middleware, env *Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

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
	return mw(env, w, r)
}

func Auth(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		authHeader := r.Header.Get("Authorization")
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			return errInvalidRequest
		}

		tok := authHeader[len(prefix):]
		return authenticate(tok, mw, env, w, r)
	}
}

func Track(event string, mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		env.TrackEventName = event
		if err := mw(env, w, r); err != nil {
			return err
		}

		if env.RemoteAddr != "" {
			env.AddEventData(map[string]interface{}{"$ip": env.RemoteAddr})
		}

		if env.TrackEventName != "" {
			env.Tracker.TrackEvent(event, env.User, env.trackEventData, env.Logger)
		}

		return nil
	}
}

func AuthQS(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") != "" {
			return Auth(mw)(env, w, r)
		}
		tok := r.FormValue("token")
		if tok == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			return errInvalidRequest
		}
		return authenticate(tok, mw, env, w, r)
	}
}

type hostHandlerFn func(h *host.Host, env *Env, w http.ResponseWriter, r *http.Request) error

func HostViaForm(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("form", srcVar, fn)
}

func HostViaPath(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("path", srcVar, fn)
}

// withHost takes a src ("form" or "path") and the formvalue/pattern to look
// for to retrieve host slug.
func withHost(src string, srcVar string, fn hostHandlerFn) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		var param string
		var h *host.Host

		switch src {
		case "form":
			param = r.FormValue(srcVar)
		case "path":
			param = mux.Vars(r)[srcVar]
		default:
			return errors.New("Invalid host source: " + src)
		}

		// param can either be a slug or an ID
		h, err := host.Find(env.DB, env.User, param)
		if err != nil {
			return err
		}
		if h == nil {
			return ErrHostNotFound
		}

		return fn(h, env, w, r)
	}
}
