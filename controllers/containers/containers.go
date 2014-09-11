package containers

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/hmactoken"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/cvolume"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

// DefaultImage is the default Docker image used to create containers if an
// image isn't specified.
const DefaultImage = "nitrousio/ubuntu:trusty"

var (
	errSlugRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errImageNameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "image name is required")
	errHostNotFound      = middleware.ErrHostNotFound
	errVolumeNotFound    = httperror.New(http.StatusBadRequest, "invalid_state", "volume not found")
	errContainerNotFound = httperror.New(http.StatusNotFound, "not_found", "container does not exist")
	errInvalidPortspec   = httperror.New(http.StatusBadRequest, "invalid_request", "port spec invalid")
	errContainerLocked   = httperror.New(http.StatusConflict, "container_locked", "container is locked")
)

var (
	validations = map[string]error{
		"index_containers_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var (
	Create  = middleware.HostViaForm("host", requiresRunningHost(create))
	Commit  = WithContainerAndHost(commit)
	Show    = WithContainerAndHost(show)
	Start   = WithContainerAndHost(start)
	Stop    = WithContainerAndHost(stop)
	Restart = WithContainerAndHost(restart)
	Destroy = WithContainerAndHost(destroy)
	Token   = WithContainerAndHost(token)
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	var err error

	hSlug := r.FormValue("host")
	hosts := []host.Host{}
	containersResp := []*serializers.Container{}

	// If no host is provided, then get all hosts belonging to the user
	// and get all containers from all hosts
	if hSlug != "" {
		h, err := host.Find(db, env.User, hSlug)
		if err != nil {
			return err
		}

		if h != nil {
			hosts = []host.Host{*h}
		} else {
			return errHostNotFound
		}
	} else {
		hosts, err = host.FindAllByUser(db, env.User)
		if err != nil {
			return err
		}
	}

	for _, h := range hosts {
		liveContainers, freighterErr := freighterList(db, &h)

		savedContainers, err := container.FindAllByHostID(db, h.ID)
		if err != nil {
			return err
		}

		if freighterErr != nil {
			// If we can't get through to the host, then pass them along without the Freighter specific
			// info to the frontend
			if freighterErr == controllers.ErrHostNotRunning || freighterErr == controllers.ErrHostIPUnknown {
				for _, sc := range savedContainers {
					sc.State = container.StateHostStopped
					containersResp = append(containersResp, serializers.SerializeContainer(sc))
				}
			} else {
				env.Logger.Errorf("freighter err is not nil: %v, for host %s", freighterErr, h.Slug)
				return freighterErr
			}
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
	liveContainers, err := cl.List()
	if err != nil {
		return nil, err
	}
	return liveContainers, nil
}

func create(h runningHost, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	log := env.Logger

	slug := strings.ToLower(r.FormValue("slug"))
	name := r.FormValue("name")
	imageSlug := r.FormValue("image")

	if imageSlug == "" {
		imageSlug = DefaultImage
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
		Image:  imageSlug,
		Name:   sql.NullString{name, true},
	}

	// Save record to db first to reserve name (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	if err := cn.Insert(tx); err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	deleteContainer := func(cn *container.Container) {
		err := cn.DeletePermanently(db)
		if err != nil {
			env.Logger.Errorf("Unable to delete container %d that failed on freighter: %v", cn.ID, err)
		}
	}

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		deleteContainer(cn)
		return err
	}

	created, err := cl.Create(slug, imageSlug, cn.Secret, config)
	if err != nil {
		deleteContainer(cn)
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	// Update existing record with new info from Freighter.
	cn.CID = created.ID
	cn.Image = created.Image
	cn.IPAddress = created.Host
	cn.GatewayIP = created.Gateway
	cn.Cmd = created.Cmd
	cn.Entrypoint = created.Entrypoint
	cn.ExitCode = created.ExitCode
	cn.StartedAt = timePtr(created.StartedAt)
	cn.FinishedAt = timePtr(created.FinishedAt)

	if ok := cn.SetState(created.State); !ok { // log and continue
		log.Errorf("containers.create: Freighter returned container with unexpected state: %v", created.State)
	}

	err = cn.Update(db)
	if err != nil { // log and continue
		log.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to update database with '%+v'", created, cn)
	}

	// If image doesn't already exist, create a record.
	if _, err := image.Find(db, env.User, h, imageSlug); err == sql.ErrNoRows {
		repo, tag := image.ParseRepoAndTag(imageSlug)
		if tag == "" {
			tag = image.DefaultTag
		}
		img := &image.Image{
			UserID:      env.User.ID,
			HostID:      h.ID,
			FreighterID: created.ImageID,
			Slug:        repo,
			Tag:         tag,
		}

		if err := img.Insert(db); err != nil { // log and continue
			env.Logger.Errorf("containers.create: failed to insert image '%+v'", img)
		} else {
			env.Events.Publish(
				controllers.NewEvent("image.created", serializers.SerializeImage(img)),
				env.User.Topic(),
			)
		}
	}

	for _, v := range volumeSlugs(config.Mounts) {
		cv := cvolume.ContainerVolume{ContainerID: cn.ID, VolumeID: hv[v].ID}
		if err := cv.Insert(db); err != nil { // log and continue
			log.Errorf("containers.create: failed to attach volume %s to container %s", hv[v].Slug, cn.CID)
		}
	}

	publishContainerEvent(env, "container.created", cn)

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

func commit(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	name := r.FormValue("name")
	if name == "" {
		return errImageNameRequired
	}

	created, err := cl.Commit(cn.CID, name)
	if err == freighterclient.ErrImageExists {
		env.Status = http.StatusBadRequest
		env.Data = map[string]interface{}{
			"error":             "invalid_request",
			"error_description": "image already exists",
		}
		return nil
	}
	if err != nil {
		return err
	}

	var builtAt *time.Time
	if created.CreatedAt != 0 {
		t := time.Unix(created.CreatedAt, 0).UTC()
		builtAt = &t
	}

	img := &image.Image{
		UserID:      env.User.ID,
		HostID:      h.ID,
		FreighterID: created.ID,
		Slug:        created.Name,
		Tag:         created.Tag,
		ActualSize:  created.ActualSize,
		VirtualSize: created.VirtualSize,
		BuiltAt:     builtAt,
	}
	if err := img.InsertOrUpdate(env.DB); err != nil { // log and continue
		env.Logger.Errorf("containers.create: failed to insert or update image '%+v'", img)
	}

	sImg := serializers.SerializeImage(img)

	env.Events.Publish(controllers.NewEvent("image.created", sImg), env.User.Topic())
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": sImg,
	}
	return nil
}

func show(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		if err == controllers.ErrHostNotRunning || err == controllers.ErrHostIPUnknown {
			cn.State = container.StateHostStopped

			env.Status = http.StatusOK
			env.Data = map[string]interface{}{
				"container": serializers.SerializeContainer(cn),
			}
			return nil
		} else {
			return err
		}
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

func start(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Start, "container.started", cn, h, env, w, r)
}

func stop(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Stop, "container.stopped", cn, h, env, w, r)
}

func restart(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Restart, "container.restarted", cn, h, env, w, r)
}

