package app

import (
	"context"
	"errors"
	"strings"

	"umberrelay/internal/category"
	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

var ErrListNotFound = errors.New("list not found")

type AddListInput struct {
	URL      string
	Name     string
	Category string
}

func AddList(ctx context.Context, db *store.DB, input AddListInput) (int64, error) {
	input.URL = strings.TrimSpace(input.URL)
	input.Name = strings.TrimSpace(input.Name)
	input.Category = strings.TrimSpace(input.Category)

	if input.URL == "" || input.Name == "" || input.Category == "" {
		return 0, newInvalidInputError("url, name, and category are required")
	}
	normalizedCategory, ok := category.Normalize(input.Category)
	if !ok {
		return 0, newInvalidInputError("invalid category")
	}
	input.Category = normalizedCategory
	if _, err := classify.ParseAndValidateListURL(ctx, input.URL); err != nil {
		return 0, newInvalidInputError(err.Error())
	}
	return db.AddList(input.URL, input.Name, input.Category)
}

func UpdateListEnabled(db *store.DB, id int64, enabled bool) error {
	err := db.UpdateListEnabled(id, enabled)
	if errors.Is(err, store.ErrNotFound) {
		return ErrListNotFound
	}
	return err
}

func DeleteList(db *store.DB, id int64) error {
	err := db.DeleteList(id)
	if errors.Is(err, store.ErrNotFound) {
		return ErrListNotFound
	}
	return err
}

func EnabledListSources(db *store.DB) ([]classify.ListSource, error) {
	lists, err := db.ListEnabledLists()
	if err != nil {
		return nil, err
	}
	return classify.SourcesFromListEntries(lists), nil
}
