package publickey

import (
	"fmt"
	"time"

	"github.com/nitrous-io/nitrous-common/sshkey"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
)

var (
	ErrNotFound   = fmt.Errorf("public key not found")
	ErrKeyBlank   = fmt.Errorf("missing public key")
	ErrInvalidKey = fmt.Errorf("invalid public key")
)

type PublicKey struct {
	ID          int64      `db:"id"`
	UserID      int64      `db:"user_id"`
	ContainerID string     `db:"container_id"`
	Fingerprint string     `db:"fingerprint"`
	Key         string     `db:"key"`
	CreatedAt   *time.Time `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}

func FindAllByUser(db *db.Database, user *user.User) ([]PublicKey, error) {
	return FindAllByUserID(db, user.ID)
}

func FindAllByUserID(db *db.Database, userID int64) ([]PublicKey, error) {
	keys := []PublicKey{}
	err := db.Select(&keys, "SELECT * FROM public_keys WHERE user_id = $1", userID)
	if err != nil {
		return []PublicKey{}, err
	}
	return keys, nil
}

func DestroyAllByUserID(db *db.Database, userID int64) error {
	_, err := db.Queryx("DELETE FROM public_keys WHERE user_id = $1", userID)
	return err
}

func DestroyByUserIDAndFingerprint(db *db.Database, userID int64, fingerprint string) error {
	keys := []PublicKey{}
	sqlf := "%s FROM public_keys WHERE user_id = $1 AND fingerprint = $2"
	err := db.Select(&keys, fmt.Sprintf(sqlf, "SELECT *"), userID, fingerprint)

	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return ErrNotFound
	}

	_, err = db.Queryx(fmt.Sprintf(sqlf, "DELETE"), userID, fingerprint)
	return err
}

func DestroyByUserIDAndContainerIDAndFingerprint(db *db.Database, userID int64, containerID string, fingerprint string) error {
	keys := []PublicKey{}
	sqlf := "%s FROM public_keys WHERE user_id = $1 AND container_id = $2 AND fingerprint = $3"
	err := db.Select(&keys, fmt.Sprintf(sqlf, "SELECT *"), userID, containerID, fingerprint)

	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return ErrNotFound
	}

	_, err = db.Queryx(fmt.Sprintf(sqlf, "DELETE"), userID, containerID, fingerprint)
	return err
}

func FindAllByContainerID(db *db.Database, containerID string) ([]PublicKey, error) {
	keys := []PublicKey{}
	err := db.Select(&keys, "SELECT * FROM public_keys WHERE container_id = $1", containerID)
	if err != nil {
		return []PublicKey{}, err
	}
	return keys, nil
}

func (pk *PublicKey) Insert(d db.QueryReceiver) error {
	// this is a very basic check for valid characters; it will not catch
	// keys like "ssh-rsa fail"
	fingerprint, err := pk.Md5Fingerprint()
	if err != nil {
		return ErrInvalidKey
	}
	pk.Fingerprint = fingerprint

	nstmt, err := d.PrepareNamed(`INSERT INTO public_keys
		(user_id, container_id, key, fingerprint)
		VALUES (:user_id, :container_id, :key, :fingerprint)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pk).StructScan(pk)
}

// Fingerprint is set in Insert()
func (pk *PublicKey) Validate() error {
	if pk.Key == "" {
		return ErrKeyBlank
	}
	// We can do more checks with
	// https://godoc.org/code.google.com/p/go.crypto/ssh#ParsePublicKey but
	// seems overkill for now
	return nil
}

func (pk *PublicKey) Md5Fingerprint() (string, error) {
	return sshkey.Md5Fingerprint(pk.Key)
}
