package test

import (
	"github.com/nitrous-io/nitrous/models/user"

	"github.com/nitrous-io/nitrous-common/logger"
)

type FakeAnalytics struct {
	Event                string
	TrackEventProperties map[string]interface{}
	User                 *user.User
	TrackEventCalled     bool
	CreateProfileCalled  bool
}

func NewFakeAnalytics() *FakeAnalytics {
	return &FakeAnalytics{}
}

func (f *FakeAnalytics) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	f.TrackEventCalled = true
	f.Event = event
	f.User = u

	f.TrackEventProperties = p
}

func (f *FakeAnalytics) CreateProfile(u *user.User, logger *logger.Logger) {
	f.CreateProfileCalled = true
	f.User = u
}
