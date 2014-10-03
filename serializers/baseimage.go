package serializers

import (
	"time"

	"github.com/nitrous-io/nitrous/models/baseimage"
)

type BaseImage struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug,omitempty"`
	Repo        string     `json:"repo"`
	Description string     `json:"description"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

func SerializeBaseImage(img *baseimage.BaseImage) *BaseImage {
	if img == nil {
		return nil
	}
	return &BaseImage{
		ID:          img.ID,
		Name:        img.Name,
		Slug:        img.Slug,
		Repo:        img.Repo,
		Description: img.Description,
		CreatedAt:   img.CreatedAt,
		UpdatedAt:   img.UpdatedAt,
	}
}

func SerializeBaseImages(imgs []*baseimage.BaseImage) []*BaseImage {
	ret := make([]*BaseImage, len(imgs))
	for i, img := range imgs {
		ret[i] = SerializeBaseImage(img)
	}
	return ret
}
