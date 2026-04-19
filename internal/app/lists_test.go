package app

import (
	"context"
	"testing"
)

func TestAddListPersistsTrimmedValues(t *testing.T) {
	db := testDB(t)

	id, err := AddList(context.Background(), db, AddListInput{
		URL:      " https://93.184.216.34/list.txt ",
		Name:     " Example ",
		Category: " tracking ",
	})
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if id == 0 {
		t.Fatalf("id = %d, want non-zero", id)
	}

	lists, err := db.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("len(lists) = %d, want 1", len(lists))
	}
	if lists[0].URL != "https://93.184.216.34/list.txt" {
		t.Fatalf("url = %q, want trimmed URL", lists[0].URL)
	}
	if lists[0].Name != "Example" {
		t.Fatalf("name = %q, want %q", lists[0].Name, "Example")
	}
	if lists[0].Category != "tracking" {
		t.Fatalf("category = %q, want %q", lists[0].Category, "tracking")
	}
}

func TestAddListRejectsInvalidCategory(t *testing.T) {
	db := testDB(t)

	_, err := AddList(context.Background(), db, AddListInput{
		URL:      "https://93.184.216.34/list.txt",
		Name:     "Example",
		Category: "invalid",
	})
	if err == nil {
		t.Fatal("AddList succeeded, want error")
	}
	if err.Error() != "invalid category" {
		t.Fatalf("error = %q, want %q", err.Error(), "invalid category")
	}
}

func TestAddListNormalizesUnclassifiedAlias(t *testing.T) {
	db := testDB(t)

	_, err := AddList(context.Background(), db, AddListInput{
		URL:      "https://93.184.216.34/list.txt",
		Name:     "Example",
		Category: "unclassified",
	})
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}

	lists, err := db.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 1 {
		t.Fatalf("len(lists) = %d, want 1", len(lists))
	}
	if lists[0].Category != "uncategorized" {
		t.Fatalf("category = %q, want %q", lists[0].Category, "uncategorized")
	}
}

func TestAddListRejectsMissingFields(t *testing.T) {
	db := testDB(t)

	_, err := AddList(context.Background(), db, AddListInput{})
	if err == nil {
		t.Fatal("AddList succeeded, want error")
	}
	if err.Error() != "url, name, and category are required" {
		t.Fatalf("error = %q, want %q", err.Error(), "url, name, and category are required")
	}
}

func TestEnabledListSourcesReturnsOnlyEnabledLists(t *testing.T) {
	db := testDB(t)

	firstID, err := db.AddList("https://93.184.216.34/one.txt", "One", "tracking")
	if err != nil {
		t.Fatalf("AddList(One): %v", err)
	}
	secondID, err := db.AddList("https://93.184.216.34/two.txt", "Two", "analytics")
	if err != nil {
		t.Fatalf("AddList(Two): %v", err)
	}
	if err := db.UpdateListEnabled(secondID, false); err != nil {
		t.Fatalf("UpdateListEnabled: %v", err)
	}

	sources, err := EnabledListSources(db)
	if err != nil {
		t.Fatalf("EnabledListSources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].ID != firstID {
		t.Fatalf("source ID = %d, want %d", sources[0].ID, firstID)
	}
}

func TestUpdateListEnabledReturnsNotFoundForMissingList(t *testing.T) {
	db := testDB(t)
	err := UpdateListEnabled(db, 999, false)
	if err == nil {
		t.Fatal("UpdateListEnabled succeeded, want error")
	}
	if err.Error() != "list not found" {
		t.Fatalf("error = %q, want %q", err.Error(), "list not found")
	}
}

func TestDeleteListReturnsNotFoundForMissingList(t *testing.T) {
	db := testDB(t)
	err := DeleteList(db, 999)
	if err == nil {
		t.Fatal("DeleteList succeeded, want error")
	}
	if err.Error() != "list not found" {
		t.Fatalf("error = %q, want %q", err.Error(), "list not found")
	}
}
