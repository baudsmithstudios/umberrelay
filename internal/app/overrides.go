package app

import (
	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

// SetDomainOverride persists a domain classification override.
func SetDomainOverride(db *store.DB, mgr *classify.Manager, domain, category string) error {
	if mgr != nil {
		return mgr.SetOverride(domain, category)
	}
	return db.SetDomainOverride(domain, category)
}

// DeleteDomainOverride removes a domain classification override.
func DeleteDomainOverride(db *store.DB, mgr *classify.Manager, domain string) error {
	if mgr != nil {
		return mgr.RemoveOverride(domain)
	}
	return db.DeleteDomainOverride(domain)
}
