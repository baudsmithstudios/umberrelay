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

func TestUpdateDeviceLabelReturnsNotFoundForMissingDevice(t *testing.T) {
	db := testDB(t)
	err := UpdateDeviceLabel(db, "aa:bb:cc:dd:ee:ff", "Living Room TV")
	if err == nil {
		t.Fatal("UpdateDeviceLabel succeeded, want error")
	}
	if err.Error() != "device not found" {
		t.Fatalf("error = %q, want %q", err.Error(), "device not found")
	}
}

func TestUpdateSourceLabel(t *testing.T) {
	db := testDB(t)

	if err := UpdateSourceLabel(db, "10.44.0.7", "Kitchen Display"); err != nil {
		t.Fatalf("UpdateSourceLabel: %v", err)
	}

	label, err := db.GetSourceLabel("10.44.0.7")
	if err != nil {
		t.Fatalf("GetSourceLabel: %v", err)
	}
	if label != "Kitchen Display" {
		t.Fatalf("label = %q, want %q", label, "Kitchen Display")
	}
}
