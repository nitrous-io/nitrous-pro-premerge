package images

import (
	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"

	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errIDOrNameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "id or name is required")
	errRepoRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "repository is required")
	errDestRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "destination is required")
	errTagRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "tag is required")
	errNameRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "name is required")
	errFileRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "file is required")
)

func Index(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	images, err := cl.ListImages()
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"images": images,
	}

	return nil
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

	name := r.FormValue("name")
	if name == "" {
		return errNameRequired
	}

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	image, err := cl.BuildImage(name, f)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image": image,
	}

	return nil
}

func Remove(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	image := r.FormValue("image")
	if image == "" {
		return errIDOrNameRequired
	}

	if err := cl.RemoveImage(image); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}

func Pull(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	repo := r.FormValue("repository")
	if repo == "" {
		return errRepoRequired
	}

	image, err := cl.PullImage(repo)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
		"image":  image,
	}
	return nil
}

func Push(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	image := r.FormValue("image")
	if image == "" {
		return errIDOrNameRequired
	}

	dest := r.FormValue("destination")
	if dest == "" {
		return errDestRequired
	}

	if err := cl.PushImage(image, dest); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}

func Tag(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	image := r.FormValue("image")
	if image == "" {
		return errIDOrNameRequired
	}

	tag := r.FormValue("tag")
	if tag == "" {
		return errTagRequired
	}

	force := r.FormValue("force") != ""

	if err := cl.TagImage(image, tag, force); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}
