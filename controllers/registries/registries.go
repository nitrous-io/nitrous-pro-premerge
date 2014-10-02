package registries

import (
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/registry"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	errUsernameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "username is required")
	errPasswordRequired = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errEmailRequired    = httperror.New(http.StatusBadRequest, "invalid_request", "email is required")
	errRegistryRequired = httperror.New(http.StatusBadRequest, "invalid_request", "registry is required")
	errLoginFailed      = httperror.New(http.StatusBadRequest, "login_failed", "registry login failed")
	errLogoutFailed     = httperror.New(http.StatusBadRequest, "logout_failed", "registry logout failed")
	errAlreadyLoggedIn  = httperror.New(http.StatusBadRequest, "login_failed", "already logged in to this registry")
	errRegistryNotFound = httperror.New(http.StatusNotFound, "not_found", "registry does not exist")
)

var (
	Index  middleware.Middleware = middleware.HostViaPath("slug", index)
	Login  middleware.Middleware = middleware.HostViaPath("slug", login)
	Logout middleware.Middleware = middleware.HostViaPath("slug", logout)
)

func index(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	registries, err := registry.FindAll(env.DB, env.User.ID, h.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"registries": serializers.SerializeRegistries(registries),
	}
	return nil
}

func login(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	username := r.FormValue("username")
	if username == "" {
		return errUsernameRequired
	}

	password := r.FormValue("password")
	if password == "" {
		return errPasswordRequired
	}

	email := r.FormValue("email")
	if email == "" {
		return errEmailRequired
	}

	registryURL := r.FormValue("url")
	if registryURL == "" {
		return errRegistryRequired
	}

	existingReg, err := registry.Find(db, env.User.ID, h.ID, registryURL)
	if err != nil {
		return err
	}

	if existingReg != nil {
		return errAlreadyLoggedIn
	}

	params := &freighterclient.RegistryAuth{
		Username: username,
		Password: password,
		Email:    email,
		Registry: registryURL,
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	err = cl.RegistryLogin(params)
	if err != nil {
		if err == freighterclient.ErrInvalidParams {
			return errLoginFailed
		}

		return err
	}

	reg := &registry.Registry{
		UserID:   env.User.ID,
		HostID:   h.ID,
		URL:      registryURL,
		Username: username,
		Email:    email,
	}

	err = reg.Insert(db)
	if err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventRegistryAdded, map[string]*serializers.Registry{
			"registry": serializers.SerializeRegistry(reg),
		}),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"registry": serializers.SerializeRegistry(reg),
	}
	return nil
}

func logout(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	// rSlug can either be a URL or an ID
	rSlug := mux.Vars(r)["rslug"]
	rSlug, err = url.QueryUnescape(rSlug)
	if err != nil {
		return err
	}

	reg, err := registry.Find(db, env.User.ID, h.ID, rSlug)
	if err != nil {
		return err
	}
	if reg == nil {
		return errRegistryNotFound
	}

	err = cl.RegistryLogout(reg.URL)
	if err != nil {
		if err == freighterclient.ErrInvalidParams {
			return errLogoutFailed
		}
		return err
	}

	err = registry.Delete(db, env.User.ID, reg.ID)
	if err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventRegistryRemoved, map[string]*serializers.Registry{
			"registry": serializers.SerializeRegistry(reg),
		}),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}
