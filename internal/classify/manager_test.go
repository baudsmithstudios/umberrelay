package classify

import (
	"context"
	"testing"
	"time"
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
	m.SetOverride("ads.example.com", "telemetry")

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
