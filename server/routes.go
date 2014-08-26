package server

import (
	"github.com/nitrous-io/nitrous/controllers/containers"
	"github.com/nitrous-io/nitrous/controllers/events"
	"github.com/nitrous-io/nitrous/controllers/home"
	"github.com/nitrous-io/nitrous/controllers/hosts"
	"github.com/nitrous-io/nitrous/controllers/images"
	"github.com/nitrous-io/nitrous/controllers/keys"
	"github.com/nitrous-io/nitrous/controllers/plans"
	"github.com/nitrous-io/nitrous/controllers/regions"
	"github.com/nitrous-io/nitrous/controllers/registries"
	"github.com/nitrous-io/nitrous/controllers/tokens"
	"github.com/nitrous-io/nitrous/controllers/users"
	"github.com/nitrous-io/nitrous/controllers/volumes"
	mw "github.com/nitrous-io/nitrous/middleware"
)

func (s *Server) drawRoutes() {
	s.route("/", home.Index, "GET")
	s.route("/events", mw.Auth(events.Stream), "GET")

	s.namespace("/api/v0", func() {
		s.routeJSON("/oauth/token", tokens.Post, "POST")
		s.routeJSON("/users", users.Create, "POST")
		s.routeJSON("/users", mw.Auth(users.Update), "PUT")
		s.routeJSON("/user", mw.Auth(users.GetAuthenticatedUser), "GET")

		s.routeJSON("/hosts", mw.Auth(hosts.Index), "GET")
		s.routeJSON("/hosts", mw.Auth(hosts.Create), "POST")
		s.routeJSON("/hosts/{slug}", mw.Auth(mw.HostViaPath("slug", hosts.Show)), "GET")
		s.routeJSON("/hosts/{slug}/stats", mw.Auth(mw.HostViaPath("slug", hosts.Stats)), "GET")
		s.routeJSON("/hosts/{slug}", mw.Auth(mw.HostViaPath("slug", hosts.Update)), "PUT")
		s.routeJSON("/hosts/{slug}", mw.Auth(mw.HostViaPath("slug", hosts.Destroy)), "DELETE")
		s.routeJSON("/hosts/{slug}/start", mw.Auth(mw.HostViaPath("slug", hosts.Start)), "POST")
		s.routeJSON("/hosts/{slug}/stop", mw.Auth(mw.HostViaPath("slug", hosts.Stop)), "POST")
		s.routeJSON("/hosts/{slug}/restart", mw.Auth(mw.HostViaPath("slug", hosts.Restart)), "POST")

		s.routeJSON("/hosts/{slug}/registries", mw.Auth(registries.Index), "GET")
		s.routeJSON("/hosts/{slug}/registries", mw.Auth(registries.Login), "POST")
		s.routeJSON("/hosts/{slug}/registries", mw.Auth(registries.Logout), "DELETE")

		s.routeJSON("/hosts/{slug}/volumes", mw.Auth(mw.HostViaPath("slug", volumes.Index)), "GET")
		s.routeJSON("/hosts/{slug}/volumes", mw.Auth(mw.HostViaPath("slug", volumes.Create)), "POST")
		s.routeJSON("/hosts/{slug}/volumes/{vslug}", mw.Auth(mw.HostViaPath("slug", volumes.Destroy)), "DELETE")
		s.routeJSON("/hosts/{slug}/volumes/{vslug}", mw.Auth(mw.HostViaPath("slug", volumes.Show)), "GET")

		s.routeJSON("/hosts/{slug}/images", mw.Auth(mw.HostViaPath("slug", images.Index)), "GET")
		s.routeJSON("/hosts/{slug}/images", mw.Auth(mw.HostViaPath("slug", images.Build)), "POST")
		// pull is not a JSON endpoint. It's just a proxy which streams back
		// docker's json response
		s.route("/hosts/{slug}/images/pull", mw.Auth(mw.HostViaPath("slug", images.Pull)), "POST")
		// Test thoroughly before adding a POST /hosts/{slug}/images/{image:.*}
		// endpoint because that would probably match the push and tag routes.
		s.routeJSON("/hosts/{slug}/images/{image:.*}/push", mw.Auth(mw.HostViaPath("slug", images.Push)), "POST")
		s.routeJSON("/hosts/{slug}/images/{image:.*}", mw.Auth(mw.HostViaPath("slug", images.Remove)), "DELETE")
		s.routeJSON("/hosts/{slug}/images/{image:.*}/tag", mw.Auth(mw.HostViaPath("slug", images.Tag)), "POST")

		s.routeJSON("/containers", mw.Auth(containers.Index), "GET")
		s.routeJSON("/containers", mw.Auth(containers.Create), "POST")
		s.routeJSON("/containers/{slug}", mw.Auth(containers.Show), "GET")
		s.routeJSON("/containers/{slug}/commit", mw.Auth(containers.Commit), "POST")
		s.routeJSON("/containers/{slug}/start", mw.Auth(containers.Start), "POST")
		s.routeJSON("/containers/{slug}/stop", mw.Auth(containers.Stop), "POST")
		s.routeJSON("/containers/{slug}/restart", mw.Auth(containers.Restart), "POST")
		s.routeJSON("/containers/{slug}", mw.Auth(containers.Destroy), "DELETE")
		s.routeJSON("/containers/{slug}/token", mw.Auth(containers.Token), "POST")

		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.Index), "GET")
		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.Create), "POST")
		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.Create), "PUT")
		s.routeJSON("/containers/{slug}/keys/{md5_fingerprint}", mw.Auth(keys.DeleteByFingerprint), "DELETE")
		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.DeleteAll), "DELETE")

		s.routeJSON("/plans", plans.Index, "GET")

		s.routeJSON("/regions", regions.Index, "GET")
	})

	s.static("/", "public") // catch-all, must come last
}
