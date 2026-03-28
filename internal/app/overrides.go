package app

import (
	"scrye/internal/classify"
	"scrye/internal/store"
)

// SetDomainOverride persists a domain classification override.
func SetDomainOverride(db *store.DB, mgr *classify.Manager, domain, category string) error {
	if mgr != nil {
		mgr.SetOverride(domain, category)
		return nil
	}
	return db.SetDomainOverride(domain, category)
}

// DeleteDomainOverride removes a domain classification override.
func DeleteDomainOverride(db *store.DB, mgr *classify.Manager, domain string) error {
	if mgr != nil {
		mgr.RemoveOverride(domain)
		return nil
	}
	return db.DeleteDomainOverride(domain)
}
