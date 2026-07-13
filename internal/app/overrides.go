package app

import (
	"errors"

	"umberrelay/internal/category"
	"umberrelay/internal/classify"
)

var ErrInvalidCategory = errors.New("invalid category")

func SetDomainOverride(mgr *classify.Manager, domain, categoryValue string) error {
	normalizedCategory, ok := category.Normalize(categoryValue)
	if !ok {
		return ErrInvalidCategory
	}
	return mgr.SetOverride(domain, normalizedCategory)
}
