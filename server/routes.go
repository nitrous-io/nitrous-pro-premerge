package server

import (
	"github.com/nitrous-io/nitrous/controllers/home"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/controllers/keys"
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
		s.route("/hosts/{name}", mw.Auth(mw.Vars(hosts.Destroy)), "DELETE")
		s.route("/hosts/{name}/start", mw.Auth(mw.Vars(hosts.Start)), "POST")
		s.route("/hosts/{name}/stop", mw.Auth(mw.Vars(hosts.Stop)), "POST")
		s.route("/hosts/{name}/restart", mw.Auth(mw.Vars(hosts.Restart)), "POST")

		s.route("/keys", mw.Auth(mw.JSON(keys.Index)), "GET")
		s.route("/keys", mw.Auth(mw.JSON(keys.Create)), "POST")
		s.route("/keys", mw.Auth(mw.JSON(keys.Create)), "PUT")
		s.route("/keys/{md5_fingerprint}", mw.Auth(mw.JSON(keys.DeleteByFingerprint)), "DELETE")
		s.route("/keys", mw.Auth(mw.JSON(keys.DeleteAll)), "DELETE")
	})

	s.static("/", "public") // catch-all, must come last
}
