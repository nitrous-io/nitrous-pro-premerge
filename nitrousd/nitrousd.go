package main

import (
	"os"

	"bitbucket.org/kardianos/osext"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous-common/must"
	"github.com/nitrous-io/nitrous/config"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/server"
)

const DBEnvVar = "NITROUS_PG_URL"
const HostbuilderURLEnvVar = "HOSTBUILDER_URL"

func main() {
	logr := &logger.Logger{Level: logger.LogLevelDebug}

	defer func() {
		if r := recover(); r != nil {
			logr.FatalExit(r)
		}
	}()

	pid := os.Getpid()

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

	hbURL := os.Getenv(HostbuilderURLEnvVar)
	if hbURL == "" {
		logr.FatalExit(HostbuilderURLEnvVar, "is not defined")
	}
	env := &config.Env{HostbuilderURL: hbURL}

	svr := server.NewServer(must.String(osext.ExecutableFolder()), dbConn, logr, env)
	host := "0.0.0.0"
	port := uint16(3000)
	logr.Infof("nitrousd (pid: %v) listening on %s:%d", pid, host, port)
	svr.Start(host, port)
}
