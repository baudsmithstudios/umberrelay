package app

import (
	"errors"

	"umberrelay/internal/category"
	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

var ErrInvalidCategory = errors.New("invalid category")

// SetDomainOverride persists a domain classification override.
func SetDomainOverride(db *store.DB, mgr *classify.Manager, domain, categoryValue string) error {
	normalizedCategory, ok := normalizeOverrideCategory(categoryValue)
	if !ok {
		return ErrInvalidCategory
	}
	if mgr != nil {
		return mgr.SetOverride(domain, normalizedCategory)
	}
	return db.SetDomainOverride(domain, normalizedCategory)
}

// DeleteDomainOverride removes a domain classification override.
func DeleteDomainOverride(db *store.DB, mgr *classify.Manager, domain string) error {
	if mgr != nil {
		return mgr.RemoveOverride(domain)
	}
	return db.DeleteDomainOverride(domain)
}

func normalizeOverrideCategory(value string) (string, bool) {
	normalized, ok := category.Normalize(value)
	if !ok {
		return "", false
	}
	if normalized == "" {
		return "", false
	}
	return normalized, true
}
