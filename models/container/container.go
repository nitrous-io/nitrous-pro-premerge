package container

import (
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/securerandom"
)

var slugRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*[a-z0-9]\z`)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug may only contain alphanumeric characters or dashes and cannot begin or end with a dash")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 255 characters)")
	ErrImageBlank   = errors.New("image can't be blank")
	ErrLocked       = errors.New("container is locked")
)

type Container struct {
	ID     int64          `db:"id"`
	CID    string         `db:"cid"`
	UserID int64          `db:"user_id"`
	HostID int64          `db:"host_id"`
	Image  string         `db:"image"`
	Slug   string         `db:"slug"`
	Name   sql.NullString `db:"name"`
	Secret string         `db:"secret"`

	TerminatedAt *time.Time `db:"terminated_at"`
	TouchedAt    *time.Time `db:"touched_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
	LockedAt     *time.Time `db:"locked_at"`

	// These fields come from Freighter.
	State      State
	IPAddress  string
	GatewayIP  string
	Cmd        []string
	Entrypoint []string
	ExitCode   int
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// Find queries for a container scoped to the user.
// identifier can be either an ID or a slug.
func Find(d db.QueryReceiver, user *user.User, identifier string) (*Container, error) {
	c := &Container{}
	id, err := strconv.ParseInt(identifier, 10, 64)

	if err != nil {
		err = d.Get(c,
			`SELECT * FROM containers WHERE user_id = $1 AND slug = $2 AND containers.terminated_at IS NULL;`, user.ID, identifier)
	} else {
		err = d.Get(c,
			`SELECT * FROM containers WHERE user_id = $1 AND id = $2 AND containers.terminated_at IS NULL;`, user.ID, id)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func FindByCID(d db.QueryReceiver, cid string) (*Container, error) {
	c := &Container{}
	err := d.Get(c, `SELECT * FROM containers WHERE cid = $1 AND terminated_at IS NULL;`, cid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func FindAllByHostID(d db.QueryReceiver, hostID int64) ([]*Container, error) {
	containers := []*Container{}
	err := d.Select(&containers, `SELECT * FROM containers WHERE host_id = $1 AND terminated_at IS NULL;`, hostID)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func DeleteAllByHostID(d db.QueryReceiver, hostID int64) error {
	_, err := d.Exec(`UPDATE containers SET terminated_at = now() WHERE host_id = $1;`, hostID)
	return err
}

func (c *Container) Insert(d db.QueryReceiver) error {
	if c.Secret == "" {
		sec, err := securerandom.Base64(191)
		if err != nil {
			return err
		}
		c.Secret = sec[:255]
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO containers
		(cid, user_id, host_id, image, slug, name, secret)
    VALUES (:cid, :user_id, :host_id, :image, :slug, :name, :secret)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE containers
		SET (cid, slug, name, image, touched_at, updated_at)
		= (:cid, :slug, :name, :image, :touched_at, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) Delete(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE containers SET terminated_at = now() WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) DeletePermanently(d db.QueryReceiver) error {
	if _, err := d.Exec(`DELETE FROM container_volumes WHERE container_id = $1`, c.ID); err != nil {
		return err
	}
	_, err := d.Exec(`DELETE FROM containers WHERE id = $1`, c.ID)
	return err
}

func (c *Container) Validate() error {
	if c.Slug == "" {
		return ErrSlugBlank
	}
	nameLen := len(c.Slug)
	if nameLen < 2 {
		return ErrSlugTooShort
	}
	if nameLen > 255 {
		return ErrSlugTooLong
	}
	if !slugRe.MatchString(c.Slug) {
		return ErrSlugInvalid
	}
	if c.Image == "" {
		return ErrImageBlank
	}
	return nil
}

func (c *Container) SetState(state string) bool {
	var ok bool
	switch state {
	case "running":
		c.State, ok = StateRunning, true
	case "stopped":
		c.State, ok = StateStopped, true
	}
	return ok
}

func (c *Container) ResolvedState() State {
	if c.TerminatedAt != nil {
		return StateTerminated
	}
	return c.State
}

func (c *Container) Lock(expiresIn time.Duration, d db.QueryReceiver) error {
	now := time.Now()
	expiredAt := now.Add(expiresIn)

	res, err := d.Exec(`
	UPDATE containers SET locked_at = $1 WHERE id = $2 AND (locked_at IS NULL OR locked_at < $3)
	`, expiredAt, c.ID, now)

	if err != nil {
		return err
	}

	i, _ := res.RowsAffected()
	if i < 1 {
		return ErrLocked
	}
	c.LockedAt = &expiredAt
	return nil
}

func (c *Container) Unlock(d db.QueryReceiver) error {
	_, err := d.Exec(`UPDATE containers SET locked_at = null WHERE id = $1;`, c.ID)
	c.LockedAt = nil
	return err
}
