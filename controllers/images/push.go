package images

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/flusher"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
)

type pushContext struct {
	env    *middleware.Env
	client *controllers.FreighterClient
	img    *image.Image
	repo   string
	writer io.Writer
}

func (c *pushContext) run(async bool) error {
	ch := make(chan error, 1)
	env, cl, img, repo := c.env, c.client, c.img, c.repo

	type data struct {
		Image      string `json:"image"`
		Repository string `json:"repository"`
	}

	go func() {
		if err := cl.PushImage(img.FullRepository(), repo, c.writer); err != nil {
			var finalErr error
			if err == freighterclient.ErrImagePushAuthRequired {
				finalErr = err
			} else {
				finalErr = fmt.Errorf("failed to push image %s", err)
			}

			env.Events.Publish(
				controllers.NewEvent(controllers.EventImagePushFailed, data{Image: img.FullRepository(), Repository: repo}),
				env.User.Topic(),
			)

			ch <- finalErr

			return
		}

		env.Data = map[string]interface{}{
			"status": "ok",
		}
		env.Events.Publish(
			controllers.NewEvent(controllers.EventImagePushed, data{Image: img.FullRepository(), Repository: repo}),
			env.User.Topic(),
		)

		ch <- nil
	}()
	if async {
		return nil
	}

	r := <-ch
	if r != nil {
		return r
	}
	return nil
}

func Push(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slugOrId := mux.Vars(r)["image"]
	if slugOrId == "" {
		return errIDOrSlugRequired
	}

	repo := r.FormValue("repository")
	if repo == "" {
		return errRepoRequired
	}

	img, err := image.Find(db, env.User, h, slugOrId)
	if err != nil {
		return errImageMissing
	}

	async, _ := strconv.ParseBool(r.FormValue("async"))
	// if async, we will just discard the progress data from freighter. It will
	// only be outputed into the events stream. The only output we want to
	// include in async is the image data
	var writer io.Writer
	pp := &controllers.ProgressPublisher{
		EventName: controllers.EventImagePushing,
		Topic:     env.User.Topic(),
		Events:    env.Events,
		ID:        img.ID,
		Slug:      img.Slug,
	}
	if async {
		writer = pp
	} else {
		writer = io.MultiWriter(pp, flusher.NewWriteFlusher(w))
	}

	ctx := &pushContext{env: env, client: cl, img: img, repo: repo, writer: writer}

	if err := ctx.run(async); err != nil {
		if err == freighterclient.ErrImagePushAuthRequired {
			env.Status = http.StatusUnauthorized
			env.Data = map[string]interface{}{
				"error":             "registry_auth_failed",
				"error_description": err.Error(),
			}
			return nil
		}
		return err
	}

	if async {
		env.Status = http.StatusAccepted
	} else {
		env.Status = http.StatusOK
	}
	env.Data = map[string]interface{}{
		"status": "ok",
	}

	return nil
}
