package serializers

import (
	"time"

	"github.com/nitrous-io/nitrous/models/container"
)

type Container struct {
	ID        int64  `json:"id"`
	CID       string `json:"cid"`
	UserID    int64  `json:"user_id"`
	HostID    int64  `json:"host_id"`
	Image     string `json:"image"`
	Slug      string `json:"slug"`
	Name      string `json:"name,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	State     string `json:"state"`
	HostSlug  string `json:"host_slug,omitempty"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
}

func SerializeContainer(c *container.Container) *Container {
	return &Container{
		ID:           c.ID,
		CID:          c.CID,
		UserID:       c.UserID,
		HostID:       c.HostID,
		Image:        c.Image,
		Slug:         c.Slug,
		Name:         c.Name.String,
		HostSlug:     c.HostSlug,
		IPAddress:    c.IPAddress,
		State:        c.State,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		TerminatedAt: c.TerminatedAt,
	}
}
