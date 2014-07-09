package freighter

import "fmt"

const defaultPort = 65343

// Host represents a Freighter host.
type Host struct {
	IP     string
	Port   int
	Secure bool
}

func NewHost(ip string, port int, secure bool) *Host {
	if port == 0 {
		port = defaultPort
	}
	return &Host{IP: ip, Port: port, Secure: secure}
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
	return fmt.Sprintf("%s://%s:%d", scheme, h.IP, port)
}
