package images

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"

	"github.com/nitrous-io/nitrous/pkg/httperror"
)

type imageJSON struct {
	ID          string `json:"id"`
	HostID      int64  `json:"host_id"`
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	CreatedAt   int64  `json:"created_at"`
	ActualSize  int64  `json:"actual_size"`
	VirtualSize int64  `json:"virtual_size"`
}

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

	imageJSONs := make([]*imageJSON, 0, len(images))
	for _, image := range images {
		imageJSONs = append(imageJSONs, &imageJSON{
			ID:          image.ID,
			HostID:      h.ID,
			Name:        image.Name,
			Tag:         image.Tag,
			CreatedAt:   image.CreatedAt,
			ActualSize:  image.ActualSize,
			VirtualSize: image.VirtualSize,
		})
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"images": imageJSONs,
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
	fmt.Println(fh)

	name := r.FormValue("name")
	if name == "" {
		return errNameRequired
	}
	fmt.Println(name)

	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	image, err := cl.BuildImage(name, f)
	if err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent("image.built", image),
		env.User.Topic(),
	)
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

	image := mux.Vars(r)["image"]
	if image == "" {
		return errIDOrNameRequired
	}

	if err := cl.RemoveImage(image); err != nil {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent("image.removed", image),
		env.User.Topic(),
	)
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

	env.Events.Publish(
		controllers.NewEvent("image.pulled", image),
		env.User.Topic(),
	)
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

	image := mux.Vars(r)["image"]
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

	type data struct {
		Image       string `json:"image"`
		Destination string `json:"destination"`
	}
	env.Events.Publish(
		controllers.NewEvent("image.pushed", data{Image: image, Destination: dest}),
		env.User.Topic(),
	)
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

	image := mux.Vars(r)["image"]
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

	type data struct {
		Image string `json:"image"`
		Tag   string `json:"tag"`
	}
	env.Events.Publish(
		controllers.NewEvent("image.tagged", data{Image: image, Tag: tag}),
		env.User.Topic(),
	)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "OK",
	}
	return nil
}
