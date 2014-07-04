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

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/models/providertemplate"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

const hostbuilderURLEnvVar = "HOSTBUILDER_URL"

var hostbuilderURL string

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
		return httperror.New(http.StatusBadRequest, "invalid_request", "host size is required")
	}

	pl, err := plan.FindByName(db, size)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if pl == nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan for the given size")
	}

	if regionName == "" {
		regionName = "apse1" //FIXME: user model should specify a default region?
	}
	rg, err := region.FindByName(db, regionName)
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if rg == nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", "invalid region name")
	}

	pts, err := providertemplate.FindAllByRegionAndPlan(db, pl.ID, rg.ID)
	// TODO: Actual logic for picking the correct provider template
	// pick the first template for now
	if err != nil {
		return httperror.New(http.StatusBadRequest, "invalid_request", err.Error())
	}
	if len(pts) == 0 {
		return httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates for the given plan and regions combination")
	}
	pt := pts[0]

	config := pt.Config.Map
	if config == nil {
		config = map[string]sql.NullString{}
	}
	config["name"] = sql.NullString{name, true}
	config["public_keys"] = sql.NullString{"", true} //FIXME: pass actual public keys

	info, err := createInHostbuilder(pt.Provider, config)
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

func createInHostbuilder(provider string, config map[string]sql.NullString) (map[string]string, error) {
	h := os.Getenv(hostbuilderURLEnvVar)
	if h == "" {
		panic(fmt.Sprintf("Hostbuilder URL is not set. export %s environment variable with a valid URL.", hostbuilderURLEnvVar))
	}

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	p := url.Values{}
	p.Set("provider", provider)
	p.Set("config", string(cg))

	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: v},
	}

	client := &http.Client{Transport: tr}
	url := fmt.Sprintf("%s/host", h)

	//TODO: probably we need to timeout the request,
	//but leaving it out for the sake of simplicity.
	response, err := client.PostForm(url, p)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode == 201 {
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
