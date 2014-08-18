package test

import (
	"database/sql"
	"fmt"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
)

type HostStack struct {
	R    *region.Region
	P    *plan.Plan
	PT   *providertemplate.ProviderTemplate
	H    *host.Host
	U    *user.User
	Cert *cert.Cert
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

	err = hs.H.Insert(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	// create a certificate for the host
	crt := &cert.Cert{
		CA:         localhostCert,
		ServerCert: localhostCert,
		ServerKey:  localhostKey,
	}
	if err := crt.Insert(tx, hs.H.ID); err != nil {
		tx.Rollback()
		return nil, err
	}
	hs.Cert = crt

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	HostStackCount += 1

	return hs, nil
}

// These certs are the same as the certs embeded with net/http/httptest.
// If there's mismatched certs error, look at net/http/httptest/server.go to
// check if the cert has changed.
//
// localhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at the last second of 2049 (the end
// of ASN.1 time).
// generated from src/pkg/crypto/tls:
// go run generate_cert.go  --rsa-bits 512 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = `-----BEGIN CERTIFICATE-----
MIIBdzCCASOgAwIBAgIBADALBgkqhkiG9w0BAQUwEjEQMA4GA1UEChMHQWNtZSBD
bzAeFw03MDAxMDEwMDAwMDBaFw00OTEyMzEyMzU5NTlaMBIxEDAOBgNVBAoTB0Fj
bWUgQ28wWjALBgkqhkiG9w0BAQEDSwAwSAJBAN55NcYKZeInyTuhcCwFMhDHCmwa
IUSdtXdcbItRB/yfXGBhiex00IaLXQnSU+QZPRZWYqeTEbFSgihqi1PUDy8CAwEA
AaNoMGYwDgYDVR0PAQH/BAQDAgCkMBMGA1UdJQQMMAoGCCsGAQUFBwMBMA8GA1Ud
EwEB/wQFMAMBAf8wLgYDVR0RBCcwJYILZXhhbXBsZS5jb22HBH8AAAGHEAAAAAAA
AAAAAAAAAAAAAAEwCwYJKoZIhvcNAQEFA0EAAoQn/ytgqpiLcZu9XKbCJsJcvkgk
Se6AbGXgSlq+ZCEVo0qIwSgeBqmsJxUu7NCSOwVJLYNEBO2DtIxoYVk+MA==
-----END CERTIFICATE-----`

// localhostKey is the private key for localhostCert.
var localhostKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBPAIBAAJBAN55NcYKZeInyTuhcCwFMhDHCmwaIUSdtXdcbItRB/yfXGBhiex0
0IaLXQnSU+QZPRZWYqeTEbFSgihqi1PUDy8CAwEAAQJBAQdUx66rfh8sYsgfdcvV
NoafYpnEcB5s4m/vSVe6SU7dCK6eYec9f9wpT353ljhDUHq3EbmE4foNzJngh35d
AekCIQDhRQG5Li0Wj8TM4obOnnXUXf1jRv0UkzE9AHWLG5q3AwIhAPzSjpYUDjVW
MCUXgckTpKCuGwbJk7424Nb8bLzf3kllAiA5mUBgjfr/WtFSJdWcPQ4Zt9KTMNKD
EUO0ukpTwEIl6wIhAMbGqZK3zAAFdq8DD2jPx+UJXnh0rnOkZBzDtJ6/iN69AiEA
1Aq8MJgTaYsDQWyU/hDq5YkDJc9e9DSCvUIzqxQWMQE=
-----END RSA PRIVATE KEY-----`
