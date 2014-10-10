package hostbuilderclient

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

type Config map[string]interface{}

type Hostbuilder struct {
	URL    string
	Client *httpclient.Client
}

func NewHostbuilder(url string) *Hostbuilder {
	client := httpclient.New("")
	// FIXME: make this a global setting
	v, _ := strconv.ParseBool(os.Getenv("NO_SSL_VERIFY"))
	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: v},
	}
	client.Client.Transport = tr
	return &Hostbuilder{URL: url, Client: client}
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config *Config) (*hostbuilder.Response, error) {
	var endpoint string
	if path == "" {
		endpoint = hb.URL + "/hosts"
	} else {
		endpoint = hb.URL + "/hosts/" + path
	}

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to encode config: %s", err)
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	response, err := hb.Client.PostForm(endpoint, &p)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to post: %s, err: %s", endpoint, err)
	}
	defer response.Body.Close()

	m := &hostbuilder.Response{}
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		err = json.NewDecoder(response.Body).Decode(m)
		if err != nil {
			return nil, fmt.Errorf("hostbuilder failed to decode: %s response, err: %s", endpoint, err)
		}
		return m, nil
	default:
		e, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, fmt.Errorf("hostbuilder error response %s, err: %s", endpoint, err)
		}
		return nil, fmt.Errorf("hostbuilder error response %s, err: %s", endpoint, string(e))
	}
}

// OperationFn defines an action that can be invoked on hostbuilder
type OperationFn func(provider string, config *Config) (*hostbuilder.Response, error)

// Create sends POST /hosts request to hostbuilder and stream response back to
// the provided writer
func (hb *Hostbuilder) Create(provider string, config *Config, out io.Writer) (*hostbuilder.Response, error) {
	endpoint := hb.URL + "/hosts"

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("hostbuilder failed to encode config: %s", err)
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	response, err := hb.Client.PostForm(endpoint, &p)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	pr, pw := io.Pipe()
	combined := io.MultiWriter(out, pw) // Tee response to output stream and pipe.

	// Stream hostbuilder progress to output streams.
	writeErrCh := make(chan error, 1)
	go func() {
		_, err = io.Copy(combined, response.Body)
		writeErrCh <- err
		pw.Close()
	}()

	// Check if JSON stream contains an error and return it as an error.
	decoder := json.NewDecoder(pr)
	for {
		msg := &hostbuilder.Response{}
		err := decoder.Decode(msg)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("hostbuilder: failed to decode stream: %s", err)
		}
		if msg.Error != "" {
			return nil, fmt.Errorf("hostbuilder: create failed with error: %s", msg.Error)
		}

		if msg.ID != "" {
			return msg, nil
		}
	}
	return nil, errors.New("hostbuilder: failed to return a proper host")
}

func (hb *Hostbuilder) Delete(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("delete", provider, config)
}

func (hb *Hostbuilder) Start(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("start", provider, config)
}

func (hb *Hostbuilder) Stop(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("stop", provider, config)
}

func (hb *Hostbuilder) Restart(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.defaultRequestHandler("restart", provider, config)
}
