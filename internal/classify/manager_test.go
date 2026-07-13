package classify

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
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
	m := NewManager(testDB(t))
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
	m := NewManager(testDB(t))
	m.domains.Store(newDomainMap(map[string]string{
		"mystery.example.com": "uncategorized",
	}))

	got := m.Classify("mystery.example.com.")
	if got != "uncategorized" {
		t.Errorf("Classify uncategorized domain = %q, want uncategorized", got)
	}
}

func TestManagerOverrides(t *testing.T) {
	m := NewManager(testDB(t))
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

func TestParseAndValidateListURLRejectsHostResolvingToPrivateAddress(t *testing.T) {
	withListNetworkHooks(t,
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("10.0.0.5")}, nil
		},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := ParseAndValidateListURL(ctx, "https://lists.example.invalid/tracking.txt"); err == nil {
		t.Fatal("expected host resolving to private address to be rejected")
	}
}

func TestDialValidatedRemoteRejectsPrivateLiteralAddress(t *testing.T) {
	var dialCalled bool
	withListNetworkHooks(t, nil, func(context.Context, string, string) (net.Conn, error) {
		dialCalled = true
		return nil, errors.New("dial should not be called")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := dialValidatedRemote(ctx, "tcp", "192.168.1.10:80"); err == nil {
		t.Fatal("expected private literal address to be rejected")
	}
	if dialCalled {
		t.Fatal("dial was called for private literal address")
	}
}

func TestDialValidatedRemoteDialsFirstPublicResolvedAddress(t *testing.T) {
	withListNetworkHooks(t,
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("10.0.0.5"),
				netip.MustParseAddr("8.8.8.8"),
			}, nil
		},
		func(_ context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" {
				return nil, errors.New("unexpected network")
			}
			if address != "8.8.8.8:443" {
				return nil, errors.New("unexpected dial address: " + address)
			}
			clientConn, serverConn := net.Pipe()
			serverConn.Close()
			return clientConn, nil
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, err := dialValidatedRemote(ctx, "tcp", "lists.example.com:443")
	if err != nil {
		t.Fatalf("dialValidatedRemote: %v", err)
	}
	conn.Close()
}

func TestDialValidatedRemoteRejectsWhenOnlyPrivateResolvedAddresses(t *testing.T) {
	var dialCalled bool
	withListNetworkHooks(t,
		func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("127.0.0.1"),
				netip.MustParseAddr("10.0.0.12"),
			}, nil
		},
		func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, errors.New("dial should not be called")
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := dialValidatedRemote(ctx, "tcp", "lists.example.com:80")
	if err == nil {
		t.Fatal("expected error when resolver only returns private addresses")
	}
	if !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("error = %q, want non-public rejection", err.Error())
	}
	if dialCalled {
		t.Fatal("dial was called when resolver only returned private addresses")
	}
}

func TestRefreshKeepsExistingDomainsWhenAllSourcesFail(t *testing.T) {
	m := NewManager(testDB(t))
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
	db := testDB(t)
	m := NewManager(db)
	if err := m.SetOverride("ads.example.com", "tracking"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := m.RemoveOverride("ads.example.com"); err != nil {
		t.Fatalf("RemoveOverride: %v", err)
	}
	if got := m.Classify("ads.example.com."); got != "" {
		t.Fatalf("Classify after RemoveOverride = %q, want empty", got)
	}
	overrides, err := db.ListDomainOverrides()
	if err != nil {
		t.Fatalf("ListDomainOverrides: %v", err)
	}
	if _, ok := overrides["ads.example.com"]; ok {
		t.Fatalf("override still persisted: %#v", overrides)
	}
}

func TestManagerRemoveOverrideReturnsErrorWhenPersistenceFails(t *testing.T) {
	db := testDB(t)
	m := NewManager(db)
	if err := m.SetOverride("ads.example.com", "tracking"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.RemoveOverride("ads.example.com"); err == nil {
		t.Fatal("RemoveOverride() error = nil, want non-nil")
	}
	if got := m.Classify("ads.example.com."); got != "tracking" {
		t.Fatalf("Classify after failed remove = %q, want tracking", got)
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
		"ads.example.com":     "tracking",
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
	sources, err := m.loadSourcesFromDB()
	if err != nil {
		t.Fatalf("loadSourcesFromDB: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].ID != firstID || sources[0].Name != "One" {
		t.Fatalf("sources[0] = %#v, want enabled list only", sources[0])
	}
}

func TestManagerRunDoesNotFallbackToInitialSourcesWhenNoListsEnabled(t *testing.T) {
	db := testDB(t)
	disabledID, err := db.AddList("https://example.com/disabled.txt", "Disabled", "tracking")
	if err != nil {
		t.Fatalf("AddList(disabled): %v", err)
	}
	if err := db.UpdateListEnabled(disabledID, false); err != nil {
		t.Fatalf("UpdateListEnabled: %v", err)
	}

	m := NewManager(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refreshSources := make(chan []ListSource, 1)
	m.refresh = func(_ context.Context, sources []ListSource) error {
		out := append([]ListSource(nil), sources...)
		refreshSources <- out
		cancel()
		return nil
	}

	initial := []ListSource{{ID: 42, URL: "https://example.com/initial.txt", Name: "Initial", Category: "tracking"}}
	go m.Run(ctx, initial, time.Millisecond)

	select {
	case sources := <-refreshSources:
		if len(sources) != 0 {
			t.Fatalf("len(sources) = %d, want 0 when no lists are enabled", len(sources))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not trigger refresh")
	}
}

func TestManagerRunFallsBackToInitialSourcesWhenDBLoadFails(t *testing.T) {
	db := testDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	m := NewManager(db)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refreshSources := make(chan []ListSource, 1)
	m.refresh = func(_ context.Context, sources []ListSource) error {
		out := append([]ListSource(nil), sources...)
		refreshSources <- out
		cancel()
		return nil
	}

	initial := []ListSource{{ID: 7, URL: "https://example.com/initial.txt", Name: "Initial", Category: "tracking"}}
	go m.Run(ctx, initial, time.Millisecond)

	select {
	case sources := <-refreshSources:
		if len(sources) != 1 || sources[0].ID != initial[0].ID {
			t.Fatalf("sources = %#v, want fallback to initial sources", sources)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not trigger refresh")
	}
}

func TestManagerNotifyConfigChangedDoesNotBlock(t *testing.T) {
	m := NewManager(testDB(t))
	m.NotifyConfigChanged()
	m.NotifyConfigChanged()
}

func TestManagerRunReturnsWhenContextCancelled(t *testing.T) {
	m := NewManager(testDB(t))
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
	m := NewManager(testDB(t))
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

func withListNetworkHooks(
	t *testing.T,
	lookup func(context.Context, string, string) ([]netip.Addr, error),
	dial func(context.Context, string, string) (net.Conn, error),
) {
	t.Helper()

	prevLookup := listResolverLookupNetIP
	prevDial := listDialContext

	if lookup != nil {
		listResolverLookupNetIP = lookup
	}
	if dial != nil {
		listDialContext = dial
	}

	t.Cleanup(func() {
		listResolverLookupNetIP = prevLookup
		listDialContext = prevDial
	})
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
