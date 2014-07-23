package server

import (
	"github.com/nitrous-io/nitrous/controllers/containers"
	"github.com/nitrous-io/nitrous/controllers/home"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/controllers/tokens"
	"github.com/nitrous-io/nitrous/controllers/users"
	mw "github.com/nitrous-io/nitrous/middlewares"
)

func (s *Server) drawRoutes() {
	s.route("/", home.Index, "GET")
	s.route("/oauth/token", mw.JSON(tokens.Post), "POST")

	s.namespace("/api/v0", func() {
		s.route("/users", mw.JSON(users.Create), "POST")
		s.route("/user", mw.Auth(mw.JSON(users.GetAuthenticatedUser)), "GET")

		s.route("/hosts", mw.Auth(mw.JSON(hosts.Index)), "GET")
		s.route("/hosts", mw.Auth(mw.JSON(hosts.Create)), "POST")
		s.route("/hosts/{slug}", mw.Auth(mw.Vars(hosts.Destroy)), "DELETE")
		s.route("/hosts/{slug}/start", mw.Auth(mw.Vars(hosts.Start)), "POST")
		s.route("/hosts/{slug}/stop", mw.Auth(mw.Vars(hosts.Stop)), "POST")
		s.route("/hosts/{slug}/restart", mw.Auth(mw.Vars(hosts.Restart)), "POST")

		s.route("/containers", mw.Auth(mw.JSON(containers.Index)), "GET")
		s.route("/containers", mw.Auth(mw.JSON(containers.Create)), "POST")
		s.route("/containers/{slug}", mw.Auth(mw.Vars(mw.JSON(containers.Show))), "GET")
		s.route("/containers/{slug}/start", mw.Auth(mw.Vars(mw.JSON(containers.Start))), "POST")
		s.route("/containers/{slug}/stop", mw.Auth(mw.Vars(mw.JSON(containers.Stop))), "POST")
		s.route("/containers/{slug}/restart", mw.Auth(mw.Vars(mw.JSON(containers.Restart))), "POST")
		s.route("/containers/{slug}", mw.Auth(mw.Vars(mw.JSON(containers.Destroy))), "DELETE")
	})

	s.static("/", "public") // catch-all, must come last
}
