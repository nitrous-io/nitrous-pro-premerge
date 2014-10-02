package images

import (
	"archive/tar"
	"bufio"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous-common/flusher"
	"github.com/nitrous-io/nitrous-common/isarchive"

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
	in     io.Reader
	out    io.Writer
	remote string
}

// build sends the build context to freighter in a goroutine. If we want an
// async build, this returns immediately with the current stored image,
// otherwise, the image is updated with new data after the build is done.
func (c *buildContext) build(async bool, isNew bool) (*image.Image, error) {
	ch := make(chan *buildResult, 1)
	env, cl, img, slug := c.env, c.client, c.img, c.slug

	go func() {
		if err := cl.BuildImage(slug, c.in, c.out, c.remote); err != nil {
			env.Events.Publish(
				controllers.NewEvent(controllers.EventImageBuildFailed, map[string]*serializers.Image{
					"image": serializers.SerializeImage(img),
				}),
				env.User.Topic(),
			)

			if isNew {
				if delErr := img.Delete(env.DB); delErr != nil {
					env.Logger.Errorf("images.build: unable to delete image %d that failed to build on freighter: %v", img.ID, delErr)
				}
			}
			ch <- &buildResult{err: err}
			return
		}

		remoteImg, err := cl.InspectImage(img.FullRepository())
		if err != nil {
			ch <- &buildResult{
				err: fmt.Errorf("images.build: failed to inspect image %s: %s", img.FullRepository(), err),
			}
			return
		}

		mergeImage(remoteImg, img)
		if err := img.Update(env.DB); err != nil { // log and continue
			env.Logger.Errorf("images.build: successfully built image '%+v' in Freighter but failed to update database with '%+v'", remoteImg, img)
		}

		env.Events.Publish(
			controllers.NewEvent(controllers.EventImageBuilt, map[string]*serializers.Image{
				"image": serializers.SerializeImage(img),
			}),
			env.User.Topic(),
		)

		ch <- &buildResult{Image: img}
	}()

	// if the build is async, we will just return the stored image immmediately
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

func Build(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	slug := r.FormValue("slug")
	if slug == "" {
		return errSlugRequired
	}
	slug = slugToLower(slug)

	var archive io.Reader

	code := r.FormValue("code")
	remote := r.FormValue("remote")
	if remote != "" {
		// do nothing
	} else if code != "" {
		archive, err = makeArchive(bytes.NewBufferString(code))
	} else {
		_, fh, err := r.FormFile("file")
		if err != nil {
			return errFileRequired
		}

		f, err := fh.Open()
		if err != nil {
			return err
		}
		defer f.Close()
		archive, err = ensureArchive(f)
	}
	if err != nil {
		return err
	}

	var img *image.Image
	var isNew bool
	img, err = image.Find(env.DB, env.User, h, slug)
	if err != nil {
		repo, tag := image.ParseRepoAndTag(slug)
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
		if err := img.Insert(env.DB); err != nil {
			return fmt.Errorf("images/build failed to insert new image %s", err)
		}
		isNew = true
	}

	async, _ := strconv.ParseBool(r.FormValue("async"))
	var writer io.Writer
	pp := &controllers.ProgressPublisher{
		EventName: controllers.EventImageBuilding,
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
	ctx := &buildContext{env: env, client: cl, img: img, slug: slug, in: archive, out: writer, remote: remote}
	img, err = ctx.build(async, isNew)
	if err != nil {
		return err
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

// mergeImage merge changes from remoteImg ( which is returned by freighter )
// to dbImg ( which is the record we stored in database )
func mergeImage(remoteImg *freighterclient.Image, dbImg *image.Image) {
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

func ensureArchive(f multipart.File) (io.Reader, error) {
	bufRdr := bufio.NewReader(f)

	header, err := bufRdr.Peek(isarchive.MaxHeaderLength)
	// Peek can return err even if header is read if read bytes is shorter than requested
	if err != nil && header == nil {
		return nil, err
	}

	// if not archive, make tarball
	if !isarchive.IsArchive(header) {
		b := &bytes.Buffer{}
		_, err := b.ReadFrom(bufRdr)
		if err != nil {
			return nil, err
		}
		return makeArchive(b)
	}

	return bufRdr, nil
}

func makeArchive(dockerfileBuf *bytes.Buffer) (io.Reader, error) {
	buf := bytes.NewBuffer(nil)
	tw := tar.NewWriter(buf)

	now := time.Now()
	hdr := &tar.Header{
		Name:       "Dockerfile",
		Size:       int64(dockerfileBuf.Len()),
		ModTime:    now,
		AccessTime: now,
		ChangeTime: now,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(dockerfileBuf.Bytes()); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}
