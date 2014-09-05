package images

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
)

type buildResult struct {
	*image.Image
	err error
}

type buildContext struct {
	env    *middleware.Env
	client *controllers.FreighterClient
	img    *image.Image
	slug   string
	input  io.Reader
}

// build sends the build context to freighter in a goroutine. If we want an
// async build, this returns immediately with the current stored image,
// otherwise, the image is updated with new data after the build is done.
//
// this also returns the recommended HTTP status for user's convenient
func (c *buildContext) build(async bool) (*image.Image, int, error) {
	ch := make(chan *buildResult, 1)
	env, cl, img, slug, input := c.env, c.client, c.img, c.slug, c.input

	go func() {
		remoteImg, err := cl.BuildImage(slug, input)
		if err != nil {
			if delErr := img.Delete(env.DB); delErr != nil {
				env.Logger.Errorf("Unable to delete image %d that failed to build on freighter: %v", img.ID, delErr)
			}
			ch <- &buildResult{err: err}
			return
		}

		mergeImage(img, remoteImg)
		if err := img.Update(env.DB); err != nil { // log and continue
			env.Logger.Errorf("images.build: successfully built image '%+v' in Freighter but failed to update database with '%+v'", remoteImg, img)
		}

		// FIXME When BuildImage is changed to stream the build progress, this
		// should be changed to publish the build progress periodically.
		env.Events.Publish(
			controllers.NewEvent("image.built", serializers.SerializeImage(img)),
			env.User.Topic(),
		)

		ch <- &buildResult{Image: img}
	}()

	// if the build is async, we will just return the stored image immmediately
	// without waiting for result from freighter
	if async {
		return img, http.StatusAccepted, nil
	}
	r := <-ch
	if r.err != nil {
		return nil, 0, r.err
	}
	return r.Image, http.StatusOK, nil
}

func Build(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	_, fh, err := r.FormFile("file")
	if err != nil {
		return errFileRequired
	}
	slug := r.FormValue("slug")
	if slug == "" {
		return errSlugRequired
	}
	slug = slugToLower(slug)

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	repo, tag := image.ParseRepoAndTag(slug)
	if tag == "" {
		tag = image.DefaultTag
	}

	img := &image.Image{
		UserID: env.User.ID,
		HostID: h.ID,
		Name:   r.FormValue("name"),
		Slug:   repo,
		Tag:    tag,
	}
	if err := img.Insert(env.DB); err != nil {
		return err
	}

	ctx := &buildContext{env: env, client: cl, img: img, slug: slug, input: f}
	async, _ := strconv.ParseBool(r.FormValue("async"))

	var st int
	img, st, err = ctx.build(async)
	if err != nil {
		return err
	}
	env.Status = st
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(img),
	}

	return nil
}

// mergeImage merge changes from remoteImg ( which is returned by freighter )
// to dbImg ( which is the record we stored in database )
func mergeImage(dbImg *image.Image, remoteImg *freighterclient.Image) {
	tag := remoteImg.Tag
	if tag == "" {
		tag = image.DefaultTag
	}
	var builtAt *time.Time
	if remoteImg.CreatedAt != 0 {
		t := time.Unix(remoteImg.CreatedAt, 0).UTC()
		builtAt = &t
	}
	dbImg.FreighterID = remoteImg.ID
	dbImg.Slug = remoteImg.Name
	dbImg.BuiltAt = builtAt
	dbImg.Tag = tag
	dbImg.ActualSize = remoteImg.ActualSize
	dbImg.VirtualSize = remoteImg.VirtualSize
}
