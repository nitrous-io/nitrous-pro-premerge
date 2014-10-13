package test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/pkg/analytics"
)

var tables []string // retrieve list of tables once and store here

func TestDatabase(dbname string) (*db.Database, error) {
	d, err := ConnectToTestDatabase(dbname)
	if err != nil {
		return nil, err
	}
	err = d.Migrate(db.Migrations)
	if err != nil {
		d.Close()
		return nil, err
	}
	err = truncateTables(d)
	if err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func ConnectToTestDatabase(dbname string) (*db.Database, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	dburl := fmt.Sprintf("host=/var/run/postgresql user=%s dbname=%s sslmode=disable", u.Username, dbname)
	d, err := db.NewDatabase(dburl)
	if err != nil {
		if d != nil {
			d.Close()
		}
		return nil, err
	}
	return d, nil
}

func truncateTables(db *db.Database) error {
	if len(tables) == 0 {
		tbls, err := getTables(db)
		if err != nil {
			return err
		}
		tables = tbls
	}

	_, err := db.Exec(fmt.Sprintf(`TRUNCATE TABLE %s;`, strings.Join(tables, ", ")))
	return err
}

func getTables(db *db.Database) ([]string, error) {
	rows, err := db.Query(`SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations';`)
	if err != nil {
		return []string{}, err
	}
	defer rows.Close()
	tbls := []string{}
	for rows.Next() {
		var tablename string
		err = rows.Scan(&tablename)
		if err != nil {
			return []string{}, err
		}
		tbls = append(tbls, tablename)
	}
	return tbls, nil
}

func NewAPIRequest(token string, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := NewRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

func NewRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	return http.NewRequest(method, u.String(), nil)
}

func NewAuthedRequest(t *token.Token, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := NewRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	return req, nil
}

func NewFormRequest(method string, urlStr string, params *url.Values) (*http.Request, error) {
	var r io.Reader
	if params != nil {
		r = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequest(method, urlStr, r)
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, err
}

func NewAuthedFormRequest(t *token.Token, method string, urlStr string, params *url.Values) (*http.Request, error) {
	req, err := NewFormRequest(method, urlStr, params)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	return req, nil
}

// ErrorSlug expects a JSON object as string with 'error' and
// 'error_description' keys, and returns those respectively.  It swallows any
// errors and returns two empty strings.
func ErrorSlug(body io.Reader) (string, string) {
	j := map[string]string{}
	err := json.NewDecoder(body).Decode(&j)
	if err != nil {
		return "", ""
	}
	return j["error"], j["error_description"]
}

func TempFile(extension string) string {
	return path.Join(os.TempDir(), fmt.Sprintf("%v.%v", RandomName("freightertest"), extension))
}

func TempDir() (string, error) {
	dir := path.Join(os.TempDir(), RandomName("freightertest"))
	err := os.Mkdir(dir, 0755)
	if err != nil {
		return "", err
	}
	return dir, nil
}

func RandomName(name string) string {
	return fmt.Sprintf("%v-%v-%v", name, time.Now().UnixNano(), rand.Intn(1000))
}

func TLSConfig(hostname string, certPEM []byte, keyPEM []byte) (*tls.Config, error) {
	caInfo, err := controllers.GetCAInfo(true)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AddCert(caInfo.X509Cert)

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      certPool,
		ServerName:   hostname,
	}, nil
}

func WrapInTransaction(db *db.Database, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = fn(tx)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func StubAnalyticsClient() (*FakeAnalytics, func()) {
	origClient := analytics.Client
	fakeClient := &FakeAnalytics{}
	analytics.Client = fakeClient
	return fakeClient, func() { analytics.Client = origClient }
}
