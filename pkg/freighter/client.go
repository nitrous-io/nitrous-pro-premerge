package freighter

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

var (
	ResponseError = errors.New("freighter: unexpected response")
)

// SharedHTTPClient is the shared default http.Client used by the convenience
// List, Create, Info, Start, Stop, Restart, Terminate methods.
var SharedHTTPClient = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// Client represents a Freighter API consumer.
type Client interface {
	List(*Host) ([]*Container, error)
	// Create()
	// Info()
	// Start()
	// Stop()
	// Restart()
	// Terminate()
}

type (
	Container struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Host    string `json:"host"`
		State   string `json:"state"`
		Image   string `json:"image"`
		SSHPort int    `json:"ssh_port"`
	}

	containerResponse struct {
		*Container `json:"container"`
	}

	containersResponse struct {
		Containers []*Container `json:"containers"`
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
	url := h.BaseURL() + "/containers"
	req, err := http.NewRequest("GET", url, nil)
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

	var containers containersResponse
	err = json.NewDecoder(resp.Body).Decode(&containers)
	resp.Body.Close()
	if err != nil {
		return nil, ResponseError
	}

	return containers.Containers, nil
}
