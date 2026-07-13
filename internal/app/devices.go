package app

import (
	"errors"

	"umberrelay/internal/store"
)

var ErrDeviceNotFound = errors.New("device not found")

func UpdateDeviceLabel(db *store.DB, mac, label string) error {
	err := db.UpdateDeviceLabel(mac, label)
	if errors.Is(err, store.ErrNotFound) {
		return ErrDeviceNotFound
	}
	return err
}
