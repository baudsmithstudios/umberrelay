package app

import (
	"errors"

	"scrye/internal/store"
)

var ErrDeviceNotFound = errors.New("device not found")

// UpdateDeviceLabel persists a user-assigned device label.
func UpdateDeviceLabel(db *store.DB, mac, label string) error {
	err := db.UpdateDeviceLabel(mac, label)
	if errors.Is(err, store.ErrNotFound) {
		return ErrDeviceNotFound
	}
	return err
}
