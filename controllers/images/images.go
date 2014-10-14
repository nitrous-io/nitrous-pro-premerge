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
	errRepoRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "repository is required")
	errTagRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "tag is required")
	errSlugRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errImageInUse       = httperror.New(http.StatusBadRequest, "invalid_request", "image is being used by one or more containers")
	errImageMissing     = httperror.New(http.StatusNotFound, "not_found", "image does not exist")
	errFileRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "an archive or a plain file is required")
	errImageAuthFailed  = httperror.New(http.StatusBadRequest, "invalid_request", "you do not have access to this image")

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

	onlyHidden, _ := strconv.ParseBool(r.FormValue("hidden"))

	if onlyHidden {
		hiddenImages := []*image.Image{}
		for _, img := range fImgs {
			dbImg := findMatchingDbImg(img, dbImgs)
			if dbImg == nil {
				hiddenImages = append(hiddenImages, convertImage(img))
			}
		}
		dbImgs = hiddenImages
	} else {
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
		if fImg.ID == i.FreighterID && fImg.Name == i.Slug && fImg.Tag == i.Tag {
			return fImg
		}
	}
	return nil
}

func findMatchingDbImg(i *freighterclient.Image, images []*image.Image) *image.Image {
	for _, dbImg := range images {
		if dbImg.FreighterID == i.ID && dbImg.Slug == i.Name && dbImg.Tag == i.Tag {
			return dbImg
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

	img, err := image.Find(db, env.User, h, slug)
	if err != nil {
		return errImageMissing
	}

	if err := cl.RemoveImage(img.FullRepository()); err != nil {
		if err == freighterclient.ErrImageInUse {
			return errImageInUse
		}
		return err
	}

	if err := img.Delete(db); err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(controllers.EventImageRemoved, map[string]*serializers.Image{
			"image": serializers.SerializeImage(img),
		}),
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
		controllers.NewEvent(controllers.EventImageTagged, map[string]*serializers.Image{
			"image": serializers.SerializeImage(taggedImg),
		}), env.User.Topic())
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": serializers.SerializeImage(taggedImg),
	}
	return nil
}

func Inspect(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	slug := mux.Vars(r)["image"]
	if slug == "" {
		return errIDOrSlugRequired
	}

	img, err := cl.InspectImage(slug)
	if err != nil {
		if err == freighterclient.ErrImageNotFound {
			return errImageMissing
		}

		if err == freighterclient.ErrImageAuthNotAllowed {
			return errImageAuthFailed
		}

		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": img,
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
