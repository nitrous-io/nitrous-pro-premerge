package hosts

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

const hostbuilderURLEnvVar = "HOSTBUILDER_URL"

var (
	ErrSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host size not provided")
	ErrHostRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "host not provided")
	ErrPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	ErrRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	ErrProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	ErrHostNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching host found for the given name")
)

func Index(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	hosts, err := host.FindAllByUser(db, u)
	if err != nil {
		return err
	}
	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{
		"hosts": hosts,
	})
	return nil
}

func Create(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	name := r.FormValue("name")
	longname := r.FormValue("longname")
	size := r.FormValue("size")
	regionName := r.FormValue("region")

	// TODO: check if user account is eligible to create hosts (has a credit card on file)

	if size == "" {
		return ErrSizeRequired
	}

	pl, err := plan.FindByName(db, size)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if pl == nil {
		return ErrPlanNotFound
	}

	if regionName == "" {
		regionName = "apse1" //FIXME: user model should specify a default region?
	}
	rg, err := region.FindByName(db, regionName)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if rg == nil {
		return ErrRegionNotFound
	}

	pts, err := providertemplate.FindAllByRegionAndPlan(db, pl.ID, rg.ID)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if len(pts) == 0 {
		return ErrProvidertemplateNotFound
	}
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	pt := pts[0]

	config := pt.Config.Map
	if config == nil {
		config = map[string]sql.NullString{}
	}
	config["name"] = sql.NullString{name, true}
	config["public_keys"] = sql.NullString{"", true} //FIXME: pass actual public keys

	info, err := doInHostbuilder("create", pt.Provider, config)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	if longname == "" {
		longname = name
	}

	h := &host.Host{
		UserID:             u.ID,
		RegionID:           rg.ID,
		PlanID:             pl.ID,
		ProviderTemplateID: pt.ID,

		Name:          name,
		LongName:      sql.NullString{longname, true},
		MaxContainers: sql.NullInt64{pl.MaxContainers, true},

		IPAddress:          sql.NullString{info["ip_address"], true},
		ProviderIdentifier: sql.NullString{info["id"], true},
		State:              "created",
	}
	h.Insert(db)

	c.RespondEncodeJSON(w, http.StatusCreated, map[string]interface{}{
		"host": h,
	})
	return nil
}

func HandleAction(c controllers.Helper, u *user.User, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	p := strings.Split(r.URL.Path, "/")
	action := p[len(p)-1]
	if action == "hosts" {
		action = "delete"
	}

	name := r.FormValue("name")
	if name == "" {
		return ErrHostRequired
	}

	ht, err := host.FindByUserAndName(db, u, name)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if ht == nil {
		return ErrHostNotFound
	}

	pt, err := providertemplate.FindByID(db, ht.ProviderTemplateID)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	config := pt.Config.Map
	if config == nil {
		config = map[string]sql.NullString{}
	}
	config["name"] = sql.NullString{ht.Name, true}
	config["public_keys"] = sql.NullString{"", true} //FIXME: pass actual public keys

	_, err = doInHostbuilder(action, pt.Provider, config)
	if err != nil {
		return httperror.New(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	}

	switch action {
	case "delete":
		ht.Delete(db)
	case "start", "restart":
		ht.State = "running"
		ht.UpdateColumn(db, "state")
	case "stop":
		ht.State = "stopped"
		ht.UpdateColumn(db, "state")
	}
	c.RespondEncodeJSON(w, http.StatusOK, map[string]interface{}{})
	return nil
}

func doInHostbuilder(action, provider string, config map[string]sql.NullString) (map[string]string, error) {
	h := os.Getenv(hostbuilderURLEnvVar)
	if h == "" {
		panic(fmt.Sprintf("Hostbuilder URL is not set. export %s environment variable with a valid URL.", hostbuilderURLEnvVar))
	}

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: v},
	}
	client := &http.Client{Transport: tr}
	urlParts := []string{h, "hosts", action}
	successCode := http.StatusOK

	if action == "create" {
		urlParts = urlParts[:len(urlParts)-1]
		successCode = http.StatusCreated
	}

	p := url.Values{}
	p.Set("provider", provider)
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	url := strings.Join(urlParts, "/")
	response, err := client.PostForm(url, p)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode == successCode {
		var m map[string]string
		err = json.NewDecoder(response.Body).Decode(&m)
		if err != nil {
			return nil, err
		}
		return m, nil
	}

	e, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	return nil, errors.New(string(e))
}
