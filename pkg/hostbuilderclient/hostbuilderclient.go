package hostbuilderclient

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"

	"github.com/nitrous-io/hostbuilder"
	"github.com/nitrous-io/nitrous-common/httpclient"
)

var (
	ca  = os.Getenv("HOSTBUILDER_CA")
	crt = os.Getenv("HOSTBUILDER_CERT")
	key = os.Getenv("HOSTBUILDER_KEY")
)

type Config map[string]interface{}

type RequestHandler func(path string, provider string, config *Config) (*hostbuilder.Response, error)

type Hostbuilder struct {
	URL            string
	requestHandler RequestHandler
}

func NewHostbuilder(url string) *Hostbuilder {
	return &Hostbuilder{URL: url}
}

func tlsConfig() *tls.Config {
	cert, err := tls.LoadX509KeyPair(crt, key)
	if err != nil {
		fmt.Errorf("Failed to load X509 keypair - %s & %s: %s", crt, key, err)
	}

	tlsConfig := &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{cert},
	}

	certPool := x509.NewCertPool()
	caFile, err := ioutil.ReadFile(ca)
	if err != nil {
		fmt.Errorf("Failed to read CA certificate, %s: %s", ca, err)
	}

	certPool.AppendCertsFromPEM(caFile)
	tlsConfig.RootCAs = certPool

	return tlsConfig
}

func tlsClient() *httpclient.Client {
	client := httpclient.New("")

	config := tlsConfig()
	tr := &http.Transport{
		TLSClientConfig: config,
	}
	client.Client.Transport = tr

	return client
}

func (hb *Hostbuilder) defaultRequestHandler(path string, provider string, config *Config) (*hostbuilder.Response, error) {
	client := tlsClient()

	p := url.Values{}
	p.Set("provider", provider)

	cg, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	p.Set("config", string(cg))

	//TODO: probably we need to timeout the request,
	//but leaving it out for now, for the sake of simplicity.
	var url string
	if path == "" {
		url = hb.URL + "/hosts"
	} else {
		url = hb.URL + "/hosts/" + path
	}

	response, err := client.PostForm(url, &p)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	m := &hostbuilder.Response{}
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		err = json.NewDecoder(response.Body).Decode(m)
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

func (hb *Hostbuilder) OverrideRequestHandler(fn RequestHandler) {
	hb.requestHandler = fn
}

func (hb *Hostbuilder) getRequestHandler() RequestHandler {
	if hb.requestHandler != nil {
		return hb.requestHandler
	}
	return hb.defaultRequestHandler
}

type OperationFn func(provider string, config *Config) (*hostbuilder.Response, error)

func (hb *Hostbuilder) Create(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("", provider, config)
}

func (hb *Hostbuilder) Delete(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("delete", provider, config)
}

func (hb *Hostbuilder) Start(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("start", provider, config)
}

func (hb *Hostbuilder) Stop(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("stop", provider, config)
}

func (hb *Hostbuilder) Restart(provider string, config *Config) (*hostbuilder.Response, error) {
	return hb.getRequestHandler()("restart", provider, config)
}
