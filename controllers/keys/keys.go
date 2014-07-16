package keys

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/publickey"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrContainerBlank      = httperror.New(http.StatusBadRequest, "validation_failed", "missing container id")
	ErrContainerNotFound   = httperror.New(http.StatusBadRequest, "validation_failed", "container not found")
	ErrInvalidKey          = httperror.New(http.StatusBadRequest, "validation_failed", "invalid key")
	ErrKeyExists           = httperror.New(http.StatusBadRequest, "validation_failed", "key already exists")
	ErrFingerprintNotFound = httperror.New(http.StatusNotFound, "validation_failed", "fingerprint not found")
)

func Index(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	cid, err := getContainerID(db, r, u.ID, true)
	if err != nil {
		return err
	}

	keys := []publickey.PublicKey{}
	if cid == "" {
		keys, err = publickey.FindAllByUser(db, u)
	} else {
		keys, err = publickey.FindAllByUserIDAndContainerID(db, u.ID, cid)
	}
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
		"keys":   keys,
	}
	return nil
}

func Create(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	cid, err := getContainerID(db, r, u.ID, false)
	if err != nil {
		return err
	}

	pk := publickey.PublicKey{
		UserID:      u.ID,
		ContainerID: cid,
		Key:         r.FormValue("keytext"),
	}

	err = pk.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = pk.Insert(tx)
	if err != nil {
		tx.Rollback()
		if err == publickey.ErrInvalidKey {
			return ErrInvalidKey
		}
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" && e.Constraint == "index_public_keys_on_container_id_and_key" {
			return ErrKeyExists
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
	}
	return nil
}

func DeleteAll(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	err := publickey.DestroyAllByUserID(db, u.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func DeleteByFingerprint(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	u := env.User

	fingerprint := mux.Vars(r)["md5_fingerprint"]
	cid, err := getContainerID(db, r, u.ID, true)
	if err != nil {
		return err
	}

	if cid == "" {
		err = publickey.DestroyByUserIDAndFingerprint(db, u.ID, fingerprint)
	} else {
		err = publickey.DestroyByUserIDAndContainerIDAndFingerprint(db, u.ID, cid, fingerprint)
	}
	if err != nil {
		if err == publickey.ErrNotFound {
			return ErrFingerprintNotFound
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func getContainerID(d db.QueryReceiver, r *http.Request, userID int64, optional bool) (string, error) {
	cid := r.FormValue("container_id")
	if cid == "" {
		if optional {
			return cid, nil
		} else {
			return "", ErrContainerBlank
		}
	}
	c, _ := container.FindByID(d, cid)
	if c == nil || c.UserID != userID {
		return "", ErrContainerNotFound
	}
	return c.ID, nil
}
