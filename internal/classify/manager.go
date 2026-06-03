package classify

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	urlpkg "net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"umberrelay/internal/store"
)

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
	wake      chan struct{}
	refresh   func(context.Context, []ListSource) error
}

func NewManager(db *store.DB) *Manager {
	m := &Manager{
		db:   db,
		wake: make(chan struct{}, 1),
	}
	m.domains.Store(newDomainMap(make(map[string]string)))
	m.refresh = m.Refresh
	return m
}

// Classify returns the category for a domain, or empty string if unclassified.
// Domain should include trailing dot (DNS format); it is stripped before lookup.
func (m *Manager) Classify(domain string) string {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")

	if cat, ok := m.overrides.Load(domain); ok {
		return cat.(string)
	}

	dm := m.domains.Load()
	if dm == nil {
		return ""
	}
	return dm.m[domain]
}

func (m *Manager) SetOverride(domain, category string) error {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	if m.db != nil {
		if err := m.db.SetDomainOverride(domain, category); err != nil {
			return err
		}
	}
	m.overrides.Store(domain, category)
	return nil
}

func (m *Manager) RemoveOverride(domain string) error {
	domain = strings.TrimSuffix(strings.ToLower(domain), ".")
	if m.db != nil {
		if err := m.db.DeleteDomainOverride(domain); err != nil {
			return err
		}
	}
	m.overrides.Delete(domain)
	return nil
}

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

type ListSource struct {
	ID       int64
	URL      string
	Name     string
	Category string
}

func SourcesFromListEntries(entries []store.ListEntry) []ListSource {
	sources := make([]ListSource, 0, len(entries))
	for _, entry := range entries {
		sources = append(sources, ListSource{
			ID:       entry.ID,
			URL:      entry.URL,
			Name:     entry.Name,
			Category: entry.Category,
		})
	}
	return sources
}

func (m *Manager) Refresh(ctx context.Context, sources []ListSource) error {
	attemptedAt := time.Now().UTC()
	var refreshErr error
	defer func() {
		if m.db == nil {
			return
		}
		if err := m.db.RecordListRefreshAttempt(attemptedAt, refreshErr); err != nil {
			log.Printf("record list refresh status: %v", err)
		}
	}()

	combined := make(map[string]string)
	successCount := 0
	for _, src := range sources {
		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		domains, err := fetchList(fetchCtx, src.URL)
		cancel()
		if err != nil {
			log.Printf("fetch list %s: %v", src.Name, err)
			continue
		}
		successCount++
		listDomains := make(map[string]string, len(domains))
		for _, d := range domains {
			cat := src.Category
			if cat == "" {
				cat = "uncategorized"
			}
			combined[d] = cat
			listDomains[d] = cat
		}
		if m.db != nil && src.ID > 0 {
			if err := m.db.WriteListDomains(src.ID, listDomains); err != nil {
				log.Printf("cache list %s: %v", src.Name, err)
			}
			m.db.UpdateListFetchTime(src.ID)
		}
		log.Printf("loaded %s: %d domains", src.Name, len(domains))
	}
	if len(sources) > 0 && successCount == 0 {
		refreshErr = fmt.Errorf("all list refreshes failed")
		return refreshErr
	}
	m.domains.Store(newDomainMap(combined))
	log.Printf("classification database: %d domains total", len(combined))
	return nil
}

func (m *Manager) NotifyConfigChanged() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) Run(ctx context.Context, initialSources []ListSource, interval time.Duration) {
	for {
		timer := time.NewTimer(m.refreshInterval(interval))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.wake:
			timer.Stop()
			continue
		case <-timer.C:
			sources, err := m.loadSourcesFromDB()
			if err != nil {
				log.Printf("load list sources from db: %v", err)
				sources = initialSources
			}
			if err := m.refresh(ctx, sources); err != nil {
				log.Printf("refresh lists: %v", err)
			}
		}
	}
}

func (m *Manager) refreshInterval(defaultInterval time.Duration) time.Duration {
	if m.db == nil {
		return defaultInterval
	}
	val, err := m.db.GetConfig("list_refresh_hours")
	if err != nil || val == "" {
		return defaultInterval
	}
	n, err := time.ParseDuration(val + "h")
	if err != nil || n <= 0 {
		return defaultInterval
	}
	return n
}

func (m *Manager) loadSourcesFromDB() ([]ListSource, error) {
	if m.db == nil {
		return nil, nil
	}
	lists, err := m.db.ListEnabledLists()
	if err != nil {
		return nil, err
	}
	return SourcesFromListEntries(lists), nil
}

const maxListBytes int64 = 20 << 20

var (
	listResolverLookupNetIP = net.DefaultResolver.LookupNetIP
	listDialContext         = (&net.Dialer{}).DialContext
)

func ParseAndValidateListURL(ctx context.Context, rawURL string) (*urlpkg.URL, error) {
	parsedURL, err := urlpkg.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse url %q: %w", rawURL, err)
	}
	if err := validateListURL(ctx, parsedURL); err != nil {
		return nil, err
	}
	return parsedURL, nil
}

func fetchList(ctx context.Context, rawURL string) ([]string, error) {
	parsedURL, err := ParseAndValidateListURL(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default http transport type unsupported")
	}
	transport := baseTransport.Clone()
	transport.DialContext = dialValidatedRemote
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return validateListURL(req.Context(), req.URL)
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxListBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxListBytes {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", rawURL, maxListBytes)
	}
	// Try hosts file format first, fall back to domain list
	domains := parseHostsFile(body)
	if len(domains) == 0 {
		domains = parseDomainList(body)
	}
	return domains, nil
}

func dialValidatedRemote(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	var addrs []netip.Addr
	if parsedAddr, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{parsedAddr}
	} else {
		addrs, err = listResolverLookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
	}

	var firstDialErr error
	for _, addr := range addrs {
		addr = addr.Unmap()
		if !isAllowedRemoteAddr(addr) {
			continue
		}
		conn, err := listDialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		if firstDialErr == nil {
			firstDialErr = err
		}
	}

	if firstDialErr != nil {
		return nil, firstDialErr
	}
	return nil, fmt.Errorf("host %s resolves to a non-public address", host)
}

func validateListURL(ctx context.Context, u *urlpkg.URL) error {
	if u == nil {
		return fmt.Errorf("missing url")
	}
	if u.User != nil {
		return fmt.Errorf("list urls must not include credentials")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("list url must include a host")
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("loopback hosts are not allowed")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isAllowedRemoteAddr(addr) {
			return fmt.Errorf("host %s resolves to a non-public address", host)
		}
		return nil
	}

	addrs, err := listResolverLookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolve %s: no addresses found", host)
	}
	for _, addr := range addrs {
		if !isAllowedRemoteAddr(addr) {
			return fmt.Errorf("host %s resolves to a non-public address", host)
		}
	}
	return nil
}

func isAllowedRemoteAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() {
		return false
	}
	return addr.IsGlobalUnicast()
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
