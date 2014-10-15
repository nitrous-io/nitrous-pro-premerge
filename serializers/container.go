package serializers

import (
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/container"
)

type Container struct {
	ID         int64    `json:"id"`
	CID        string   `json:"cid"`
	UserID     int64    `json:"user_id"`
	HostID     int64    `json:"host_id"`
	Image      string   `json:"image"`
	Slug       string   `json:"slug"`
	Name       string   `json:"name,omitempty"`
	IPAddress  string   `json:"ip_address,omitempty"`
	GatewayIP  string   `json:"gateway_ip,omitempty"`
	State      string   `json:"state"`
	Cmd        []string `json:"cmd,omitempty"`
	Entrypoint []string `json:"entrypoint,omitempty"`
	ExitCode   int      `json:"exit_code"`

	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	TerminatedAt *time.Time `json:"terminated_at,omitempty"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
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
		IPAddress:    c.IPAddress,
		GatewayIP:    c.GatewayIP,
		State:        c.ResolvedState().String(),
		Cmd:          c.Cmd,
		Entrypoint:   c.Entrypoint,
		ExitCode:     c.ExitCode,
		StartedAt:    c.StartedAt,
		FinishedAt:   c.FinishedAt,
		CreatedAt:    c.CreatedAt,
		UpdatedAt:    c.UpdatedAt,
		TerminatedAt: c.TerminatedAt,
	}
}

type ContainerConfig struct {
	Cmd        []string `json:"cmd"`
	Env        []string `json:"env"`
	Ports      []string `json:"port"`
	Mounts     []string `json:"volume"`
	PublishAll bool     `json:"publish_all"`
}

func SerializeContainerConfig(cfg *freighterclient.ContainerConfig) *ContainerConfig {
	return &ContainerConfig{
		Cmd:        cfg.Cmd,
		Env:        cfg.Env,
		Ports:      cfg.PortBindings,
		Mounts:     cfg.Mounts,
		PublishAll: cfg.PublishAll,
	}
}
