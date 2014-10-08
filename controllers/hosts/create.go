package hosts

import (
	"crypto/rsa"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous-common/flusher"
	"github.com/nitrous-io/nitrous/pkg/httperror"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous/controllers"
	ndb "github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/keypairs"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/nitrous/serializers"
)

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	var size, regionSlug, slug, name string

	if size = r.FormValue("size"); size == "" {
		return errSizeRequired
	}
	if regionSlug = r.FormValue("region"); regionSlug == "" {
		return errRegionRequired
	}
	if slug = strings.ToLower(r.FormValue("slug")); slug == "" {
		return errSlugRequired
	}
	if name = r.FormValue("name"); name == "" {
		name = slug
	}

	ctx, err := newCreateContext(db, env.User, size, regionSlug, slug, name)
	if err != nil {
		return err
	}

	async, _ := strconv.ParseBool(r.FormValue("async"))
	// if async, we will just discard the progress data from hostbuilder. It will
	// only be outputed into the events stream. The only output we want to
	// include in async is the host data
	var writer io.Writer
	pp := &controllers.ProgressPublisher{
		EventName: controllers.EventHostCreating,
		Topic:     env.User.Topic(),
		Events:    env.Events,
		ID:        ctx.Host.ID,
		Slug:      ctx.Host.Slug,
	}
	if async {
		writer = pp
	} else {
		// turn off nginx buffer caching so that we can flush output to the
		// clients immediately
		w.Header().Set("X-Accel-Buffering", "no")
		writer = io.MultiWriter(pp, flusher.NewWriteFlusher(w))
	}
	h, err := ctx.run(env, writer, async)
	if err != nil {
		return err
	}
	if async {
		env.Status = http.StatusAccepted
	} else {
		env.Status = http.StatusCreated
	}
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
	return nil
}

// createContext handles the creation of host record and its dependencies in
// database and propagate the changes to host builder.
type createContext struct {
	User     *user.User
	Plan     *plan.Plan
	Region   *region.Region
	Template *providertemplate.ProviderTemplate
	Keypair  *keypairs.Keypair
	Cert     *cert.Cert
	Host     *host.Host
	Token    *token.Token
	Pubkey   []byte

	db *ndb.Database
}

// run propagates the changes to hostbuilder. If we want the creation to
// happens asynchronously, the current host infomation will be returned
// immediately while we build the host. Otherwise, the default is building the
// host asynchronously
func (ctx *createContext) run(env *middleware.Env, writer io.Writer, async bool) (*host.Host, error) {
	h, db := ctx.Host, ctx.db

	// get ca to save to the host
	caInfo, err := controllers.GetCAInfo(false)
	if err != nil {
		return nil, err
	}

	// generate hostbuilder config
	config := hostbuilderclient.Config{
		"name":         h.Slug,
		"secret":       h.Secret,
		"public_keys":  []string{string(ctx.Pubkey)},
		"apt_username": ctx.User.Username,
		"apt_password": ctx.Token.Token,
		"apt_location": defaultAPTLocation,
		"apt_dist":     defaultAPTDist,
		"tls": map[string]string{
			"ca":   string(caInfo.CertPEM),
			"cert": ctx.Cert.ServerCert,
			"key":  ctx.Cert.ServerKey,
		},
	}
	for k, v := range ctx.Template.Config.Map {
		if v.Valid {
			config[k] = v.String
		} else {
			config[k] = ""
		}
	}

	createHost := func() error {
		var err error
		defer func() {
			if err != nil {
				h.State = host.StateOrphan
				_ = h.Update(db) // Ignore error here.
				publishHostEvent(env, controllers.EventHostCreateFailed, h)
			}
		}()

		var info *hostbuilder.Response
		info, err = HostbuilderClient.Create(ctx.Template.Provider, &config, writer)
		if err != nil {
			return err
		}

		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.ProviderIdentifier = sql.NullString{info.ID, true}
		h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
		h.State = host.StateRunning

		var tx *sqlx.Tx
		tx, err = db.Beginx()
		if err != nil {
			return err
		}
		if err = h.Update(tx); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}

		publishHostEvent(env, controllers.EventHostCreated, h)
		return nil
	}

	if async {
		go createHost()
		return h, nil
	}
	err = createHost()
	return h, err
}

