package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Listen != "0.0.0.0:53" {
		t.Errorf("Listen = %q, want 0.0.0.0:53", cfg.Listen)
	}
	if len(cfg.Upstream) != 2 {
		t.Errorf("Upstream count = %d, want 2", len(cfg.Upstream))
	}
	if cfg.DataDir != "/data" {
		t.Errorf("DataDir = %q, want /data", cfg.DataDir)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
}

func TestLoadMissing(t *testing.T) {
	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	if cfg.Listen != "0.0.0.0:53" {
		t.Errorf("missing file should return defaults, got Listen = %q", cfg.Listen)
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	err := os.WriteFile(path, []byte(`
listen = "127.0.0.1:5353"
upstream = ["9.9.9.9:53"]
data_dir = "/tmp/scrye"
http_port = 9090
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load valid file: %v", err)
	}
	if cfg.Listen != "127.0.0.1:5353" {
		t.Errorf("Listen = %q, want 127.0.0.1:5353", cfg.Listen)
	}
	if len(cfg.Upstream) != 1 || cfg.Upstream[0] != "9.9.9.9:53" {
		t.Errorf("Upstream = %v, want [9.9.9.9:53]", cfg.Upstream)
	}
	if cfg.HTTPPort != 9090 {
		t.Errorf("HTTPPort = %d, want 9090", cfg.HTTPPort)
	}
}

func TestLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		content string
	}{
		{"empty upstream", `upstream = []`},
		{"bad port", `http_port = 0`},
		{"port too high", `http_port = 70000`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".toml")
			os.WriteFile(path, []byte(tt.content), 0644)
			_, err := Load(path)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
