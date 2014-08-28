package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/volume"
	"github.com/nitrous-io/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("volume serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &volume.Volume{
			ID:        10,
			HostID:    123,
			Slug:      "music",
			Name:      "music share",
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		}
	)

	Describe("SerializeVolume", func() {
		It("serializes to JSON", func() {
			v := serializers.SerializeVolume(model)
			b, err := json.Marshal(v)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
        "host_id": 123,
				"slug": "music",
				"name": "music share",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeVolumes", func() {
		It("serializes to JSON", func() {
			vols := serializers.SerializeVolumes([]*volume.Volume{model})
			b, err := json.Marshal(vols)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": 10,
        "host_id": 123,
				"slug": "music",
				"name": "music share",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`))
		})
	})

	Describe("SerializeFreighterVolume", func() {
		It("serializes to JSON", func() {
			v := serializers.SerializeFreighterVolume(
				&freighterclient.Volume{Slug: "music", Size: 123456})
			b, err := json.Marshal(v)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"slug": "music",
				"size": 123456
			}`))
		})
	})
})
