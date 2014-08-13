package containers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/cvolume"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

// DefaultImage is the default Docker image used to create containers if an
// image isn't specified.
const DefaultImage = "nitrousio/ubuntu:trusty"

var (
	errSlugRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errHostNotFound      = middleware.ErrHostNotFound
	errVolumeNotFound    = httperror.New(http.StatusBadRequest, "invalid_state", "volume not found")
	errContainerNotFound = httperror.New(http.StatusNotFound, "not_found", "container does not exist")
	errInvalidPortspec   = httperror.New(http.StatusBadRequest, "invalid_request", "port spec invalid")
)

var (
	validations = map[string]error{
		"index_containers_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var (
	Create  middleware.Middleware = middleware.HostViaForm("host", requiresRunningHost(create))
	Show    middleware.Middleware = WithContainerAndHost(show)
	Start   middleware.Middleware = WithContainerAndHost(start)
	Stop    middleware.Middleware = WithContainerAndHost(stop)
	Restart middleware.Middleware = WithContainerAndHost(restart)
	Destroy middleware.Middleware = WithContainerAndHost(destroy)
)

func Index(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	var err error
	var hosts []host.Host

	if slug := r.FormValue("host"); slug != "" {
		h, err := host.FindByUserAndSlug(db, env.User, slug)
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}

		hosts = []host.Host{*h}
	} else {
		// If no host is specified, return all containers from all hosts.
		hosts, err = host.FindAllByUser(db, env.User)
		if err != nil {
			return err
		}
	}

	containersResp := []*serializers.Container{}
	for _, h := range hosts {
		savedContainers, err := container.FindAllByHostID(db, h.ID)
		if err != nil {
			return err
		}

		liveContainers, err := freighterList(db, &h)
		if err != nil {
			// If we can't get through to the host, return only container data
			// from the database.
			if err == controllers.ErrHostNotRunning || err == controllers.ErrHostIPUnknown {
				for _, sc := range savedContainers {
					containersResp = append(containersResp, serializers.SerializeContainer(sc))
				}
				continue
			}
			return err
		}

		for _, lc := range liveContainers {
			var savedContainer *container.Container
			for _, sc := range savedContainers {
				if sc.CID == lc.ID {
					savedContainer = sc
					break
				}
			}
			var cn *container.Container
			if savedContainer != nil {
				cn = hydrateContainer(savedContainer, lc, &h)
			} else {
				cn = mkContainer(lc, &h)
			}

			containersResp = append(containersResp, serializers.SerializeContainer(cn))
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"containers": containersResp,
	}
	return nil
}

type listresult struct {
	containers []*freighterclient.Container
	err        error
}

func freighterList(db db.QueryReceiver, h *host.Host) ([]*freighterclient.Container, error) {
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return nil, err
	}
	return cl.List()
}

