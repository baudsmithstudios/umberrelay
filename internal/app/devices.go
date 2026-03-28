package app

import "scrye/internal/store"

// UpdateDeviceLabel persists a user-assigned device label.
func UpdateDeviceLabel(db *store.DB, mac, label string) error {
	return db.UpdateDeviceLabel(mac, label)
}
