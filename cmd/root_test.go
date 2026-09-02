package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCredentialsPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(`
profiles:
  default:
    appid: cfg-appid
    secret: cfg-secret
`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Reset package-level flag vars and restore them after the test so
	// other tests aren't affected by leftover state.
	origCfgPath, origProfile, origAppID, origSecret := cfgPath, profileName, appIDOverride, secretOverride
	t.Cleanup(func() {
		cfgPath, profileName, appIDOverride, secretOverride = origCfgPath, origProfile, origAppID, origSecret
	})

	cfgPath = cfgFile
	profileName = "default"
	appIDOverride = ""
	secretOverride = ""

	t.Run("config file only", func(t *testing.T) {
		appid, secret, err := resolveCredentials()
		if err != nil {
			t.Fatalf("resolveCredentials() error = %v", err)
		}
		if appid != "cfg-appid" || secret != "cfg-secret" {
			t.Fatalf("got %q/%q, want cfg-appid/cfg-secret", appid, secret)
		}
	})

	t.Run("env overrides config file", func(t *testing.T) {
		t.Setenv("WEAPP_APPID", "env-appid")
		t.Setenv("WEAPP_SECRET", "env-secret")
		appid, secret, err := resolveCredentials()
		if err != nil {
			t.Fatalf("resolveCredentials() error = %v", err)
		}
		if appid != "env-appid" || secret != "env-secret" {
			t.Fatalf("got %q/%q, want env-appid/env-secret", appid, secret)
		}
	})

	t.Run("flag overrides env and config file", func(t *testing.T) {
		t.Setenv("WEAPP_APPID", "env-appid")
		t.Setenv("WEAPP_SECRET", "env-secret")
		appIDOverride = "flag-appid"
		secretOverride = "flag-secret"
		t.Cleanup(func() { appIDOverride, secretOverride = "", "" })

		appid, secret, err := resolveCredentials()
		if err != nil {
			t.Fatalf("resolveCredentials() error = %v", err)
		}
		if appid != "flag-appid" || secret != "flag-secret" {
			t.Fatalf("got %q/%q, want flag-appid/flag-secret", appid, secret)
		}
	})

	t.Run("unknown profile with no overrides errors", func(t *testing.T) {
		profileName = "missing-profile"
		t.Cleanup(func() { profileName = "default" })

		if _, _, err := resolveCredentials(); err == nil {
			t.Fatalf("resolveCredentials() error = nil, want error for unknown profile")
		}
	})
}
