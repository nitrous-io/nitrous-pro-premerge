package freighter

import "fmt"

const defaultPort = 65343

// Host represents a Freighter host.
type Host struct {
	// Host is the hostname or IP address of a Freighter host.
	Host   string
	Port   int
	Secure bool
}

func NewHost(host string, port int, secure bool) *Host {
	if port == 0 {
		port = defaultPort
	}
	return &Host{Host: host, Port: port, Secure: secure}
}

func (h *Host) BaseURL() string {
	scheme := "http"
	if h.Secure {
		scheme = "https"
	}
	port := h.Port
	if port == 0 {
		port = 65333
	}
	return fmt.Sprintf("%s://%s:%d", scheme, h.Host, port)
}
