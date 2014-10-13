package analytics

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/nitrous-io/go-mixpanel"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/models/user"
)

const (
	UserSignedUp     = "User Signed Up"
	UserSignedIn     = "User Signed In"
	HostCreated      = "Host Created"
	ContainerCreated = "Container Created"
)

var (
	mixpanelToken string

	disabled = false
)

func init() {
	mixpanelToken = os.Getenv("MIXPANEL_TOKEN")
	if mixpanelToken == "" || strings.HasPrefix(mixpanelToken, "disable") {
		log.Println("Info: Mixpanel token is not set. Analytics events will not be tracked.")
		disabled = true
	}
}

type MixpanelAnalytics struct {
	client *mixpanel.Mixpanel
}

func NewMixpanelAnalytics() *MixpanelAnalytics {
	var mc *mixpanel.Mixpanel = nil

	if !disabled {
		mc = mixpanel.NewMixpanelClient(mixpanelToken)
	}

	return &MixpanelAnalytics{client: mc}
}

func (m *MixpanelAnalytics) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	if disabled {
		return
	}

	if p == nil {
		p = map[string]interface{}{}
	}

	p["distinct_id"] = strconv.Itoa(int(u.ID))
	p["email"] = u.Email

	go func() {
		err := m.client.Track(event, p)
		if err != nil {
			logger.Errorf("analytics: failed to track event: %s. err: %v", event, err)
		}
	}()
}

func (m *MixpanelAnalytics) CreateProfile(u *user.User, logger *logger.Logger) {
	if disabled {
		return
	}

	p := map[string]interface{}{
		"$email":    u.Email,
		"$name":     u.Name,
		"$username": u.Username,
		"$created":  u.CreatedAt,
		"user_id":   u.ID,
	}

	go func() {
		err := m.client.CreateProfile(strconv.Itoa(int(u.ID)), p)
		if err != nil {
			logger.Errorf("analytics: failed to create user profile: %s. err: %v", u.Email, err)
		}
	}()
}
