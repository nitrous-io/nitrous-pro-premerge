package main

import (
	"flag"
	"os"

	"bitbucket.org/kardianos/osext"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous-common/must"
	"github.com/nitrous-io/nitrous/server"
)

func main() {
	logr := &logger.Logger{Level: logger.LogLevelDebug}
	port := flag.Uint("p", 3000, "Port nitrousd will listen for requests")
	flag.Parse()

	defer func() {
		if r := recover(); r != nil {
			logr.FatalExit(r)
		}
	}()

	pid := os.Getpid()

	svr := server.NewServer(must.String(osext.ExecutableFolder()), logr)
	host := "0.0.0.0"
	logr.Infof("nitrousd (pid: %v) listening on %s:%d", pid, host, *port)
	svr.Start(host, uint16(*port))
}
