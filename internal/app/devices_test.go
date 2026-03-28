package app

import "testing"

func TestUpdateDeviceLabelPersistsLabel(t *testing.T) {
	db := testDB(t)
	if err := db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := UpdateDeviceLabel(db, "aa:bb:cc:dd:ee:ff", "Living Room TV"); err != nil {
		t.Fatalf("UpdateDeviceLabel: %v", err)
	}

	device, err := db.GetDevice("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if device.Label != "Living Room TV" {
		t.Fatalf("label = %q, want %q", device.Label, "Living Room TV")
	}
}
