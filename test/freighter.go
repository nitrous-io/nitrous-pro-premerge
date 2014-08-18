package test

import (
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
)

type freighterClientFn func(db db.QueryReceiver, h *host.Host) (*controllers.FreighterClient, error)

var actualClient = controllers.NewFreighterClient

// SetDefaultFreighterFn assumes that we only want to inject the client, and we
// have a valid host with proper cert.
func StubDefaultFreighterFn(cl freighterclient.Client) {
	fn := func(db db.QueryReceiver, h *host.Host) (*controllers.FreighterClient, error) {
		crt, err := cert.Find(db, h.ID)
		if err != nil {
			return nil, err
		}
		return &controllers.FreighterClient{
			Client: cl,
			Cert:   crt,
		}, nil
	}
	StubFreighterFn(fn)
}

func StubFreighterFn(fn freighterClientFn) {
	controllers.NewFreighterClient = fn
}

func UnstubFreighterFn() {
	controllers.NewFreighterClient = actualClient
}
