package host

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
)

var (
	nameRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
)

var (
	ErrNameBlank    = errors.New("name can't be blank")
	ErrNameInvalid  = errors.New("name is not valid")
	ErrNameTooShort = errors.New("name is too short (minimum is 2 characters)")
	ErrNameTooLong  = errors.New("name is too long (maximum is 63 characters)")
)

type Host struct {
	ID                 int64 `db:"id"`
	UserID             int64 `db:"user_id"`
	RegionID           int64 `db:"region_id"`
	PlanID             int64 `db:"plan_id"`
	ProviderTemplateID int64 `db:"provider_template_id"`

	// Name is the hostname and is used as a DNS label in a FQDN (thus limited
	// to 63 characters).
	Name      string         `db:"name"`
	LongName  sql.NullString `db:"long_name"`
	State     string         `db:"state"`
	IPAddress sql.NullString `db:"ip_address"`

	// ProviderIdentifier is the identifier used by the host's provider to
	// uniquely identify this host.
	ProviderIdentifier sql.NullString `db:"provider_identifier"`

	// MaxContainers is the maximum number of containers that can be created on
	// this host. This is copied over from the associated Plan so that it can
	// be changed independently when necessary.
	MaxContainers sql.NullInt64 `db:"max_containers"`

	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}

func FindAllByUser(db *db.Database, user *user.User) ([]Host, error) {
	return findAllByUserID(db, user.ID)
}

func findAllByUserID(db *db.Database, userID int64) ([]Host, error) {
	hosts := []Host{}
	err := db.Select(&hosts, "SELECT * from hosts WHERE user_id = $1 ORDER BY id", userID)
	if err != nil {
		return []Host{}, err
	}
	return hosts, nil
}

func FindByUserAndName(db *db.Database, user *user.User, name string) (*Host, error) {
	host := &Host{}
	err := db.Get(host, "SELECT * from hosts WHERE user_id = $1 AND name = $2 ORDER BY id", user.ID, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return host, err
}

func (h *Host) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO hosts
		(user_id, region_id, plan_id, provider_template_id, name, long_name,
			state, ip_address, provider_identifier, max_containers)
		VALUES
		(:user_id, :region_id, :plan_id, :provider_template_id, :name,
			:long_name, :state, :ip_address, :provider_identifier, :max_containers)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) UpdateColumn(d db.QueryReceiver, column string) error {
	q := fmt.Sprintf("UPDATE hosts SET %s = :%s WHERE id = :id", column, column)
	_, err := d.NamedExec(q, h)
	return err
}

func (h *Host) Delete(d db.QueryReceiver) error {
	_, err := d.NamedExec("DELETE FROM hosts WHERE id = :id", h)
	return err
}

func (h *Host) Validate(db *db.Database) error {
	if h.Name == "" {
		return ErrNameBlank
	}
	if !nameRe.MatchString(h.Name) {
		return ErrNameInvalid
	}
	nameLen := len(h.Name)
	if nameLen < 2 {
		return ErrNameTooShort
	}
	if nameLen > 63 {
		return ErrNameTooLong
	}
	return nil
}
