package serializers

import (
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/host"
)

type Host struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	Slug           string `json:"slug"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int64  `json:"freighterd_port"`
	Hostname       string `json:"hostname"` // where we can access the host i.e. slug.nitrouspro.com
	State          string `json:"state"`

	Stats *HostInfo `json:"stats,omitempty"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
}

func SerializeHost(h *host.Host, info *freighterclient.HostInfo) *Host {
	if h == nil {
		return nil
	}
	ret := &Host{
		ID:             h.ID,
		UserID:         h.UserID,
		Slug:           h.Slug,
		IPAddress:      h.IPAddress.String,
		FreighterdPort: h.FreighterdPort.Int64,
		State:          h.State,
		Hostname:       h.Hostname(),
		CreatedAt:      h.CreatedAt,
		UpdatedAt:      h.UpdatedAt,
		TerminatedAt:   h.TerminatedAt,
	}
	if info != nil {
		ret.Stats = SerializeHostInfo(info)
	}
	return ret
}
