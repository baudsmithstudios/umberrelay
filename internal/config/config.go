package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config holds bootstrap configuration loaded from TOML.
type Config struct {
	Listen   string   `toml:"listen"`
	Upstream []string `toml:"upstream"`
	DataDir  string   `toml:"data_dir"`
	HTTPPort int      `toml:"http_port"`
}

// Defaults returns a Config with sane defaults.
func Defaults() Config {
	return Config{
		Listen:   "0.0.0.0:53",
		Upstream: []string{"1.1.1.1:53", "8.8.8.8:53"},
		DataDir:  "/data",
		HTTPPort: 8080,
	}
}

// Load reads a TOML config file, falling back to defaults for missing fields.
func Load(path string) (Config, error) {
	cfg := Defaults()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()

	if _, err := toml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if len(c.Upstream) == 0 {
		return fmt.Errorf("upstream must have at least one DNS server")
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("http_port must be 1-65535, got %d", c.HTTPPort)
	}
	return nil
}