// create a new createContext with user-supplied parameters. This will tries to
// create a host with proper plan, region, template, etc..., associate them
// with the current context and save them to database
//
// TODO: check if user account is eligible to create hosts (has a credit card on file)
func newCreateContext(db *ndb.Database, user *user.User, size, regionSlug, slug, name string) (*createContext, error) {
	var err error

	ctx := &createContext{
		User: user,
		db:   db,
	}

	if ctx.Plan, err = findPlan(db, size); err != nil {
		return nil, err
	}
	if ctx.Region, err = findRegion(db, regionSlug); err != nil {
		return nil, err
	}
	if ctx.Template, err = findTemplate(db, ctx.Plan, ctx.Region, defaultProvider); err != nil {
		return nil, err
	}

	// Assuming that t is not nil cos if user gets here,
	// that means she has a token, but return err if there's
	// some other unexpected error
	if ctx.Token, err = token.FindByUser(db, user.ID); err != nil {
		return nil, err
	}

	// generate the keypair to communicate with the host
	priv, pubkey, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypairs for '%s', err %s", slug, err)
	}

	ctx.Pubkey = pubkey
	ctx.Host = &host.Host{
		UserID:             ctx.User.ID,
		RegionID:           ctx.Region.ID,
		PlanID:             ctx.Plan.ID,
		ProviderTemplateID: ctx.Template.ID,
		Slug:               slug,
		Name:               sql.NullString{name, true},
		MaxContainers:      sql.NullInt64{ctx.Plan.MaxContainers, true},
		Size:               sql.NullString{ctx.Plan.Slug, true},
		State:              host.StateBorn,
	}

	if err = ctx.Host.Validate(); err != nil {
		return nil, httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}

	// Save record to db first to reserve slug (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("failed to open a transaction for host %s, err: %s", slug, err)
	}

	// don't allow hosts to be created concurrently
	locked, err := ndb.TransactionLock(tx, ndb.HOST_CREATE_LOCK, int(user.ID))
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if !locked {
		tx.Rollback()
		return nil, errMultipleRequests
	}

	canCreateHosts, err := user.CanCreateHosts(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if !canCreateHosts {
		tx.Rollback()
		return nil, errHostCreationNotAllowed
	}

	if err := ctx.Host.Insert(tx); err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return nil, validationErr
			}
		}
		return nil, fmt.Errorf("failed to insert new host %s, err: %s", slug, err)
	}

	// create the cert
	if ctx.Cert, err = generateCerts(ctx.Host.Hostname()); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to generate certificates for '%s', err %s", slug, err)
	}
	if err := ctx.Cert.Insert(tx, ctx.Host.ID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to save certificates for '%s', err %s", slug, err)
	}

	// create the keypair
	ctx.Keypair, err = keypairs.Create(tx, priv, aesKey, ctx.Host.ID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to save keypairs for '%s', err %s", slug, err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to complete save transaction for '%s', err %s", slug, err)
	}

	return ctx, nil
}

func findPlan(db ndb.QueryReceiver, size string) (*plan.Plan, error) {
	pl, err := plan.FindBySlug(db, size)
	if err != nil {
		return nil, fmt.Errorf("no matching plan for '%s', error %s", size, err)
	}
	if pl == nil {
		return nil, errPlanNotFound
	}
	return pl, nil
}

func findRegion(db ndb.QueryReceiver, regionSlug string) (*region.Region, error) {
	r, err := region.FindBySlug(db, regionSlug)
	if err != nil {
		return nil, fmt.Errorf("no matching region for '%s', error %s", regionSlug, err)
	}
	if r == nil {
		return nil, errRegionNotFound
	}
	return r, nil
}

func findTemplate(db ndb.QueryReceiver, pl *plan.Plan, rg *region.Region, defaultProvider string) (*providertemplate.ProviderTemplate, error) {
	pt, err := providertemplate.FindByRegionAndPlanAndProvider(db, rg.ID, pl.ID, defaultProvider)
	if err != nil {
		return nil, fmt.Errorf("no matching templates found, error %s", err)
	}
	if pt == nil {
		return nil, errProvidertemplateNotFound
	}
	return pt, nil
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

// generate cert with server certificate and key. The cert will be used for
// TLS encryption
func generateCerts(hostname string) (*cert.Cert, error) {
	caInfo, err := controllers.GetCAInfo(true)
	if err != nil {
		return nil, err
	}
	return cert.Generate(hostname, caInfo.X509Cert, caInfo.Key)
}
