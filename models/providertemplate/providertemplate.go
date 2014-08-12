package providertemplate

import (
	"database/sql"
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/nitrous/db"
)

type ProviderTemplate struct {
	ID        int64         `db:"id"`
	Name      string        `db:"name"`
	Provider  string        `db:"provider"`
	Config    hstore.Hstore `db:"config"`
	CreatedAt *time.Time    `db:"created_at"`
	UpdatedAt *time.Time    `db:"updated_at"`
}

func FindByID(d db.QueryReceiver, id int64) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	err := d.Get(pt, "SELECT * FROM provider_templates WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return pt, nil
}

func (pt *ProviderTemplate) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO provider_templates
		(name, provider, config)
		VALUES (:name, :provider, :config)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pt).StructScan(pt)
}
