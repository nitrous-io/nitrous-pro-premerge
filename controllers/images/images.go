package images

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"

	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errIDOrSlugRequired = httperror.New(http.StatusBadRequest, "invalid_request", "id or slug is required")
	errDestRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "destination is required")
	errTagRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "tag is required")
	errSlugRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errImageInUse       = httperror.New(http.StatusBadRequest, "invalid_request", "image is being used by one or more containers")
	errImageMissing     = httperror.New(http.StatusNotFound, "not_found", "image does not exist")
	errFileRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "an archive or a plain file is required")

	errInvalidArchive = httperror.New(http.StatusBadRequest, "invalid_request", "not a valid Docker archive")
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hSlug := mux.Vars(r)["slug"]
	var (
		hosts      []host.Host
		hostFilter *host.Host
		db         = env.DB
		user       = env.User
	)

	// If no host is provided, then get all hosts belonging to the user
	// and get all images from all hosts
	if hSlug != "" {
		h, err := host.Find(db, user, hSlug)
		if err != nil {
			return err
		}
		if h == nil {
			return middleware.ErrHostNotFound
		}
		hostFilter = h
	}

	if hostFilter == nil {
		var err error
		hosts, err = host.FindAllByUser(db, user)
		if err != nil {
			return err
		}
	} else {
		hosts = []host.Host{*hostFilter}
	}

	dbImgs, err := image.FindAll(db, user, hostFilter)
	if err != nil {
		return err
	}

	fImgs, err := findImagesInHosts(db, hosts)
	if err != nil {
		return err
	}

	for _, img := range dbImgs {
		fImg := findMatchingFreighterImage(img, fImgs)
		if fImg == nil { // log and continue
			env.Logger.Errorf("images.index: image '%+v' exists in database but not in Freighter: %v", img, err)
			continue
		}
		img.ActualSize = fImg.ActualSize
		img.VirtualSize = fImg.VirtualSize
		if fImg.CreatedAt != 0 {
			t := time.Unix(fImg.CreatedAt, 0).UTC()
			img.BuiltAt = &t
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"images": serializers.SerializeImages(dbImgs),
	}

	return nil
}

func findImagesInHosts(db db.QueryReceiver, hosts []host.Host) ([]*freighterclient.Image, error) {
	var allImgs []*freighterclient.Image
	for _, h := range hosts {
		cl, err := controllers.NewFreighterClient(db, &h)
		if err != nil {
			return nil, err
		}

		hImgs, err := cl.ListImages()
		if err != nil {
			return nil, err
		}

		allImgs = append(allImgs, hImgs...)
	}
	return allImgs, nil
}

func findMatchingFreighterImage(i *image.Image, images []*freighterclient.Image) *freighterclient.Image {
	for _, fImg := range images {
		if fImg.ID == i.FreighterID && fImg.Name == i.Slug && i.Tag == i.Tag {
			return fImg
		}
	}
	return nil
}

func Remove(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["image"]
	if slug == "" {
		return errIDOrSlugRequired
	}

	i, err := image.Find(db, env.User, h, slug)
	if err != nil {
		return errImageMissing
	}

	if err := cl.RemoveImage(i.FullRepository()); err != nil {
		if err == freighterclient.ErrImageInUse {
			return errImageInUse
		}
		return err
	}

	if err := i.Delete(db); err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventImageRemoved, serializers.SerializeImage(i)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func Push(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	image := mux.Vars(r)["image"]
	if image == "" {
		return errIDOrSlugRequired
	}

	dest := r.FormValue("destination")
	if dest == "" {
		return errDestRequired
	}

	type data struct {
		Image       string `json:"image"`
		Destination string `json:"destination"`
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventImagePushing, data{Image: image, Destination: dest}),
		env.User.Topic(),
	)

	if err := cl.PushImage(image, dest); err != nil {
		if err == freighterclient.ErrImagePushAuthRequired {
			env.Status = http.StatusUnauthorized
			env.Data = map[string]interface{}{
				"error":             "invalid_request",
				"error_description": err.Error(),
			}
			return nil
		}
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventImagePushed, data{Image: image, Destination: dest}),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func Tag(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["image"]
	if slug == "" {
		return errIDOrSlugRequired
	}
	newRepo := r.FormValue("tag")
	if newRepo == "" {
		return errTagRequired
	}
	newRepo = slugToLower(newRepo)
	force, _ := strconv.ParseBool(r.FormValue("force"))

	srcImg, err := image.Find(db, env.User, h, slug)
	if err != nil {
		return errImageMissing
	}

	if err := cl.TagImage(srcImg.FullRepository(), newRepo, force); err != nil {
		return err
	}

	repo, tag := image.ParseRepoAndTag(newRepo)
	if tag == "" {
		tag = image.DefaultTag
	}
	taggedImg := &image.Image{
		UserID:      env.User.ID,
		HostID:      h.ID,
		FreighterID: srcImg.FreighterID,
		Name:        r.FormValue("name"),
		Slug:        repo,
		Tag:         tag,
	}
	// If image record already exists, update it.
	if existing, err := image.Find(db, env.User, h, newRepo); err == nil {
		taggedImg.ID = existing.ID
		if taggedImg.Name == "" {
			taggedImg.Name = existing.Name
		}
	}
	if err := taggedImg.InsertOrUpdate(db); err != nil { // log and continue
		env.Logger.Errorf("images.tag: successfully tagged image '%+v' as '%s' in Freighter but failed to update database with '%+v'", srcImg, newRepo, taggedImg)
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventImageTagged, serializers.SerializeImage(taggedImg)),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(taggedImg),
	}
	return nil
}

func convertImage(img *freighterclient.Image) *image.Image {
	tag := img.Tag
	if tag == "" {
		tag = image.DefaultTag
	}
	var builtAt *time.Time
	if img.CreatedAt != 0 {
		t := time.Unix(img.CreatedAt, 0).UTC()
		builtAt = &t
	}
	return &image.Image{
		FreighterID: img.ID,
		Slug:        img.Name,
		BuiltAt:     builtAt,
		Tag:         tag,
		ActualSize:  img.ActualSize,
		VirtualSize: img.VirtualSize,
	}
}

// slugToLower converts the name portion of an image repo to lowercase but
// leaves the tag portion unchanged.
// E.g. "NitrousIO/Scratch:MyTag" -> "nitrousio/scratch:MyTag"
func slugToLower(slug string) string {
	repo, tag := image.ParseRepoAndTag(slug)
	repo = strings.ToLower(repo)
	if tag != "" {
		repo = repo + ":" + tag
	}
	return repo
}
