package app

import (
	"testing"

	"umberrelay/internal/classify"
)

func TestUpdateSettingsPersistsValues(t *testing.T) {
	db := testDB(t)
	retentionDays := 7
	refreshHours := 12

	err := UpdateSettings(db, classify.NewManager(db), SettingsInput{
		RetentionDays:    &retentionDays,
		ListRefreshHours: &refreshHours,
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	retention, err := db.GetConfig("retention_days")
	if err != nil {
		t.Fatalf("GetConfig(retention_days): %v", err)
	}
	if retention != "7" {
		t.Fatalf("retention_days = %q, want %q", retention, "7")
	}

	refresh, err := db.GetConfig("list_refresh_hours")
	if err != nil {
		t.Fatalf("GetConfig(list_refresh_hours): %v", err)
	}
	if refresh != "12" {
		t.Fatalf("list_refresh_hours = %q, want %q", refresh, "12")
	}
}
