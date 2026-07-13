package app

import (
	"testing"

	"umberrelay/internal/classify"
)

func TestSetDomainOverridePersistsOverride(t *testing.T) {
	db := testDB(t)
	mgr := classify.NewManager(db)

	if err := SetDomainOverride(mgr, "example.com", "tracking"); err != nil {
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

func TestSetDomainOverrideRejectsInvalidCategory(t *testing.T) {
	mgr := classify.NewManager(testDB(t))

	if err := SetDomainOverride(mgr, "example.com", "not-a-real-category"); err == nil {
		t.Fatal("SetDomainOverride() error = nil, want non-nil")
	}
}
