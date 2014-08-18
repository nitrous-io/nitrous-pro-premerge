package cert

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/nitrous/db"
)

type Cert struct {
	ID     int64 `db:"id"`
	HostID int64 `db:"host_id"`

	CA         string `db:"ca"`
	ServerCert string `db:"server_cert"`
	ServerKey  string `db:"server_key"`

	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}

// Insert creates a new entry for cert in database. We specifically use
// `*sql.Tx` becase we requires the cert to be inserted in the same transaction
// as the host
func (c *Cert) Insert(tx *sqlx.Tx, hostID int64) error {
	c.HostID = hostID
	stmt, err := tx.PrepareNamed(`INSERT INTO certs
		(host_id, ca, server_cert, server_key)
		VALUES (:host_id, :ca, :server_cert, :server_key)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return stmt.QueryRowx(c).StructScan(c)
}

// Find gets the cert belongs to a host from database
func Find(db db.QueryReceiver, hostID int64) (*Cert, error) {
	crt := &Cert{}
	q := `SELECT * FROM certs WHERE host_id = $1 AND terminated_at IS NULL`
	err := db.Get(crt, q, hostID)
	if err != nil {
		return nil, err
	}
	return crt, nil
}

// Delete terminates the cert belongs to a host in database. We specifically use
// `*sql.Tx` becase we requires the cert to be deleted in the same transaction
// as the host
func Delete(tx *sqlx.Tx, hostID int64) error {
	_, err := tx.Exec("UPDATE certs SET terminated_at = now() WHERE host_id = $1", hostID)
	return err
}
