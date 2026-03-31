package app

import (
	"testing"

	"umberrelay/internal/classify"
)

func TestSetDomainOverridePersistsOverride(t *testing.T) {
	db := testDB(t)
	mgr := classify.NewManager(db)

	if err := SetDomainOverride(db, mgr, "example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	overrides, err := db.ListDomainOverrides()
	if err != nil {
		t.Fatalf("ListDomainOverrides: %v", err)
	}
	if overrides["example.com"] != "tracking" {
		t.Fatalf("override = %q, want %q", overrides["example.com"], "tracking")
	}
}

func TestDeleteDomainOverrideRemovesOverride(t *testing.T) {
	db := testDB(t)
	mgr := classify.NewManager(db)

	if err := SetDomainOverride(db, mgr, "example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}
	if err := DeleteDomainOverride(db, mgr, "example.com"); err != nil {
		t.Fatalf("DeleteDomainOverride: %v", err)
	}

	overrides, err := db.ListDomainOverrides()
	if err != nil {
		t.Fatalf("ListDomainOverrides: %v", err)
	}
	if _, ok := overrides["example.com"]; ok {
		t.Fatalf("override still present: %#v", overrides)
	}
}

func TestSetDomainOverrideReturnsErrorWhenPersistenceFails(t *testing.T) {
	db := testDB(t)
	mgr := classify.NewManager(db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := SetDomainOverride(db, mgr, "example.com", "tracking"); err == nil {
		t.Fatalf("SetDomainOverride() error = nil, want non-nil")
	}
}

func TestDeleteDomainOverrideReturnsErrorWhenPersistenceFails(t *testing.T) {
	db := testDB(t)
	mgr := classify.NewManager(db)

	if err := SetDomainOverride(db, mgr, "example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}
	if got := mgr.Classify("example.com."); got != "tracking" {
		t.Fatalf("Classify before delete = %q, want tracking", got)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := DeleteDomainOverride(db, mgr, "example.com"); err == nil {
		t.Fatalf("DeleteDomainOverride() error = nil, want non-nil")
	}
	if got := mgr.Classify("example.com."); got != "tracking" {
		t.Fatalf("Classify after failed delete = %q, want tracking", got)
	}
}
