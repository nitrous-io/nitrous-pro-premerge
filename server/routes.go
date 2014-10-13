package server

import (
	"net/http/pprof"
	"os"

	"github.com/nitrous-io/nitrous/controllers/baseimages"
	"github.com/nitrous-io/nitrous/controllers/containers"
	"github.com/nitrous-io/nitrous/controllers/events"
	"github.com/nitrous-io/nitrous/controllers/feedback"
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
	"github.com/nitrous-io/nitrous/pkg/analytics"
)

func (s *Server) drawRoutes() {
	s.route("/", home.Index, "GET")

	if os.Getenv("DEBUG") == "true" {
		s.router.HandleFunc("/debug/pprof/{path:.*}", pprof.Index).Methods("GET")
		s.routeJSON("/debug", s.debugHandler, "GET")
	}

	s.namespace("/api/v0", func() {
		s.route("/events", mw.AuthQS(events.Stream), "GET")

		s.routeJSON("/oauth/token", mw.Track(analytics.UserSignedIn, tokens.Post), "POST")
		s.routeJSON("/users", mw.Track(analytics.UserSignedUp, users.Create), "POST")
		s.routeJSON("/users", mw.Auth(users.Update), "PUT")
		s.routeJSON("/user", mw.Auth(users.GetAuthenticatedUser), "GET")

		s.routeJSON("/hosts", mw.Auth(hosts.Index), "GET")
		s.routeJSON("/hosts", mw.Auth(mw.Track(analytics.HostCreated, hosts.Create)), "POST")
		s.routeJSON("/hosts/{slug}", mw.Auth(mw.HostViaPath("slug", hosts.Show)), "GET")
		s.routeJSON("/hosts/{slug}/stats", mw.Auth(mw.HostViaPath("slug", hosts.Stats)), "GET")
		s.routeJSON("/hosts/{slug}", mw.Auth(mw.HostViaPath("slug", hosts.Update)), "PUT")
		s.routeJSON("/hosts/{slug}", mw.Auth(mw.HostViaPath("slug", hosts.Destroy)), "DELETE")
		s.routeJSON("/hosts/{slug}/start", mw.Auth(mw.HostViaPath("slug", hosts.Start)), "POST")
		s.routeJSON("/hosts/{slug}/stop", mw.Auth(mw.HostViaPath("slug", hosts.Stop)), "POST")
		s.routeJSON("/hosts/{slug}/restart", mw.Auth(mw.HostViaPath("slug", hosts.Restart)), "POST")

		s.routeJSON("/hosts/{slug}/registries", mw.Auth(registries.Index), "GET")
		s.routeJSON("/hosts/{slug}/registries", mw.Auth(registries.Login), "POST")
		s.routeJSON("/hosts/{slug}/registries/{rslug}", mw.Auth(registries.Logout), "DELETE")

		s.routeJSON("/hosts/{slug}/volumes", mw.Auth(mw.HostViaPath("slug", volumes.Index)), "GET")
		s.routeJSON("/hosts/{slug}/volumes", mw.Auth(mw.HostViaPath("slug", volumes.Create)), "POST")
		s.routeJSON("/hosts/{slug}/volumes/{vslug}", mw.Auth(mw.HostViaPath("slug", volumes.Destroy)), "DELETE")
		s.routeJSON("/hosts/{slug}/volumes/{vslug}", mw.Auth(mw.HostViaPath("slug", volumes.Show)), "GET")
		s.routeJSON("/hosts/{slug}/volumes/{vslug}/info", mw.Auth(mw.HostViaPath("slug", volumes.Info)), "GET")

		s.routeJSON("/images", mw.Auth(images.Index), "GET")
		s.routeJSON("/hosts/{slug}/images", mw.Auth(images.Index), "GET")
		s.routeJSON("/hosts/{slug}/images", mw.Auth(mw.HostViaPath("slug", images.Build)), "POST")
		s.routeJSON("/hosts/{slug}/images/pull", mw.Auth(mw.HostViaPath("slug", images.Pull)), "POST")
		// Test thoroughly before adding a POST /hosts/{slug}/images/{image:.*}
		// endpoint because that would probably match the push and tag routes.
		s.routeJSON("/hosts/{slug}/images/{image:.*}/push", mw.Auth(mw.HostViaPath("slug", images.Push)), "POST")
		s.routeJSON("/hosts/{slug}/images/{image:.*}/tag", mw.Auth(mw.HostViaPath("slug", images.Tag)), "POST")
		s.routeJSON("/hosts/{slug}/images/{image:.*}", mw.Auth(mw.HostViaPath("slug", images.Remove)), "DELETE")

		s.routeJSON("/containers", mw.Auth(containers.Index), "GET")
		s.routeJSON("/containers", mw.Track(analytics.ContainerCreated, mw.Auth(containers.Create)), "POST")
		s.routeJSON("/containers/{slug}", mw.Auth(containers.Show), "GET")
		s.routeJSON("/containers/{slug}/commit", mw.Auth(containers.Commit), "POST")
		s.routeJSON("/containers/{slug}/config", mw.Auth(containers.Config), "GET")
		s.routeJSON("/containers/{slug}/start", mw.Auth(containers.Start), "POST")
		s.routeJSON("/containers/{slug}/stop", mw.Auth(containers.Stop), "POST")
		s.routeJSON("/containers/{slug}/restart", mw.Auth(containers.Restart), "POST")
		s.routeJSON("/containers/{slug}/reconfig", mw.Auth(containers.Reconfig), "POST")
		s.routeJSON("/containers/{slug}", mw.Auth(containers.Destroy), "DELETE")
		s.routeJSON("/containers/{slug}/token", mw.Auth(containers.Token), "POST")

		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.Index), "GET")
		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.Create), "POST")
		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.Create), "PUT")
		s.routeJSON("/containers/{slug}/keys/{kslug}", mw.Auth(keys.Delete), "DELETE")
		s.routeJSON("/containers/{slug}/keys", mw.Auth(keys.DeleteAll), "DELETE")

		s.routeJSON("/feedback", mw.Auth(feedback.Send), "POST")

		s.routeJSON("/plans", plans.Index, "GET")
		s.routeJSON("/regions", regions.Index, "GET")
		s.routeJSON("/base_images", baseimages.Index, "GET")
	})

	s.static("/", "public") // catch-all, must come last
}
