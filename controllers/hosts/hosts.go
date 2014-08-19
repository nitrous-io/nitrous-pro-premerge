package hosts

import (
	"crypto/rsa"
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/pkg/securerandom"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	errSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "size is required")
	errSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errRegionRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "region is required")
	errPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	errRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	errProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	errHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not stopped")
	errHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	errHostNotRunning           = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
)

var (
	validations = map[string]error{
		"index_hosts_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var HostbuilderClient *hostbuilderclient.Hostbuilder
var aesKey string

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

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	size := r.FormValue("size")
	if size == "" {
		return errSizeRequired
	}

	slug := r.FormValue("slug")
	if slug == "" {
		return errSlugRequired
	}

	regionName := r.FormValue("region")
	if regionName == "" {
		return errRegionRequired
	}

	name := r.FormValue("name")
	if name == "" {
		name = slug
	}

	// TODO: check if user account is eligible to create hosts (has a credit card on file)
	pl, err := plan.FindByName(db, size)
	if err != nil {
		return err
	}
	if pl == nil {
		return errPlanNotFound
	}

	rg, err := region.FindByName(db, regionName)
	if err != nil {
		return err
	}
	if rg == nil {
		return errRegionNotFound
	}

	pts, err := providertemplate.FindAllByRegionAndPlan(db, rg.ID, pl.ID)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return errProvidertemplateNotFound
	}
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	pt := pts[0]

	priv, pubkey, err := generateKey()
	if err != nil {
		return err
	}

	secret, err := securerandom.Base64(191)
	if err != nil {
		return err
	}

	h := &host.Host{
		UserID:             env.User.ID,
		RegionID:           rg.ID,
		PlanID:             pl.ID,
		ProviderTemplateID: pt.ID,
		Slug:               slug,
		Name:               sql.NullString{name, true},
		MaxContainers:      sql.NullInt64{pl.MaxContainers, true},
		Size:               sql.NullString{size, true},
		State:              host.StateBorn,
		Secret:             secret,
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	err = h.Insert(tx)
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

	// generate hostbuilder config
	config := hostbuilderclient.Config{}
	for k, v := range pt.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}
	config["name"] = slug
	config["public_keys"] = []string{string(pubkey)}
	config["secret"] = secret

	info, err := HostbuilderClient.Create(pt.Provider, &config)
	if err != nil {
		h.State = host.StateOrphan
		updErr := h.Update(db)
		if updErr != nil {
			env.Logger.Errorf("Unable to update host %d to orphan state that failed on hostbuilder: %v", h.ID, updErr)
		}
		return err
	}

	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.ProviderIdentifier = sql.NullString{info.ID, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
	h.State = host.StateRunning

	tx, err = db.Beginx()
	if err != nil {
		return err
	}
	err = h.Update(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	if _, err = keypairs.Create(tx, priv, aesKey, h.ID); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
		"host":   serializers.SerializeHost(h),
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

	serializeHost(env, h)
	return nil
}

func Stats(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Host should be running and have an IP address.
	if h.State != host.StateRunning || h.IPAddress.String == "" {
		return errHostNotRunning
	}

	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	info, err := cl.HostInfo()
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
	_, err := HostbuilderClient.Delete(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	err = h.Delete(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	err = container.DeleteAllByHostID(tx, h.ID)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err = keypairs.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	serializeHost(env, h)
	return nil
}

func Start(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	err := transition(env.DB, h,
		&stateTransition{host.StateStopped, host.StateStarting, host.StateRunning},
		HostbuilderClient.Start)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return errHostStartInvalidState
		}
		return err
	}

	serializeHost(env, h)
	return nil
}

func Stop(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	err := transition(env.DB, h,
		&stateTransition{host.StateRunning, host.StateStopping, host.StateStopped},
		HostbuilderClient.Stop)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return errHostStopInvalidState
		}
		return err
	}

	serializeHost(env, h)
	return nil
}

func Restart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	err := transition(env.DB, h,
		&stateTransition{host.StateRunning, host.StateRestarting, host.StateRunning},
		HostbuilderClient.Restart)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return errHostRestartInvalidState
		}
		return err
	}

	serializeHost(env, h)
	return nil
}

// stateTransition reflects the transition of host states during a hostbuilder operation.
type stateTransition struct {
	From         string
	Intermediate string
	To           string
}

type stateTransitionError struct {
	Expected string
	Actual   string
}

func (e *stateTransitionError) Error() string {
	return fmt.Sprintf("Expected initial state to be %s, was %s", e.Expected, e.Actual)
}

func transition(d ndb.QueryReceiver, h *host.Host, st *stateTransition, fn hostbuilderclient.OperationFn) error {
	if h.State != string(st.From) {
		return &stateTransitionError{Expected: string(st.From), Actual: h.State}
	}

	h.State = st.Intermediate
	err := h.Update(d)
	if err != nil {
		return err
	}

	info, err := fn(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	h.State = st.To
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	err = h.Update(d)
	if err != nil {
		return err
	}

	return nil
}

// generateKey generates and returns a private key together with its public key.
func generateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
	priv, err = keypairs.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	pub, err = keypairs.PublicKey(&priv.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	return priv, pub, nil
}

func serializeHost(env *middleware.Env, h *host.Host) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
	return
}
