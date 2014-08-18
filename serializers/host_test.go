package serializers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("host serializer", func() {
	Describe("SerializeHost", func() {
		var (
			terminatedAt = time.Date(2013, time.November, 3, 0, 0, 0, 0, time.UTC)
			createdAt    = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
			updatedAt    = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

			model = &host.Host{
				ID:                 10,
				UserID:             11,
				RegionID:           12,
				PlanID:             13,
				ProviderTemplateID: 14, // Not serialized.
				Slug:               "wild-rocket-23",
				Name:               sql.NullString{"wildrocket.xxx", true},
				State:              "running",
				IPAddress:          sql.NullString{"1.2.3.4", true},
				FreighterdPort:     sql.NullInt64{65000, true},
				ProviderIdentifier: sql.NullString{"digitalocean", true},
				MaxContainers:      sql.NullInt64{3, true},
				Size:               sql.NullString{"micro", true},
				TerminatedAt:       &terminatedAt,
				CreatedAt:          &createdAt,
				UpdatedAt:          &updatedAt,
			}
		)

		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeHost(model, nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"region_id": 12,
				"plan_id": 13,
				"slug": "wild-rocket-23",
				"ip_address": "1.2.3.4",
				"freighterd_port": 65000,
				"hostname": "%s",
				"size": "micro",
				"state": "running",
				"terminated_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`, model.Hostname())))
		})

		It("serializes to null JSON when host is nil", func() {
			b, err := json.Marshal(serializers.SerializeHost(nil, nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`null`))
		})

		It("includes cert CA data when cert is not nil", func() {
			crt := &cert.Cert{
				CA: "im a ca",
			}
			res := serializers.SerializeHost(model, crt)
			Expect(res.CA).To(Equal(crt.CA))
		})
	})
})
