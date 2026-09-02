// Package config manages weappctl's on-disk configuration: a YAML file
// holding one or more named Profiles (appid + secret pairs), plus the
// directory used to cache access tokens per profile.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Profile holds the credentials for one miniprogram account.
type Profile struct {
	AppID  string `yaml:"appid"`
	Secret string `yaml:"secret"`
}

// Config is the parsed contents of the weappctl config file.
type Config struct {
	Profiles map[string]Profile `yaml:"profiles"`
}

// Profile returns the named profile and whether it was found.
func (c *Config) Profile(name string) (Profile, bool) {
	p, ok := c.Profiles[name]
	return p, ok
}

// DefaultDir returns ~/.weappctl, creating no directories.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".weappctl"), nil
}

// DefaultConfigPath returns ~/.weappctl/config.yaml.
func DefaultConfigPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DefaultCacheDir returns ~/.weappctl/cache, the directory access-token
// caches are written to (see docs/adr/0001-cache-access-token-to-local-file.md).
func DefaultCacheDir() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache"), nil
}

// Load reads and parses the config file at path. A missing file is not an
// error: it yields an empty Config so callers can fall back to flag/env
// overrides.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return &cfg, nil
}
