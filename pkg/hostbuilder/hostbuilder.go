package hostbuilder

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

const hostbuilderURLEnvVar = "HOSTBUILDER_URL"

type Hostbuilder struct {
	Host     string
	Provider string
	Config   map[string]sql.NullString
}

func NewHostbuilder(host, provider string, config map[string]sql.NullString) *Hostbuilder {
	return &Hostbuilder{Host: host, Provider: provider, Config: config}
}

func (hb *Hostbuilder) sendRequest(path string) (map[string]string, error) {
	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: v},
	}
	client := &http.Client{Transport: tr}

	p := url.Values{}
	p.Set("provider", hb.Provider)

	cg, err := json.Marshal(hb.Config)
	if err != nil {
		return nil, err
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	url := hb.Host + path
	response, err := client.PostForm(url, p)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var m map[string]string
		err = json.NewDecoder(response.Body).Decode(&m)
		if err != nil {
			return nil, err
		}
		return m, nil
	default:
		e, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(string(e))
	}
}

func (hb *Hostbuilder) Create() (map[string]string, error) {
	return hb.sendRequest("/hosts")
}

func (hb *Hostbuilder) Delete() (map[string]string, error) {
	return hb.sendRequest("/hosts/delete")
}

func (hb *Hostbuilder) Start() (map[string]string, error) {
	return hb.sendRequest("/hosts/start")
}

func (hb *Hostbuilder) Stop() (map[string]string, error) {
	return hb.sendRequest("/hosts/stop")
}

func (hb *Hostbuilder) Restart() (map[string]string, error) {
	return hb.sendRequest("/hosts/restart")
}
