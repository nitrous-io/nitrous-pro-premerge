package volumes

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/lib/pq"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	errSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errNotFound                 = httperror.New(http.StatusNotFound, "volume_not_found", "volume could not be found")
	errCannotDeleteSystemVolume = httperror.New(http.StatusForbidden, "forbidden", "system volume cannot be deleted")
)

var (
	validations = map[string]error{
		"index_volumes_on_host_id_and_slug": httperror.New(http.StatusBadRequest, "validation_failed", "volume already exists on this host"),
	}
)

func Index(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	vols, err := volume.FindAll(env.DB, h.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"volumes": serializers.SerializeVolumes(vols),
	}
	return nil
}

func Create(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	slug := strings.ToLower(r.FormValue("slug"))
	if slug == "" {
		return errSlugRequired
	}
	name := r.FormValue("name")

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	vol := &volume.Volume{
		HostID: h.ID,
		Slug:   slug,
		Name:   name,
	}

	if err := vol.Insert(db); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	if _, err := cl.CreateVolume(slug); err != nil {
		if e := vol.Delete(db); e != nil {
			env.Logger.Errorf("Unable to delete volume %d that failed on freighter: %v", vol.ID, e)
		}
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	publishVolumeEvent(env, controllers.EventVolumeCreated, vol)
	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	slug := mux.Vars(r)["vslug"]

	vol, err := getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	if vol.System {
		return errCannotDeleteSystemVolume
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	if err := cl.RemoveVolume(slug); err != nil {
		return err
	}

	if err := vol.Delete(db); err != nil {
		env.Logger.Errorf("Unable to delete volume %d that was destroyed on freighter: %v", vol.ID, err)
		return err
	}

	publishVolumeEvent(env, controllers.EventVolumeDestroyed, vol)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Show(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	slug := mux.Vars(r)["vslug"]

	vol, err := getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"volume": serializers.SerializeVolume(vol),
	}
	return nil
}

func Info(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["vslug"]

	_, err = getVolume(db, h.ID, slug)
	if err != nil {
		return err
	}

	vi, err := cl.GetVolumeInfo(slug)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"info": serializers.SerializeVolumeInfo(vi),
	}
	return nil
}

func getVolume(db ndb.QueryReceiver, hostID int64, slug string) (*volume.Volume, error) {
	if slug == "" {
		return nil, errSlugRequired
	}
	vol, err := volume.Find(db, hostID, slug)
	if err != nil {
		return nil, err
	}
	if vol == nil {
		return nil, errNotFound
	}
	return vol, nil
}

func publishVolumeEvent(env *middleware.Env, name controllers.EventName, v *volume.Volume) {
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Volume{
			"volume": serializers.SerializeVolume(v),
		}),
		env.User.Topic(),
	)
}
