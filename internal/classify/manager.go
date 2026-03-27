package classify

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"scrye/internal/store"
)

// domainMap is a read-only map of domain -> category.
type domainMap struct {
	m map[string]string
}

func newDomainMap(m map[string]string) *domainMap {
	return &domainMap{m: m}
}

// Manager fetches and caches classification lists.
type Manager struct {
	db        *store.DB
	domains   atomic.Pointer[domainMap]
	overrides sync.Map // domain -> category
}

// NewManager creates a classification manager.
func NewManager(db *store.DB) *Manager {
	m := &Manager{db: db}
	m.domains.Store(newDomainMap(make(map[string]string)))
	return m
}

// Classify returns the category for a domain, or empty string if unclassified.
// Domain should include trailing dot (DNS format); it is stripped before lookup.
func (m *Manager) Classify(domain string) string {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")

	// Check overrides first
	if cat, ok := m.overrides.Load(domain); ok {
		return cat.(string)
	}

	dm := m.domains.Load()
	if dm == nil {
		return ""
	}
	return dm.m[domain]
}

// SetOverride adds a user-defined classification override and persists it.
func (m *Manager) SetOverride(domain, category string) {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	m.overrides.Store(domain, category)
	if m.db != nil {
		m.db.SetDomainOverride(domain, category)
	}
}

// RemoveOverride removes a user-defined classification override and deletes it from storage.
func (m *Manager) RemoveOverride(domain string) {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	m.overrides.Delete(domain)
	if m.db != nil {
		m.db.DeleteDomainOverride(domain)
	}
}

// LoadOverrides loads persisted domain overrides from the store into memory.
func (m *Manager) LoadOverrides() error {
	if m.db == nil {
		return nil
	}
	overrides, err := m.db.ListDomainOverrides()
	if err != nil {
		return fmt.Errorf("load overrides: %w", err)
	}
	for domain, category := range overrides {
		m.overrides.Store(domain, category)
	}
	return nil
}

// LoadFromCache loads the domain lookup from the SQLite list_domains cache.
// Returns the number of domains loaded, or 0 if no cache exists.
func (m *Manager) LoadFromCache() (int, error) {
	if m.db == nil {
		return 0, nil
	}
	cached, err := m.db.LoadCachedDomains()
	if err != nil {
		return 0, fmt.Errorf("load cached domains: %w", err)
	}
	if len(cached) == 0 {
		return 0, nil
	}
	m.domains.Store(newDomainMap(cached))
	return len(cached), nil
}

// ListSource represents a classification list.
type ListSource struct {
	ID       int64
	URL      string
	Name     string
	Category string
}

// Refresh fetches all enabled lists, rebuilds the in-memory lookup, and updates the cache.
func (m *Manager) Refresh(ctx context.Context, sources []ListSource) error {
	combined := make(map[string]string)
	for _, src := range sources {
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		domains, err := fetchList(fetchCtx, src.URL)
		cancel()
		if err != nil {
			log.Printf("fetch list %s: %v", src.Name, err)
			continue
		}
		listDomains := make(map[string]string, len(domains))
		for _, d := range domains {
			cat := src.Category
			if cat == "" {
				cat = "uncategorized"
			}
			combined[d] = cat
			listDomains[d] = cat
		}
		// Cache to SQLite
		if m.db != nil && src.ID > 0 {
			if err := m.db.WriteListDomains(src.ID, listDomains); err != nil {
				log.Printf("cache list %s: %v", src.Name, err)
			}
			m.db.UpdateListFetchTime(src.ID)
		}
		log.Printf("loaded %s: %d domains", src.Name, len(domains))
	}
	m.domains.Store(newDomainMap(combined))
	log.Printf("classification database: %d domains total", len(combined))
	return nil
}

// Run starts the periodic refresh loop. On each cycle, it re-reads the
// enabled list sources from the database so that user-added/removed lists
// take effect without a restart. The initial sources are used for the
// first refresh only.
func (m *Manager) Run(ctx context.Context, initialSources []ListSource, interval time.Duration) {
	m.Refresh(ctx, initialSources)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sources := m.loadSourcesFromDB()
			if len(sources) == 0 {
				sources = initialSources
			}
			m.Refresh(ctx, sources)
		}
	}
}

func (m *Manager) loadSourcesFromDB() []ListSource {
	if m.db == nil {
		return nil
	}
	lists, err := m.db.ListLists()
	if err != nil {
		log.Printf("load list sources from db: %v", err)
		return nil
	}
	var sources []ListSource
	for _, l := range lists {
		if l.Enabled {
			sources = append(sources, ListSource{
				ID:       l.ID,
				URL:      l.URL,
				Name:     l.Name,
				Category: l.Category,
			})
		}
	}
	return sources
}

func fetchList(ctx context.Context, url string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Try hosts file format first, fall back to domain list
	domains := parseHostsFile(body)
	if len(domains) == 0 {
		domains = parseDomainList(body)
	}
	return domains, nil
}

func parseHostsFile(data []byte) []string {
	var domains []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := fields[0]
		if ip != "0.0.0.0" && ip != "127.0.0.1" {
			continue
		}
		domain := strings.ToLower(fields[1])
		if domain == "localhost" || domain == "localhost.localdomain" {
			continue
		}
		domains = append(domains, domain)
	}
	return domains
}

func parseDomainList(data []byte) []string {
	var domains []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		domains = append(domains, strings.ToLower(line))
	}
	return domains
}
