package analytics

import (
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/models/user"
)

var Client Analytics = NewMixpanelAnalytics()

type Analytics interface {
	TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger)
	CreateProfile(u *user.User, logger *logger.Logger)
}
