package images

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/nitrous-io/nitrous-common/flusher"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
)

type pullContext struct {
	env    *middleware.Env
	client *controllers.FreighterClient
	img    *image.Image
	repo   string
	update bool
	writer io.Writer
}

// sends the pull context to freighter in a goroutine.
func (c *pullContext) run(async, isNew bool) (*image.Image, error) {
	ch := make(chan *buildResult, 1)
	env, cl, img, repo, update := c.env, c.client, c.img, c.repo, c.update
	db := env.DB

	go func() {
		if err := cl.PullImage(repo, update, c.writer); err != nil {
			// publish image pull failed event with the current image data
			env.Events.Publish(
				controllers.NewEvent(controllers.EventImagePullFailed, map[string]*serializers.Image{
					"image": serializers.SerializeImage(img),
				}),
				env.User.Topic(),
			)
			// if we failed to pull the image, and the image is new i.e. does not
			// exist in db before, we delete the image from db
			if isNew {
				if err := img.Delete(db); err != nil {
					ch <- &buildResult{
						err: fmt.Errorf("rollback new image %s", err),
					}
					return
				}
				env.Events.Publish(
					controllers.NewEvent(controllers.EventImageRemoved, map[string]*serializers.Image{
						"image": serializers.SerializeImage(img),
					}),
					env.User.Topic(),
				)
			}
			ch <- &buildResult{
				err: fmt.Errorf("failed to pull image %s", err),
			}
			return
		}

		remoteImg, err := cl.InspectImage(repo)
		if err != nil {
			ch <- &buildResult{
				err: fmt.Errorf("failed to inspect image %s, err %s", repo, err),
			}
			return
		}

		mergeImage(remoteImg, img)
		// when the image failed to update, just log and continue as we are
		// treating freighterd as the canonical source here
		if err := img.InsertOrUpdate(db); err != nil {
			env.Logger.Errorf("images.pull: successfully pulled image '%+v' in Freighter but failed to update database with '%+v'", remoteImg, img)
		}

		env.Events.Publish(
			controllers.NewEvent(controllers.EventImagePulled, map[string]*serializers.Image{
				"image": serializers.SerializeImage(img),
			}),
			env.User.Topic(),
		)
		ch <- &buildResult{Image: img}
	}()

	// if the pull is async, we will just return the stored image immmediately
	// without waiting for result from freighter
	if async {
		return img, nil
	}
	r := <-ch
	if r.err != nil {
		return nil, r.err
	}
	return r.Image, nil
}

func Pull(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	repo := r.FormValue("repository")
	if repo == "" {
		repo = controllers.DefaultImage
	}
	update, err := strconv.ParseBool(r.FormValue("update"))
	if err != nil {
		update = true
	}

	var img *image.Image
	var isNew bool
	img, err = image.Find(db, env.User, h, repo)
	if err != nil {
		repo, tag := image.ParseRepoAndTag(repo)
		if tag == "" {
			tag = image.DefaultTag
		}
		img = &image.Image{
			UserID: env.User.ID,
			HostID: h.ID,
			Name:   r.FormValue("name"),
			Slug:   repo,
			Tag:    tag,
		}
		if err := img.Insert(db); err != nil {
			return fmt.Errorf("images/pull failed to insert new image %s", err)
		}
		isNew = true
	}

	async, _ := strconv.ParseBool(r.FormValue("async"))
	// if async, we will just discard the progress data from freighter. It will
	// only be outputed into the events stream. The only output we want to
	// include in async is the image data
	var writer io.Writer
	pp := &controllers.ProgressPublisher{
		EventName: controllers.EventImagePulling,
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
	ctx := &pullContext{env: env, client: cl, img: img, repo: repo, update: update, writer: writer}
	img, err = ctx.run(async, isNew)
	if err != nil {
		return fmt.Errorf("images.pull %s", err)
	}

	if async {
		env.Status = http.StatusAccepted
	} else {
		env.Status = http.StatusOK
	}
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(img),
	}
	return nil
}
