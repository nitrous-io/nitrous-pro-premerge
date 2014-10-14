package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/nitrous-io/go-mixpanel"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
)

const DBEnvVar = "NITROUS_PG_URL"
const MixpanelEnvVar = "MIXPANEL_TOKEN"

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

func main() {
	dbURL := os.Getenv(DBEnvVar)
	if dbURL == "" {
		logr.FatalExit(DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}

	mixpanelToken := os.Getenv(MixpanelEnvVar)
	if mixpanelToken == "" || strings.HasPrefix(mixpanelToken, "disable") {
		logr.FatalExit(MixpanelEnvVar, "is not defined")
	}
	mc := mixpanel.NewMixpanelClient(mixpanelToken)

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
		p := map[string]interface{}{
			"$email":    u.Email,
			"$name":     u.Name,
			"$username": u.Username,
			"$created":  u.CreatedAt,
			"user_id":   u.ID,
		}
		err := mc.CreateProfile(strconv.Itoa(int(u.ID)), p)
		if err != nil {
			logr.FatalExit(err)
		}
	}
}
