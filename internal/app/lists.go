package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

var ErrListNotFound = errors.New("list not found")

// AddListInput holds user input for adding a classification list.
type AddListInput struct {
	URL      string
	Name     string
	Category string
}

// AddList validates and stores a classification list.
func AddList(ctx context.Context, db *store.DB, input AddListInput) (int64, error) {
	input.URL = strings.TrimSpace(input.URL)
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)

	if input.URL == "" || input.Name == "" || input.Category == "" {
		return 0, fmt.Errorf("url, name, and category are required")
	}
	if !validCategory(input.Category) {
		return 0, fmt.Errorf("invalid category")
	}
	if _, err := classify.ParseAndValidateListURL(ctx, input.URL); err != nil {
		return 0, err
	}
	return db.AddList(input.URL, input.Name, input.Category)
}

// UpdateListEnabled toggles whether a list participates in classification.
func UpdateListEnabled(db *store.DB, id int64, enabled bool) error {
	err := db.UpdateListEnabled(id, enabled)
	if errors.Is(err, store.ErrNotFound) {
		return ErrListNotFound
	}
	return err
}

// DeleteList removes a classification list.
func DeleteList(db *store.DB, id int64) error {
	err := db.DeleteList(id)
	if errors.Is(err, store.ErrNotFound) {
		return ErrListNotFound
	}
	return err
}

// EnabledListSources returns the currently enabled classification lists.
func EnabledListSources(db *store.DB) ([]classify.ListSource, error) {
	lists, err := db.ListLists()
	if err != nil {
		return nil, err
	}

	var sources []classify.ListSource
	for _, list := range lists {
		if list.Enabled {
			sources = append(sources, classify.ListSource{
				ID:       list.ID,
				URL:      list.URL,
				Name:     list.Name,
				Category: list.Category,
			})
		}
	}

	return sources, nil
}

// RefreshListSources reloads the provided classification lists into the manager.
func RefreshListSources(ctx context.Context, mgr *classify.Manager, sources []classify.ListSource) error {
	if mgr == nil {
		return fmt.Errorf("classify manager not available")
	}
	return mgr.Refresh(ctx, sources)
}

func validCategory(category string) bool {
	switch category {
	case "tracking", "advertising", "analytics", "telemetry", "malware", "uncategorized":
		return true
	default:
		return false
	}
}
