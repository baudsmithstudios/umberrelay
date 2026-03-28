package app

import (
	"context"
	"fmt"
	"strings"

	"scrye/internal/classify"
	"scrye/internal/store"
)

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
	return db.UpdateListEnabled(id, enabled)
}

// DeleteList removes a classification list.
func DeleteList(db *store.DB, id int64) error {
	return db.DeleteList(id)
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

func validCategory(category string) bool {
	switch category {
	case "tracking", "advertising", "analytics", "telemetry", "malware", "uncategorized":
		return true
	default:
		return false
	}
}
