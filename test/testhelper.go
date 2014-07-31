package test

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/token"
)

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
	err = TruncateTables(d)
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

func TruncateTables(db *db.Database) error {
	rows, err := db.Query(`SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations';`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var tablename string
		err = rows.Scan(&tablename)
		if err != nil {
			return err
		}
		_, err = db.Exec(fmt.Sprintf(`TRUNCATE TABLE %s RESTART IDENTITY CASCADE;`, tablename))
		if err != nil {
			return err
		}
	}
	return nil
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
	req, err := http.NewRequest(method, urlStr, strings.NewReader(params.Encode()))
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

// ErrorDetails expects a JSON object as string with 'error' and
// 'error_description' keys, and returns those respectively.  If an error is
// encountered while decoding, it returns "ERRDETAILS" and the error
// description in both strings
func ErrorDetails(body io.Reader) (string, string) {
	j := map[string]string{}
	err := json.NewDecoder(body).Decode(&j)
	if err != nil {
		r := fmt.Sprintf("ERRDETAILS: %s", err.Error())
		return r, r
	}
	return j["error"], j["error_description"]
}

func Wait() {
	runtime.Gosched()
	time.Sleep(50 * time.Millisecond)
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
