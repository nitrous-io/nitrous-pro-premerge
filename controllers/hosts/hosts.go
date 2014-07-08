package hosts

import (
	"database/sql"

	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/hostbuilder"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host size not provided")
	ErrHostRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host not provided")
	ErrPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	ErrRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	ErrHostNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching host found for the given name")
)

func Index(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	hosts, err := host.FindAllByUser(db, u)
	if err != nil {
		return err
	}
	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
	return nil
}

func Create(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	env := c.Env()

	size := r.FormValue("size")
	if size == "" {
		return ErrSizeRequired
	}

	name := r.FormValue("name")
	if name == "" {
		name = "double-rainbow" //FIXME: implement a random name generator
	}

	regionName := r.FormValue("region")
	if regionName == "" {
		regionName = "apse1" //FIXME: user model should specify a default region?
	}

	longname := r.FormValue("longname")
	if longname == "" {
		longname = name
	}

	// TODO: check if user account is eligible to create hosts (has a credit card on file)

	pl, err := plan.FindByName(db, size)
	if err != nil {
		return err
	}
	if pl == nil {
		return ErrPlanNotFound
	}

	rg, err := region.FindByName(db, regionName)
	if err != nil {
		return err
	}
	if rg == nil {
		return ErrRegionNotFound
	}

	pts, err := providertemplate.FindAllByRegionAndPlan(db, pl.ID, rg.ID)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return ErrProvidertemplateNotFound
	}
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	pt := pts[0]

	config := pt.Config.Map
	if config == nil {
		config = map[string]sql.NullString{}
	}
	config["name"] = sql.NullString{name, true}
	config["public_keys"] = sql.NullString{"", true} //FIXME: pass actual public keys

	hb := hostbuilder.NewHostbuilder(env.HostbuilderURL, pt.Provider, config)
	info, err := hb.Create()
	if err != nil {
		return err
	}

	h := &host.Host{
		UserID:             u.ID,
		RegionID:           rg.ID,
		PlanID:             pl.ID,
		ProviderTemplateID: pt.ID,

		Name:          name,
		LongName:      sql.NullString{longname, true},
		MaxContainers: sql.NullInt64{pl.MaxContainers, true},

		IPAddress:          sql.NullString{info["ip_address"], true},
		ProviderIdentifier: sql.NullString{info["id"], true},
		State:              "created",
	}
	err = h.Insert(db)
	if err != nil {
		return httperror.New(http.StatusInternalServerError, "internal_server_error", err.Error())
	}

	c.RespondEncodeJSON(w, http.StatusCreated, map[string]interface{}{
		"host": h,
	})
	return nil
}

func Delete(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	env := c.Env()

	name := r.FormValue("name")
	if name == "" {
		return ErrHostRequired
	}

	ht, err := host.FindByUserAndName(db, u, name)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if ht == nil {
		return ErrHostNotFound
	}

	pt, err := providertemplate.FindByID(db, ht.ProviderTemplateID)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	config := map[string]sql.NullString{}
	config["id"] = ht.ProviderIdentifier

	hb := hostbuilder.NewHostbuilder(env.HostbuilderURL, pt.Provider, config)
	_, err = hb.Delete()
	if err != nil {
		return err
	}

	err = ht.Delete(db)
	if err != nil {
		return err
	}

	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{})
	return nil
}

func Start(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	env := c.Env()

	name := r.FormValue("name")
	if name == "" {
		return ErrHostRequired
	}

	ht, err := host.FindByUserAndName(db, u, name)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if ht == nil {
		return ErrHostNotFound
	}

	pt, err := providertemplate.FindByID(db, ht.ProviderTemplateID)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	config := map[string]sql.NullString{}
	config["id"] = ht.ProviderIdentifier

	hb := hostbuilder.NewHostbuilder(env.HostbuilderURL, pt.Provider, config)
	_, err = hb.Start()
	if err != nil {
		return err
	}

	ht.State = "running"
	err = ht.UpdateColumn(db, "state")
	if err != nil {
		return err
	}

	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{})
	return nil
}

func Stop(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	env := c.Env()

	name := r.FormValue("name")
	if name == "" {
		return ErrHostRequired
	}

	ht, err := host.FindByUserAndName(db, u, name)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if ht == nil {
		return ErrHostNotFound
	}

	pt, err := providertemplate.FindByID(db, ht.ProviderTemplateID)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	config := map[string]sql.NullString{}
	config["id"] = ht.ProviderIdentifier

	hb := hostbuilder.NewHostbuilder(env.HostbuilderURL, pt.Provider, config)
	_, err = hb.Stop()
	if err != nil {
		return err
	}

	ht.State = "stopped"
	err = ht.UpdateColumn(db, "state")
	if err != nil {
		return err
	}

	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{})
	return nil
}

func Restart(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	env := c.Env()

	name := r.FormValue("name")
	if name == "" {
		return ErrHostRequired
	}

	ht, err := host.FindByUserAndName(db, u, name)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if ht == nil {
		return ErrHostNotFound
	}

	pt, err := providertemplate.FindByID(db, ht.ProviderTemplateID)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	config := map[string]sql.NullString{}
	config["id"] = ht.ProviderIdentifier

	hb := hostbuilder.NewHostbuilder(env.HostbuilderURL, pt.Provider, config)
	_, err = hb.Restart()
	if err != nil {
		return err
	}

	ht.State = "running"
	err = ht.UpdateColumn(db, "state")
	if err != nil {
		return err
	}

	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{})
	return nil
}
