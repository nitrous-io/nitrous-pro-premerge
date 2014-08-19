package host

import (
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
)

var (
	nameRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug is not valid")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 63 characters)")
)

type ProviderTemplate struct {
	Provider string `db:"provider"`
}

const (
	StateBorn       = "born"
	StateRunning    = "running"
	StateStopped    = "stopped"
	StateStopping   = "stopping"
	StateStarting   = "starting"
	StateRestarting = "restarting"
	StateOrphan     = "orphan"
	StateTerminated = "terminated"
)

type Host struct {
	ID                 int64 `db:"id"`
	UserID             int64 `db:"user_id"`
	RegionID           int64 `db:"region_id"`
	PlanID             int64 `db:"plan_id"`
	ProviderTemplateID int64 `db:"provider_template_id"`

	// Slug is used as a DNS label in the host's FQDN (thus limited to 63 chars).
	Slug           string         `db:"slug"`
	Name           sql.NullString `db:"name"`
	State          string         `db:"state"`
	IPAddress      sql.NullString `db:"ip_address"`
	FreighterdPort sql.NullInt64  `db:"freighterd_port"`

	// shared secret with host used for authentication
	Secret string `db:"secret"`

	// ProviderIdentifier is the identifier used by the host's provider to
	// uniquely identify this host.
	ProviderIdentifier sql.NullString `db:"provider_identifier"`

	// MaxContainers is the maximum number of containers that can be created on
	// this host. This is copied over from the associated Plan so that it can
	// be changed independently when necessary.
	MaxContainers sql.NullInt64 `db:"max_containers"`

	// Size is currently the name of the plan of this host. This is copied over
	// from the associated Plan since size is often read.
	Size sql.NullString `db:"size"`

	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`

	*ProviderTemplate
}

func FindAllByUser(db *db.Database, user *user.User) ([]Host, error) {
	return findAllByUserID(db, user.ID)
}

func findAllByUserID(db *db.Database, userID int64) ([]Host, error) {
	hosts := []Host{}
	err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL ORDER BY id;", userID)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindByID(db *db.Database, id int64) (*Host, error) {
	host := &Host{}
	err := db.Get(host, `SELECT * FROM hosts WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return host, err
}

func FindByUserAndID(db *db.Database, user *user.User, id int64) (*Host, error) {
	host := &Host{}
	q := `SELECT hosts.*, provider_templates.provider
				FROM hosts
				INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
				WHERE hosts.user_id = $1 AND hosts.ID = $2 AND terminated_at IS NULL;`
	err := db.Get(host, q, user.ID, id)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return host, err
}

func FindByUserAndSlug(db *db.Database, user *user.User, slug string) (*Host, error) {
	host := &Host{}
	q := `SELECT hosts.*, provider_templates.provider
				FROM hosts
				INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
				WHERE hosts.user_id = $1 AND hosts.slug = $2 AND terminated_at IS NULL;`
	err := db.Get(host, q, user.ID, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return host, err
}

func (h *Host) Insert(d db.QueryReceiver) error {
	if h.State == "" {
		h.State = StateBorn
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO hosts
		(user_id, region_id, plan_id, provider_template_id, slug, name,
			state, ip_address, provider_identifier, max_containers, size, freighterd_port, secret)
		VALUES
		(:user_id, :region_id, :plan_id, :provider_template_id, :slug,
		:name, :state, :ip_address, :provider_identifier, :max_containers, :size, :freighterd_port, :secret)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (name, state, ip_address, provider_identifier, max_containers, freighterd_port, updated_at)
		= (:name, :state, :ip_address, :provider_identifier, :max_containers, :freighterd_port, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) Delete(d db.QueryReceiver) error {
	return d.QueryRowx(`UPDATE hosts SET terminated_at = now(), state = $1 WHERE id = $2 RETURNING *;`, StateTerminated, h.ID).StructScan(h)
}

func (h *Host) Validate(db *db.Database) error {
	if h.Slug == "" {
		return ErrSlugBlank
	}
	if !nameRe.MatchString(h.Slug) {
		return ErrSlugInvalid
	}
	nameLen := len(h.Slug)
	if nameLen < 2 {
		return ErrSlugTooShort
	}
	if nameLen > 63 {
		return ErrSlugTooLong
	}
	return nil
}

// Hostname returns the public domain that the host can be reached. This should be the
// same as the DNS address which we map the the host IP address
//
// We assume that slug is vald ( its cant be empty ), therefore we wont check
// it here
func (h *Host) Hostname() string {
	return h.Slug + ".nitrouspro.com" // FIXME
}
