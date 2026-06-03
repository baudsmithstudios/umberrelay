package app

import (
	"errors"

	"umberrelay/internal/category"
	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

var ErrInvalidCategory = errors.New("invalid category")

func SetDomainOverride(db *store.DB, mgr *classify.Manager, domain, categoryValue string) error {
	normalizedCategory, ok := category.Normalize(categoryValue)
	if !ok {
		return ErrInvalidCategory
	}
	if mgr != nil {
		return mgr.SetOverride(domain, normalizedCategory)
	}
	return db.SetDomainOverride(domain, normalizedCategory)
}

func DeleteDomainOverride(db *store.DB, mgr *classify.Manager, domain string) error {
	if mgr != nil {
		return mgr.RemoveOverride(domain)
	}
	return db.DeleteDomainOverride(domain)
}
