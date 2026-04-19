package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Listen     string   `toml:"listen"`
	Upstream   []string `toml:"upstream"`
	DataDir    string   `toml:"data_dir"`
	HTTPListen string   `toml:"http_listen"`
	HTTPPort   int      `toml:"http_port"`
}

func Defaults() Config {
	return Config{
		Listen:     "0.0.0.0:53",
		Upstream:   []string{"1.1.1.1:53", "8.8.8.8:53"},
		DataDir:    "/data",
		HTTPListen: "0.0.0.0",
		HTTPPort:   8080,
	}
}

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
	if strings.TrimSpace(c.HTTPListen) == "" {
		return fmt.Errorf("http_listen must not be empty")
	}
	return nil
}
