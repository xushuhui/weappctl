package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingFileYieldsEmptyProfiles(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("Profiles = %v, want empty", cfg.Profiles)
	}
	if _, ok := cfg.Profile("default"); ok {
		t.Fatalf("Profile(\"default\") found on empty config, want not found")
	}
}

func TestLoadParsesNamedProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, `
profiles:
  default:
    appid: wx-default
    secret: secret-default
  staging:
    appid: wx-staging
    secret: secret-staging
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	def, ok := cfg.Profile("default")
	if !ok || def.AppID != "wx-default" || def.Secret != "secret-default" {
		t.Fatalf("Profile(\"default\") = %+v, %v; want wx-default/secret-default, true", def, ok)
	}

	staging, ok := cfg.Profile("staging")
	if !ok || staging.AppID != "wx-staging" || staging.Secret != "secret-staging" {
		t.Fatalf("Profile(\"staging\") = %+v, %v; want wx-staging/secret-staging, true", staging, ok)
	}

	if _, ok := cfg.Profile("missing"); ok {
		t.Fatalf("Profile(\"missing\") found, want not found")
	}
}

func TestLoadRejectsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile(t, path, "profiles: [this is not a map]")

	if _, err := Load(path); err == nil {
		t.Fatalf("Load() error = nil, want parse error")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := writeFileErr(path, content); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
