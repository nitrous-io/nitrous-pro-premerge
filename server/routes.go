package server

import (
	"github.com/nitrous-io/nitrous/controllers/home"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/controllers/tokens"
	"github.com/nitrous-io/nitrous/controllers/users"
)

func (s *Server) drawRoutes() {
	s.route("/", s.action(home.Index), "GET")
	s.route("/oauth/token", s.action(tokens.Post), "POST")

	s.namespace("/api/v0", func() {
		s.route("/users", s.action(users.Create), "POST")
		s.route("/user", s.authedAction(users.GetAuthenticatedUser), "GET")

		s.route("/hosts", s.authedAction(hosts.Index), "GET")
		s.route("/hosts", s.authedAction(hosts.Create), "POST")
		s.route("/hosts", s.authedAction(hosts.HandleAction), "DELETE")
		s.route("/hosts/start", s.authedAction(hosts.HandleAction), "POST")
		s.route("/hosts/stop", s.authedAction(hosts.HandleAction), "POST")
		s.route("/hosts/restart", s.authedAction(hosts.HandleAction), "POST")
	})

	s.static("/", "public") // catch-all, must come last
}
