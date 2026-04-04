package classify

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"umberrelay/internal/store"
)

func TestParseHostsFile(t *testing.T) {
	content := `# comment
0.0.0.0 ads.example.com
0.0.0.0 tracker.example.com
127.0.0.1 localhost
`
	domains := parseHostsFile([]byte(content))
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}
	if domains[0] != "ads.example.com" || domains[1] != "tracker.example.com" {
		t.Errorf("domains = %v", domains)
	}
}

func TestParseDomainList(t *testing.T) {
	content := `# comment
ads.example.com
tracker.example.com

`
	domains := parseDomainList([]byte(content))
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}
}

func TestManagerClassify(t *testing.T) {
	m := NewManager(nil)
	m.domains.Store(newDomainMap(map[string]string{
		"ads.example.com":     "advertising",
		"tracker.example.com": "tracking",
	}))

	tests := []struct {
		domain string
		want   string
	}{
		{"ads.example.com.", "advertising"},
		{"tracker.example.com.", "tracking"},
		{"clean.example.com.", ""},
		{"sub.ads.example.com.", ""},
	}
	for _, tt := range tests {
		got := m.Classify(tt.domain)
		if got != tt.want {
			t.Errorf("Classify(%q) = %q, want %q", tt.domain, got, tt.want)
		}
	}
}

func TestManagerUncategorized(t *testing.T) {
	m := NewManager(nil)
	m.domains.Store(newDomainMap(map[string]string{
		"mystery.example.com": "uncategorized",
	}))

	got := m.Classify("mystery.example.com.")
	if got != "uncategorized" {
		t.Errorf("Classify uncategorized domain = %q, want uncategorized", got)
	}
}

func TestManagerOverrides(t *testing.T) {
	m := NewManager(nil)
	m.domains.Store(newDomainMap(map[string]string{
		"ads.example.com": "advertising",
	}))
	if err := m.SetOverride("ads.example.com", "telemetry"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	got := m.Classify("ads.example.com.")
	if got != "telemetry" {
		t.Errorf("Classify with override = %q, want telemetry", got)
	}
}

func TestParseAndValidateListURLRejectsLocalhost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := ParseAndValidateListURL(ctx, "http://localhost/list.txt"); err == nil {
		t.Fatal("expected localhost url to be rejected")
	}
}

func TestParseAndValidateListURLRejectsPrivateIP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := ParseAndValidateListURL(ctx, "http://192.168.1.10/list.txt"); err == nil {
		t.Fatal("expected private ip url to be rejected")
	}
}

func TestRefreshKeepsExistingDomainsWhenAllSourcesFail(t *testing.T) {
	m := NewManager(nil)
	m.domains.Store(newDomainMap(map[string]string{
		"ads.example.com": "tracking",
	}))

	err := m.Refresh(context.Background(), []ListSource{
		{
			URL:      "ftp://example.com/list.txt",
			Name:     "broken",
			Category: "tracking",
		},
	})
	if err == nil {
		t.Fatal("Refresh error = nil, want error")
	}
	if got := m.Classify("ads.example.com."); got != "tracking" {
		t.Fatalf("Classify after failed refresh = %q, want %q", got, "tracking")
	}
}

func TestManagerRemoveOverride(t *testing.T) {
	m := NewManager(nil)
	if err := m.SetOverride("ads.example.com", "tracking"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := m.RemoveOverride("ads.example.com"); err != nil {
		t.Fatalf("RemoveOverride: %v", err)
	}
	if got := m.Classify("ads.example.com."); got != "" {
		t.Fatalf("Classify after RemoveOverride = %q, want empty", got)
	}
}

func TestManagerLoadOverridesFromStore(t *testing.T) {
	db := testDB(t)
	if err := db.SetDomainOverride("ads.example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	m := NewManager(db)
	if err := m.LoadOverrides(); err != nil {
		t.Fatalf("LoadOverrides: %v", err)
	}
	if got := m.Classify("ads.example.com."); got != "tracking" {
		t.Fatalf("Classify override = %q, want tracking", got)
	}
}

func TestManagerLoadFromCache(t *testing.T) {
	db := testDB(t)
	listID, err := db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := db.WriteListDomains(listID, map[string]string{
		"ads.example.com": "tracking",
		"metrics.example.com": "analytics",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

	m := NewManager(db)
	count, err := m.LoadFromCache()
	if err != nil {
		t.Fatalf("LoadFromCache: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if got := m.Classify("ads.example.com."); got != "tracking" {
		t.Fatalf("Classify ads = %q, want tracking", got)
	}
}

func TestManagerRefreshIntervalReadsConfig(t *testing.T) {
	db := testDB(t)
	if err := db.SetConfig("list_refresh_hours", "6"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	m := NewManager(db)
	if got := m.refreshInterval(24 * time.Hour); got != 6*time.Hour {
		t.Fatalf("refreshInterval = %s, want 6h", got)
	}
}

func TestManagerLoadSourcesFromDBReturnsEnabledOnly(t *testing.T) {
	db := testDB(t)
	firstID, err := db.AddList("https://example.com/one.txt", "One", "tracking")
	if err != nil {
		t.Fatalf("AddList(one): %v", err)
	}
	secondID, err := db.AddList("https://example.com/two.txt", "Two", "analytics")
	if err != nil {
		t.Fatalf("AddList(two): %v", err)
	}
	if err := db.UpdateListEnabled(secondID, false); err != nil {
		t.Fatalf("UpdateListEnabled: %v", err)
	}

	m := NewManager(db)
	sources := m.loadSourcesFromDB()
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].ID != firstID || sources[0].Name != "One" {
		t.Fatalf("sources[0] = %#v, want enabled list only", sources[0])
	}
}

func TestManagerNotifyConfigChangedDoesNotBlock(t *testing.T) {
	m := NewManager(nil)
	m.NotifyConfigChanged()
	m.NotifyConfigChanged()
}

func TestManagerRunReturnsWhenContextCancelled(t *testing.T) {
	m := NewManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		m.Run(ctx, nil, time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestManagerRunHandlesWakeSignal(t *testing.T) {
	m := NewManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		m.Run(ctx, nil, time.Millisecond)
		close(done)
	}()

	m.NotifyConfigChanged()
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not stop")
	}
}

func testDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
