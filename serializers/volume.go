package serializers

import (
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/volume"
)

type Volume struct {
	ID        int64      `json:"id,omitempty"`
	HostID    int64      `json:"host_id"`
	Name      string     `json:"name,omitempty"`
	Slug      string     `json:"slug"`
	Size      uint64     `json:"size,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func SerializeVolume(v *volume.Volume) *Volume {
	if v == nil {
		return nil
	}
	return &Volume{
		ID:        v.ID,
		HostID:    v.HostID,
		Name:      v.Name,
		Slug:      v.Slug,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
}

func SerializeVolumes(vols []*volume.Volume) []*Volume {
	ret := make([]*Volume, len(vols))
	for i, v := range vols {
		ret[i] = SerializeVolume(v)
	}
	return ret
}

func SerializeFreighterVolume(v *freighterclient.Volume) *Volume {
	if v == nil {
		return nil
	}
	return &Volume{
		Slug: v.Slug,
		Size: v.Size,
	}
}
