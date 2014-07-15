package freighter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	StateRunning = "running"
	StateStopped = "stopped"
)

var (
	// ErrUnexpectedResponse is returned when any of the API requests receives
	// an unexpected response body.
	ErrUnexpectedResponse = errors.New("freighter: unexpected response")

	// ErrNotFound is returned when trying to perform an action against a
	// container that does not exist.
	ErrNotFound = errors.New("freighter: container not found")
)

// SharedHTTPClient is the shared default http.Client used by Clients created
// with NewClient() without a given http.Client.
var SharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
		Dial: (&net.Dialer{
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// Client represents a Freighter API consumer.
type Client interface {
	List(host *Host) ([]*Container, error)
	Create(host *Host, name, image string) (*Container, error)

	// Info returns the Container with the given id.
	// If there is no Container with the given id, Info should return
	// ErrNotFound.
	Info(host *Host, id string) (*Container, error)

	Start(host *Host, id string) (*Container, error)
	Stop(host *Host, id string) (*Container, error)
	Restart(host *Host, id string) (*Container, error)
	Terminate(host *Host, id string) (*Container, error)
}

type (
	Container struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Host    string `json:"host"`
		State   string `json:"state"`
		Image   string `json:"image"`
		SSHPort int    `json:"ssh_port,omitempty"`
	}

	containerResponse struct {
		*Container `json:"container"`

		// TODO "result" seems redundant when Container.State is set and we are
		// getting a HTTP 200 OK response.
		Result string `json:"result,omitempty"`
	}

	containersResponse struct {
		Containers []*Container `json:"containers"`
	}

	errorResponse struct {
		Error string `json:error`
	}
)

type RequestFailedError struct {
	Req *http.Request
	Err error
}

func (e RequestFailedError) Error() string {
	return fmt.Sprintf("freighter: request %s %s failed: %s",
		e.Req.Method, e.Req.URL.String(), e.Err.Error())
}

type InvalidParamError struct {
	Msg string
}

func (e InvalidParamError) Error() string {
	return fmt.Sprintf("freighter: invalid param: %s", e.Msg)
}

type StatusError struct {
	Code int
}

func (e StatusError) Error() string {
	return fmt.Sprintf("freighter: unexpected HTTP %d", e.Code)
}

type client struct {
	*http.Client
}

// NewClient returns a new Freighter API Client.
// httpclient is optional and defaults to SharedHTTPClient.
//
// Clients wrap around the underlying HTTP request/response business and are
// safe for concurrent use in multiple goroutines.
func NewClient(httpclient *http.Client) Client {
	if httpclient == nil {
		httpclient = SharedHTTPClient
	}
	return &client{Client: httpclient}
}

// List makes an API request to a given Freighter Host and returns a list of
// Containers.
func (c *client) List(h *Host) ([]*Container, error) {
	u := h.BaseURL() + "/containers"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, &RequestFailedError{Req: req, Err: err}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{resp.StatusCode}
	}

	var cntnrs containersResponse
	err = json.NewDecoder(resp.Body).Decode(&cntnrs)
	resp.Body.Close()
	if err != nil {
		return nil, ErrUnexpectedResponse
	}

	return cntnrs.Containers, nil
}

// Create makes an API request to a given Freighter Host to create a container
// with the given name and image.
func (c *client) Create(h *Host, name, image string) (*Container, error) {
	u := h.BaseURL() + "/containers"
	data := url.Values{}
	data.Set("name", name)
	data.Set("image", image)
	req, err := http.NewRequest("POST", u, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.Do(req)
	if err != nil {
		return nil, &RequestFailedError{Req: req, Err: err}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		// Success, do nothing.
	case 422:
		var errResp errorResponse
		err = json.NewDecoder(resp.Body).Decode(&errResp)
		resp.Body.Close()
		if err != nil {
			return nil, ErrUnexpectedResponse
		}
		return nil, &InvalidParamError{errResp.Error}
	default:
		return nil, &StatusError{resp.StatusCode}
	}

	containerResp, err := unmarshalContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return containerResp.Container, nil
}

// Info makes an API request to a given Freighter Host and returns the
// Container with the given id.
// If the Container does not exist, ErrNotFound is returned.
func (c *client) Info(h *Host, id string) (*Container, error) {
	u := h.BaseURL() + "/containers/" + id
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, &RequestFailedError{Req: req, Err: err}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		// Success, do nothing.
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, &StatusError{resp.StatusCode}
	}

	containerResp, err := unmarshalContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return containerResp.Container, nil
}

// Start makes an API request to a given Freighter host to start the Container
// with the given id. The Container is returned.
// If the Container does not exist, ErrNotFound is returned.
func (c *client) Start(h *Host, id string) (*Container, error) {
	return c.containerControl(h, id, "start")
}

// Stop makes an API request to a given Freighter host to stop the Container
// with the given id. The Container is returned.
// If the Container does not exist, ErrNotFound is returned.
func (c *client) Stop(h *Host, id string) (*Container, error) {
	return c.containerControl(h, id, "stop")
}

// Restart makes an API request to a given Freighter host to restart the
// Container with the given id. The Container is returned.
// If the Container does not exist, ErrNotFound is returned.
func (c *client) Restart(h *Host, id string) (*Container, error) {
	return c.containerControl(h, id, "restart")
}

// Terminate makes an API request to a given Freighter host to terminate the
// Container with the given id. The Container is returned.
// If the Container does not exist, ErrNotFound is returned.
func (c *client) Terminate(h *Host, id string) (*Container, error) {
	u := fmt.Sprintf("%s/containers/%s", h.BaseURL(), id)
	req, err := http.NewRequest("DELETE", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, &RequestFailedError{Req: req, Err: err}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		// Success, do nothing.
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, &StatusError{resp.StatusCode}
	}

	containerResp, err := unmarshalContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return containerResp.Container, nil
}

func (c *client) containerControl(h *Host, id string, action string) (*Container, error) {
	u := fmt.Sprintf("%s/containers/%s/%s", h.BaseURL(), id, action)
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.Do(req)
	if err != nil {
		return nil, &RequestFailedError{Req: req, Err: err}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		// Success, do nothing.
	case http.StatusNotFound:
		return nil, ErrNotFound
	default:
		return nil, &StatusError{resp.StatusCode}
	}

	containerResp, err := unmarshalContainerResponse(resp)
	if err != nil {
		return nil, err
	}
	return containerResp.Container, nil
}

func unmarshalContainerResponse(resp *http.Response) (*containerResponse, error) {
	var cntnr containerResponse
	err := json.NewDecoder(resp.Body).Decode(&cntnr)
	resp.Body.Close()
	if err != nil {
		return nil, ErrUnexpectedResponse
	}
	return &cntnr, nil
}
