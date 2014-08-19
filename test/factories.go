package test

import (
	"database/sql"
	"fmt"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
)

type HostStack struct {
	R  *region.Region
	P  *plan.Plan
	PT *providertemplate.ProviderTemplate
	H  *host.Host
	U  *user.User
}

// track number of hoststacks successfully created so we can generate unique
// fields
var HostStackCount int

// CreateHostStack returns a HostStack with created user,
// region, plan, providertemplate, and host.
// If any model fields in hs are present, they are used, otherwise
// placeholder ones are created.
// If any inserts fail, the entire transaction is rolled back.
func CreateHostStack(db *db.Database, hs *HostStack) (*HostStack, error) {
	tx, err := db.Beginx()
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if hs == nil {
		hs = &HostStack{}
	}

	makeUnique := func(val string) string {
		if HostStackCount == 0 {
			return val
		}
		return fmt.Sprintf("%s%d", val, HostStackCount)
	}

	if hs.U == nil {
		var username string
		if HostStackCount == 0 {
			username = "hoststackuser"
		} else {
			username = makeUnique("hoststackuser")
		}
		hs.U = &user.User{
			Username: username,
			Email:    fmt.Sprintf("%s@example.com", username),
			Password: "123456",
		}
	}
	if hs.U.ID == 0 {
		err = hs.U.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.R == nil {
		hs.R = &region.Region{Name: makeUnique("apse"), Description: "Southeast Asia"}
	}

	if hs.R.ID == 0 {
		err = hs.R.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.P == nil {
		hs.P = &plan.Plan{Name: makeUnique("micro"), MaxContainers: 1, Price: 500}
	}
	if hs.P.ID == 0 {
		err = hs.P.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.PT == nil {
		hs.PT = &providertemplate.ProviderTemplate{
			Name:     "digital-ocean-1",
			Provider: "digitalocean",
		}
	}
	hs.PT.RegionID = hs.R.ID
	hs.PT.PlanID = hs.P.ID

	if hs.PT.ID == 0 {
		err = hs.PT.Insert(tx)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if hs.H == nil {
		hs.H = &host.Host{}
	}
	if hs.H.Slug == "" {
		hs.H.Slug = makeUnique("hoststack")
	}
	if hs.H.State == "" {
		hs.H.State = host.StateRunning
	}

	if !hs.H.ProviderIdentifier.Valid {
		hs.H.ProviderIdentifier = sql.NullString{"host-0001", true}
	}

	hs.H.RegionID = hs.R.ID
	hs.H.PlanID = hs.P.ID
	hs.H.ProviderTemplateID = hs.PT.ID
	hs.H.UserID = hs.U.ID
	hs.H.Secret = "test-secret"

	err = hs.H.Insert(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	HostStackCount += 1

	return hs, nil
}
