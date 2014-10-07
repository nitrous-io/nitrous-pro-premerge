package hosts

import (
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	errSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "size is required")
	errSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errSlugInvalid              = httperror.New(http.StatusBadRequest, "invalid_request", "slug is not valid")
	errRegionRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "region is required")
	errPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	errRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	errHostCreationNotAllowed   = httperror.New(http.StatusBadRequest, "invalid_request", "not allowed to create more hosts")
	errProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	errHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not stopped")
	errHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errHostNotRunning           = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errCertNotFound             = httperror.New(http.StatusNotFound, "not_found", "host certificate does not exist")
	errMultipleRequests         = httperror.New(http.StatusConflict, "conflict", "another host creation request is currently processed")
)

var (
	validations = map[string]error{
		"index_hosts_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var HostbuilderClient *hostbuilderclient.Hostbuilder
var defaultProvider string
var aesKey string
var defaultAPTLocation string
var defaultAPTDist string

func init() {
	hostbuilderURL := os.Getenv("HOSTBUILDER_URL")
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		panic("AES_KEY is not set")
	}

	defaultProvider = os.Getenv("DEFAULT_PROVIDER")
	if defaultProvider == "" {
		panic("DEFAULT_PROVIDER is not set")
	}

	defaultAPTLocation = os.Getenv("DEFAULT_APT_LOCATION")
	if defaultAPTLocation == "" {
		panic("DEFAULT_APT_LOCATION is not set")
	}

	defaultAPTDist = os.Getenv("DEFAULT_APT_DIST")
	if defaultAPTDist == "" {
		panic("DEFAULT_APT_DIST is not set")
	}
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(env.DB, env.User)
	if err != nil {
		return err
	}

	hostsResp := make([]*serializers.Host, len(hosts))
	for i, h := range hosts {
		hostsResp[i] = serializers.SerializeHost(&h)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hostsResp,
	}
	return nil
}

func Show(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	serializeHost(env, h)
	return nil
}

func Update(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// since name can be null, we interpret absence of name field to mean no
	// change, and presence of name field with empty string to mean set it to
	// null
	r.ParseForm()
	if _, ok := r.PostForm["name"]; !ok {
		serializeHost(env, h)
		return nil
	}

	name := r.PostFormValue("name")
	h.Name.String, h.Name.Valid = name, name != ""
	if err := h.Update(env.DB); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostUpdated, h)
	serializeHost(env, h)
	return nil
}

var defaultPeriod = 100 * time.Millisecond

func Stats(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Host should be running and have an IP address.
	if h.State != host.StateRunning || h.IPAddress.String == "" {
		return errHostNotRunning
	}

	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	period, err := time.ParseDuration(r.FormValue("period"))
	if err != nil || period == 0 {
		period = defaultPeriod
	}

	info, err := cl.HostInfo(period)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"stats": serializers.SerializeHostInfo(info),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// If provider identifier is empty, don't attempt to delete the host in hostbuilder
	// Provider identifier can be empty for hosts that are dangling in born or orphan states.
	if h.ProviderIdentifier.String != "" {
		_, err := HostbuilderClient.Delete(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
		if err != nil {
			return err
		}
	}

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Delete(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := container.DeleteAllByHostID(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err = keypairs.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := cert.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostDestroyed, h)
	serializeHost(env, h)
	return nil
}

func Start(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateStopped {
		return errHostStartInvalidState
	}
	if err := transition(env.DB, h,
		host.StateStarting, host.StateRunning,
		HostbuilderClient.Start); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostStarted, h)
	serializeHost(env, h)
	return nil
}

func Stop(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostStopInvalidState
	}

	if err := transition(env.DB, h,
		host.StateStopping, host.StateStopped,
		HostbuilderClient.Stop); err != nil {
		return err
	}

	publishContainersStopped(env, h)
	publishHostEvent(env, controllers.EventHostStopped, h)
	serializeHost(env, h)
	return nil
}

func Restart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostRestartInvalidState
	}

	if err := transition(env.DB, h,
		host.StateRestarting, host.StateRunning,
		HostbuilderClient.Restart); err != nil {
		return err
	}

	publishContainersStopped(env, h)
	publishHostEvent(env, controllers.EventHostRestarted, h)
	serializeHost(env, h)
	return nil
}

func transition(d ndb.QueryReceiver, h *host.Host, midState, endState string, fn hostbuilderclient.OperationFn) error {
	h.State = midState
	if err := h.Update(d); err != nil {
		return err
	}

	info, err := fn(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	h.State = endState
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	if err := h.Update(d); err != nil {
		return err
	}

	return nil
}

func serializeHost(env *middleware.Env, h *host.Host) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
}

func publishHostEvent(env *middleware.Env, name controllers.EventName, h *host.Host) {
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Host{
			"host": serializers.SerializeHost(h),
		}),
		env.User.Topic(),
	)
}

func publishContainersStopped(env *middleware.Env, h *host.Host) {
	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		env.Logger.Errorf("hosts.Stop: Unable to list host containers: %v", err)
		return
	}
	for _, cn := range cns {
		cn.State = container.StateStopped
		env.Events.Publish(
			controllers.NewEvent(controllers.EventContainerStopped, map[string]*serializers.Container{
				"container": serializers.SerializeContainer(cn),
			}),
			env.User.Topic(),
		)
	}
}
