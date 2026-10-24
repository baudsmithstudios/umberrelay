package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatalf("Load missing file: %v", err)
	}
	want := Config{
		Listen:     "0.0.0.0:53",
		Upstream:   []string{"1.1.1.1:53", "8.8.8.8:53"},
		DataDir:    "/data",
		HTTPListen: "0.0.0.0",
		HTTPPort:   8080,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("cfg = %+v, want %+v", cfg, want)
	}
}

func TestLoadValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	err := os.WriteFile(path, []byte(`
listen = "127.0.0.1:5353"
upstream = ["9.9.9.9:53"]
data_dir = "/tmp/umberrelay"
http_listen = "127.0.0.1"
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
	if cfg.HTTPListen != "127.0.0.1" {
		t.Errorf("HTTPListen = %q, want 127.0.0.1", cfg.HTTPListen)
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
		{"empty http listen", `http_listen = ""`},
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
