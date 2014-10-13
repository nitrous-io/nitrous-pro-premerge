package main

import (
	"os"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/analytics"
)

const DBEnvVar = "NITROUS_PG_URL"

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

func main() {
	a := analytics.NewMixpanelAnalytics()

	dbURL := os.Getenv(DBEnvVar)
	if dbURL == "" {
		logr.FatalExit(DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}
	err = dbConn.MigrateAll()
	if err != nil {
		logr.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	users := []*user.User{}
	err = dbConn.Select(&users, "SELECT * FROM users;")
	if err != nil {
		logr.FatalExit(err)
	}

	for _, u := range users {
		logr.Infof("creating mixpanel profile for %s", u.Email)
		a.CreateProfile(u, logr)
	}
}
