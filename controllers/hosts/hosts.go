package hosts

import (
	"crypto/rsa"
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/serializers"
)

var (
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching provider found")
	ErrSlugRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	ErrHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not stopped")
	ErrHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")
	ErrHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running")

	hostbuilderURL string
	aesKey         string
)

var (
	validations = map[string]error{
		"index_hosts_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

const HostbuilderURLEnvVar = "HOSTBUILDER_URL"

var HostbuilderClient *hostbuilderclient.Hostbuilder

func init() {
	hostbuilderURL = os.Getenv(HostbuilderURLEnvVar)
	if hostbuilderURL == "" {
		panic(fmt.Sprintf("%v is not set", HostbuilderURLEnvVar))
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = os.Getenv("AES_KEY")
	if aesKey == "" {
		panic("AES_KEY is not set")
	}
}

func Index(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(c.DB(), env.User)
	if err != nil {
		return err
	}

	hostsResp := make([]*serializers.Host, len(hosts))
	for i, h := range hosts {
		hostsResp[i] = serializers.SerializeHost(&h, nil)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hostsResp,
	}
	return nil
}

func Create(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	slug := r.FormValue("slug")
	if slug == "" {
		return ErrSlugRequired
	}
	name := r.FormValue("name")
	if name == "" {
		name = slug
	}
	provider := r.FormValue("provider")
	if provider == "" {
		provider = providertemplate.DefaultProvider
	}

	pts, err := providertemplate.FindAllByProvider(db, provider)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		return ErrProvidertemplateNotFound
	}
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	pt := pts[0]

	priv, pubkey, err := generateKey()
	if err != nil {
		return err
	}

	h := &host.Host{
		UserID:             env.User.ID,
		ProviderTemplateID: pt.ID,
		Slug:               slug,
		Name:               sql.NullString{name, true},
		State:              host.StateBorn,
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

	info, err := HostbuilderClient.Create(pt.Provider, &config)
	if err != nil {
		updErr := h.UpdateState(db, host.StateOrphan)
		if updErr != nil {
			c.Logger().Errorf("Unable to update host %d to orphan state that failed on hostbuilder: %v", h.ID, updErr)
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
		"host":   serializers.SerializeHost(h, nil),
	}
	return nil
}

func Show(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	var info *freighterclient.HostInfo
	db := c.DB()

	// Request info from Freighter if host is running with an IP address.
	if h.IPAddress.String != "" && h.State == host.StateRunning {
		cl, err := controllers.NewFreighterClient(db, h)
		if err != nil {
			return err
		}
		info, err = cl.HostInfo()
		if err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h, info),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	_, err := HostbuilderClient.Delete(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	tx, err := c.DB().Beginx()
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

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h, nil),
	}
	return nil
}

func Start(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	err := transition(c.DB(), h,
		&stateTransition{host.StateStopped, host.StateStarting, host.StateRunning},
		HostbuilderClient.Start)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostStartInvalidState
		}
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func Stop(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	err := transition(c.DB(), h,
		&stateTransition{host.StateRunning, host.StateStopping, host.StateStopped},
		HostbuilderClient.Stop)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostStopInvalidState
		}
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func Restart(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	err := transition(c.DB(), h,
		&stateTransition{host.StateRunning, host.StateRestarting, host.StateRunning},
		HostbuilderClient.Restart)
	if err != nil {
		if _, ok := err.(*stateTransitionError); ok {
			return ErrHostRestartInvalidState
		}
		return err
	}

	respondOK(w, c.Logger())
	return nil
}

func respondOK(w http.ResponseWriter, logger *logger.Logger) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")
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

	err := h.UpdateState(d, st.Intermediate)
	if err != nil {
		return err
	}

	_, err = fn(h.Provider, &hostbuilderclient.Config{"id": h.ProviderIdentifier.String})
	if err != nil {
		return err
	}

	err = h.UpdateState(d, st.To)
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
