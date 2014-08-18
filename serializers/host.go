package serializers

import (
	"time"

	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
)

type Host struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	RegionID       int64  `json:"region_id"`
	PlanID         int64  `json:"plan_id"`
	Slug           string `json:"slug"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int64  `json:"freighterd_port"`
	Hostname       string `json:"hostname"` // where we can access the host i.e. slug.nitrouspro.com
	Size           string `json:"size"`
	State          string `json:"state"`
	CA             string `json:"ca,omitempty"` // root CA for client to connect to the host

	Stats *HostInfo `json:"stats,omitempty"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
}

func SerializeHost(h *host.Host, crt *cert.Cert) *Host {
	if h == nil {
		return nil
	}
	res := &Host{
		ID:             h.ID,
		UserID:         h.UserID,
		RegionID:       h.RegionID,
		PlanID:         h.PlanID,
		Slug:           h.Slug,
		IPAddress:      h.IPAddress.String,
		FreighterdPort: h.FreighterdPort.Int64,
		Size:           h.Size.String,
		State:          h.State,
		Hostname:       h.Hostname(),
		CreatedAt:      h.CreatedAt,
		UpdatedAt:      h.UpdatedAt,
		TerminatedAt:   h.TerminatedAt,
	}
	if crt != nil {
		res.CA = crt.CA
	}
	return res
}