func create(h runningHost, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	log := c.Logger()

	slug := r.FormValue("slug")
	name := r.FormValue("name")
	image := r.FormValue("image")

	if image == "" {
		image = DefaultImage
	}

	if slug == "" {
		return errSlugRequired
	}

	if name == "" {
		name = slug
	}

	hv, err := hostVolumes(db, h)
	if err != nil {
		return err
	}

	config, err := prepareConfig(r.PostForm, hv)
	if err != nil {
		return err
	}

	cn := &container.Container{
		UserID: env.User.ID,
		HostID: h.ID,
		Slug:   slug,
		Image:  image,
		Name:   sql.NullString{name, true},
	}

	// Save record to db first to reserve name (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	err = cn.Insert(tx)
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

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	created, err := cl.Create(slug, image, config)
	if err != nil {
		delErr := cn.DeletePermanently(db)
		if delErr != nil {
			c.Logger().Errorf("Unable to delete container %d that failed on freighter: %v", cn.ID, delErr)
		}
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	// Update existing record with new info from Freighter.
	cn.CID = created.ID
	cn.Image = created.Image
	cn.State = created.State
	cn.IPAddress = created.Host
	err = cn.Update(db)
	if err != nil {
		log.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to update database with '%+v'", created, cn)
		// Continue responding as if nothing happened.
	}

	for _, v := range volumeSlugs(config.Mounts) {
		cv := cvolume.ContainerVolume{ContainerID: cn.ID, VolumeID: hv[v].ID}
		if err := cv.Insert(db); err != nil { // log and continue
			log.Errorf("containers.create: failed to attach volume %s to container %s", hv[v].Slug, cn.CID)
		}
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status":    "created",
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

func show(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(c.DB(), h)
	if err != nil {
		return err
	}

	// TODO Run this in a goroutine so it runs concurrently with DB query.
	liveContainer, err := cl.Info(cn.CID)
	if err != nil {
		return err
	}
	cn = hydrateContainer(cn, liveContainer, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}

	return nil
}

func start(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(c.DB(), h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Start, cn, h, env, c, w, r)
}

func stop(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(c.DB(), h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Stop, cn, h, env, c, w, r)
}

func restart(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(c.DB(), h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Restart, cn, h, env, c, w, r)
}

type actionFn func(id string) (*freighterclient.Container, error)

func performControlAction(fn actionFn, cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	liveContainer, err := fn(cn.CID)
	if err != nil {
		return err
	}

	cn = hydrateContainer(cn, liveContainer, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}

	return nil
}

func destroy(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	cl, err := controllers.NewFreighterClient(c.DB(), h)
	if err != nil {
		return err
	}

	liveContainer, err := cl.Terminate(cn.CID)
	if err != nil {
		return err
	}

	err = cn.Delete(db)
	if err != nil {
		return err
	}
	cn = hydrateContainer(cn, liveContainer, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

func WithContainerAndHost(fn func(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middleware.Middleware {
	return middleware.Vars(func(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		cSlug := env.Vars["slug"]
		cn, err := container.FindByUserAndSlug(db, env.User, cSlug)
		if err != nil {
			return err
		}
		if cn == nil {
			return errContainerNotFound
		}

		h, err := host.FindByID(db, cn.HostID)
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}
		if h.State != host.StateRunning {
			return controllers.ErrHostNotRunning
		}
		return fn(cn, h, env, c, w, r)
	})
}

type runningHost *host.Host
type hostFn func(h runningHost, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error

// requiresRunningHost is a wrapper which guarantees the action is called for a running host
func requiresRunningHost(fn hostFn) func(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return func(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		if h.State != host.StateRunning {
			return controllers.ErrHostNotRunning
		}
		return fn(h, env, c, w, r)
	}
}

func hydrateContainer(c *container.Container, fc *freighterclient.Container, h *host.Host) *container.Container {
	c.CID = fc.ID
	c.Image = fc.Image
	c.Slug = dropNamespace(fc.Name)
	c.State = fc.State
	c.IPAddress = fc.Host

	return c
}

func mkContainer(fc *freighterclient.Container, h *host.Host) *container.Container {
	return &container.Container{
		CID:       fc.ID,
		Image:     fc.Image,
		Slug:      dropNamespace(fc.Name),
		State:     fc.State,
		IPAddress: fc.Host,
		UserID:    h.UserID,
		HostID:    h.ID,
	}
}

func dropNamespace(freighterName string) string {
	parts := strings.SplitN(freighterName, ".", 2)
	switch len(parts) {
	case 1:
		return freighterName
	case 2:
		// more than one period is unexpected (disallowed in container slug)
		// not this func's concern, so pass-through after stripping portion
		// before first period
		return parts[1]
	default:
		return freighterName // never reached, satisfy compiler
	}
}

func prepareConfig(form url.Values, hv map[string]*volume.Volume) (*freighterclient.ContainerConfig, error) {
	c := &freighterclient.ContainerConfig{}
	if envs, ok := form["env[]"]; ok {
		c.Env = envs
	}
	if vols, ok := form["volume[]"]; ok {
		for _, v := range volumeSlugs(vols) {
			if _, ok := hv[v]; !ok {
				return nil, errVolumeNotFound
			}
		}
		c.Mounts = vols
	}
	if ports, ok := form["port[]"]; ok {
		c.PortBindings = ports
	}
	return c, nil
}

func volumeSlugs(volParams []string) []string {
	slugs := []string{}
	for _, v := range volParams {
		slugs = append(slugs, strings.Split(v, ":")[0])
	}
	return slugs
}

func hostVolumes(d db.QueryReceiver, h *host.Host) (map[string]*volume.Volume, error) {
	slugsToVols := map[string]*volume.Volume{}
	hvols, err := volume.FindAllByHostID(d, h.ID)
	if err != nil {
		return nil, err
	}
	for _, hv := range hvols {
		slugsToVols[hv.Slug] = hv
	}
	return slugsToVols, nil
}