type actionFn func(id string) (*freighterclient.Container, error)

func performControlAction(fn actionFn, eventName string, cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// automatically expire all control action locks after 1 minute.
	// TODO: test with real ops and tweak this value
	expiresIn := 60 * time.Second
	err := cn.Lock(expiresIn, env.DB)
	if err != nil {
		if err == container.ErrLocked {
			return errContainerLocked
		}
		return err
	}
	defer cn.Unlock(env.DB)

	liveContainer, err := fn(cn.CID)
	if err != nil {
		return err
	}

	cn = hydrateContainer(cn, liveContainer, h)

	publishContainerEvent(env, eventName, cn)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}

	return nil
}

func destroy(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	// automatically expire destroy action lock after 1 minute.
	// TODO: test with real ops and tweak this value
	expiresIn := 60 * time.Second
	err := cn.Lock(expiresIn, db)
	if err != nil {
		if err == container.ErrLocked {
			return errContainerLocked
		}
		return err
	}
	defer cn.Unlock(env.DB)

	cl, err := controllers.NewFreighterClient(db, h)
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

	publishContainerEvent(env, "container.destroyed", cn)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

func token(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	r.ParseForm()
	permissions := r.PostForm["permission[]"]
	t, err := hmactoken.Generate(permissions, cn.Secret)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"token": t,
	}
	return nil
}

func WithContainerAndHost(fn func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		db := env.DB

		cn, err := container.Find(db, env.User, mux.Vars(r)["slug"])
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
		return fn(cn, h, env, w, r)
	}
}

type runningHost *host.Host
type hostFn func(h runningHost, env *middleware.Env, w http.ResponseWriter, r *http.Request) error

// requiresRunningHost is a wrapper which guarantees the action is called for a running host
func requiresRunningHost(fn hostFn) func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	return func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if h.State != host.StateRunning {
			return controllers.ErrHostNotRunning
		}
		return fn(h, env, w, r)
	}
}

func hydrateContainer(c *container.Container, fc *freighterclient.Container, h *host.Host) *container.Container {
	c.SetState(fc.State)
	c.CID = fc.ID
	c.Image = fc.Image
	c.Slug = fc.Name
	c.IPAddress = fc.Host
	c.GatewayIP = fc.Gateway
	c.Cmd = fc.Cmd
	c.Entrypoint = fc.Entrypoint
	c.ExitCode = fc.ExitCode
	c.StartedAt = timePtr(fc.StartedAt)
	c.FinishedAt = timePtr(fc.FinishedAt)

	return c
}

func mkContainer(fc *freighterclient.Container, h *host.Host) *container.Container {
	c := &container.Container{
		CID:        fc.ID,
		Image:      fc.Image,
		Slug:       fc.Name,
		IPAddress:  fc.Host,
		GatewayIP:  fc.Gateway,
		UserID:     h.UserID,
		HostID:     h.ID,
		Cmd:        fc.Cmd,
		Entrypoint: fc.Entrypoint,
		ExitCode:   fc.ExitCode,
		StartedAt:  timePtr(fc.StartedAt),
		FinishedAt: timePtr(fc.FinishedAt),
	}
	c.SetState(fc.State)
	return c
}

func publishContainerEvent(env *middleware.Env, name string, c *container.Container) {
	env.Events.Publish(
		controllers.NewEvent(name, serializers.SerializeContainer(c)),
		env.User.Topic(),
	)
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
	hvols, err := volume.FindAll(d, h.ID)
	if err != nil {
		return nil, err
	}
	for _, hv := range hvols {
		slugsToVols[hv.Slug] = hv
	}
	return slugsToVols, nil
}

func timePtr(t time.Time) *time.Time {
	if !t.IsZero() {
		return &t
	}
	return nil
}
