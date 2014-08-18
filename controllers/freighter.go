package controllers

import (
	"net/http"

	fc "github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	// ErrHostIPUnknown results when the host's ip address is empty, hence not
	// usable by the client
	ErrHostIPUnknown = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	// ErrHostNotRunning results when we tries to access a not-running host
	ErrHostNotRunning = httperror.New(http.StatusServiceUnavailable, "service_unavailable", "host is not running")
)

// FreighterClient embeds freighterclient.Client, but also expose the used
// certificate for HTTPS connection
type FreighterClient struct {
	fc.Client
	Cert *cert.Cert
}

// NewFreighterClient returns a client that is able to communicate with a
// freighter server using HTTPS
var NewFreighterClient = func(db db.QueryReceiver, h *host.Host) (*FreighterClient, error) {
	if h.IPAddress.String == "" {
		return nil, ErrHostIPUnknown
	}
	if h.State != host.StateRunning {
		return nil, ErrHostNotRunning
	}
	crt, err := cert.Find(db, h.ID)
	if err != nil {
		return nil, err
	}
	hst := fc.NewHost(
		h.IPAddress.String,
		int(h.FreighterdPort.Int64),
		h.Hostname(),
		[]byte(crt.CA),
	)
	cl, err := fc.NewClient(hst)
	if err != nil {
		return nil, err
	}
	return &FreighterClient{
		Client: cl,
		Cert:   crt,
	}, nil
}
