package server

import (
	"github.com/nitrous-io/nitrous/controllers/home"
	"github.com/nitrous-io/nitrous/controllers/tokens"
	"github.com/nitrous-io/nitrous/controllers/users"
)

func (s *Server) drawRoutes() {
	s.route("/", s.action(home.Index), "GET")
	s.route("/oauth/token", s.publicJSON(tokens.Post), "POST")

	s.namespace("/api/v0", func() {
		s.route("/users", s.action(users.Create), "POST")
		s.route("/user", s.authJSON(users.GetAuthenticatedUser), "GET")
	})

	s.static("/", "public") // catch-all, must come last
}
