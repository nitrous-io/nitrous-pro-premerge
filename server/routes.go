package server

import (
	"github.com/nitrous-io/nitrous/controllers/containers"
	"github.com/nitrous-io/nitrous/controllers/events"
	"github.com/nitrous-io/nitrous/controllers/home"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/controllers/images"
	"github.com/nitrous-io/nitrous/controllers/keys"
	"github.com/nitrous-io/nitrous/controllers/registries"
	"github.com/nitrous-io/nitrous/controllers/tokens"
	"github.com/nitrous-io/nitrous/controllers/users"
	"github.com/nitrous-io/nitrous/controllers/volumes"
	mw "github.com/nitrous-io/nitrous/middleware"
)

func (s *Server) drawRoutes() {
	s.route("/", home.Index, "GET")
	s.route("/oauth/token", mw.JSON(tokens.Post), "POST")
	s.route("/events", mw.Auth(events.Stream), "GET")

	s.namespace("/api/v0", func() {
		s.route("/users", mw.JSON(users.Create), "POST")
		s.route("/user", mw.Auth(mw.JSON(users.GetAuthenticatedUser)), "GET")

		s.route("/hosts", mw.Auth(mw.JSON(hosts.Index)), "GET")
		s.route("/hosts", mw.Auth(mw.JSON(hosts.Create)), "POST")
		s.route("/hosts/{slug}", mw.Auth(mw.JSON(mw.HostViaPath("slug", hosts.Show))), "GET")
		s.route("/hosts/{slug}", mw.Auth(mw.JSON(mw.HostViaPath("slug", hosts.Destroy))), "DELETE")
		s.route("/hosts/{slug}/start", mw.Auth(mw.HostViaPath("slug", hosts.Start)), "POST")
		s.route("/hosts/{slug}/stop", mw.Auth(mw.HostViaPath("slug", hosts.Stop)), "POST")
		s.route("/hosts/{slug}/restart", mw.Auth(mw.HostViaPath("slug", hosts.Restart)), "POST")

		s.route("/hosts/{slug}/registries", mw.Auth(mw.JSON(mw.Vars(registries.Index))), "GET")
		s.route("/hosts/{slug}/registries", mw.Auth(mw.Vars(registries.Login)), "POST")
		s.route("/hosts/{slug}/registries", mw.Auth(mw.Vars(registries.Logout)), "DELETE")

		s.route("/hosts/{slug}/volumes", mw.Auth(mw.JSON(mw.Vars(mw.HostViaPath("slug", volumes.Index)))), "GET")
		s.route("/hosts/{slug}/volumes", mw.Auth(mw.JSON(mw.Vars(mw.HostViaPath("slug", volumes.Create)))), "POST")
		s.route("/hosts/{slug}/volumes/{vslug}", mw.Auth(mw.JSON(mw.Vars(mw.HostViaPath("slug", volumes.Destroy)))), "DELETE")
		s.route("/hosts/{slug}/volumes/{vslug}", mw.Auth(mw.JSON(mw.Vars(mw.HostViaPath("slug", volumes.Show)))), "GET")

		s.route("/hosts/{slug}/images", mw.Auth(mw.Vars(mw.JSON(mw.HostViaPath("slug", images.Index)))), "GET")
		s.route("/hosts/{slug}/images", mw.Auth(mw.Vars(mw.JSON(mw.HostViaPath("slug", images.Build)))), "POST")
		s.route("/hosts/{slug}/images/pull", mw.Auth(mw.JSON(mw.HostViaPath("slug", images.Pull))), "POST")
		s.route("/hosts/{slug}/images/{image}/push", mw.Auth(mw.JSON(mw.HostViaPath("slug", images.Push))), "POST")
		s.route("/hosts/{slug}/images", mw.Auth(mw.JSON(mw.HostViaPath("slug", images.Remove))), "DELETE")
		s.route("/hosts/{slug}/images/{image}/tag", mw.Auth(mw.JSON(mw.HostViaPath("slug", images.Tag))), "POST")

		s.route("/containers", mw.Auth(mw.JSON(containers.Index)), "GET")
		s.route("/containers", mw.Auth(mw.JSON(containers.Create)), "POST")
		s.route("/containers/{slug}", mw.Auth(mw.JSON(containers.Show)), "GET")
		s.route("/containers/{slug}/start", mw.Auth(mw.JSON(containers.Start)), "POST")
		s.route("/containers/{slug}/stop", mw.Auth(mw.JSON(containers.Stop)), "POST")
		s.route("/containers/{slug}/restart", mw.Auth(mw.JSON(containers.Restart)), "POST")
		s.route("/containers/{slug}", mw.Auth(mw.JSON(containers.Destroy)), "DELETE")

		s.route("/containers/{slug}/keys", mw.Auth(mw.JSON(keys.Index)), "GET")
		s.route("/containers/{slug}/keys", mw.Auth(mw.JSON(keys.Create)), "POST")
		s.route("/containers/{slug}/keys", mw.Auth(mw.JSON(keys.Create)), "PUT")
		s.route("/containers/{slug}/keys/{md5_fingerprint}", mw.Auth(mw.JSON(keys.DeleteByFingerprint)), "DELETE")
		s.route("/containers/{slug}/keys", mw.Auth(mw.JSON(keys.DeleteAll)), "DELETE")
	})

	s.static("/", "public") // catch-all, must come last
}
