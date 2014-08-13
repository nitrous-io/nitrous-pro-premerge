package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("container serializer", func() {
	var (
		touchedAt    = time.Date(2013, time.November, 2, 12, 0, 0, 0, time.UTC)
		terminatedAt = time.Date(2013, time.November, 3, 0, 0, 0, 0, time.UTC)
		createdAt    = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt    = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &container.Container{
			ID:           10,
			CID:          "deadbeef",
			UserID:       11,
			HostID:       12,
			Image:        "busybox",
			Slug:         "railsbox-10",
			Name:         sql.NullString{"Rails dev box", true},
			IPAddress:    "1.2.3.4",
			State:        "running",
			TouchedAt:    &touchedAt, // Not serialized.
			TerminatedAt: &terminatedAt,
			CreatedAt:    &createdAt,
			UpdatedAt:    &updatedAt,
		}
	)

	Describe("SerializeContainer", func() {
		It("serializes to JSON", func() {
			c := serializers.SerializeContainer(model)
			b, err := json.Marshal(c)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"cid": "deadbeef",
				"user_id": 11,
				"host_id": 12,
				"image": "busybox",
				"slug": "railsbox-10",
				"name": "Rails dev box",
				"ip_address": "1.2.3.4",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeContainers", func() {
		It("serializes to JSON", func() {
			c := serializers.SerializeContainers([]*container.Container{model})
			b, err := json.Marshal(c)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": 10,
				"cid": "deadbeef",
				"user_id": 11,
				"host_id": 12,
				"image": "busybox",
				"slug": "railsbox-10",
				"name": "Rails dev box",
				"ip_address": "1.2.3.4",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`))
		})
	})
})
