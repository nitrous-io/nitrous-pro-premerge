package test

import (
	"github.com/nitrous-io/nitrous/models/user"

	"github.com/nitrous-io/nitrous-common/logger"
)

type FakeAnalytics struct {
	Events               []string
	TrackEventProperties []map[string]interface{}
	User                 *user.User
	TrackEventCalled     int
	CreateProfileCalled  int
}

func NewFakeAnalytics() *FakeAnalytics {
	return &FakeAnalytics{}
}

func (f *FakeAnalytics) TrackEvent(event string, u *user.User, p map[string]interface{}, logger *logger.Logger) {
	f.TrackEventCalled += 1
	f.Events = append(f.Events, event)
	f.User = u

	f.TrackEventProperties = append(f.TrackEventProperties, p)
}

func (f *FakeAnalytics) CreateProfile(u *user.User, logger *logger.Logger) {
	f.CreateProfileCalled += 1
	f.User = u
}
