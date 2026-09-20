// Package cmd wires weappctl's cobra command tree.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xsh/weappctl/internal/config"
	"github.com/xsh/weappctl/internal/weixin"
)

var (
	cfgPath        string
	profileName    string
	appIDOverride  string
	secretOverride string
)

// version is the build version reported by --version and in the MCP
// handshake; release builds inject it via -ldflags (see justfile).
var version = "dev"

var rootCmd = &cobra.Command{
	Use:           "weappctl",
	Short:         "微信小程序服务端 API 命令行工具",
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: false,
}

// Execute runs the root command; call from main.
func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		os.Exit(1)
	}
}

func init() {
	defaultCfgPath, err := config.DefaultConfigPath()
	if err != nil {
		// Home directory is unresolvable; fall back to a relative default
		// so --config can still be used to point at a real file.
		defaultCfgPath = ".weappctl/config.yaml"
	}

	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", defaultCfgPath, "配置文件路径")
	rootCmd.PersistentFlags().StringVar(&profileName, "profile", "default", "使用的配置 profile 名称")
	rootCmd.PersistentFlags().StringVar(&appIDOverride, "appid", "", "覆盖 profile 中的 appid")
	rootCmd.PersistentFlags().StringVar(&secretOverride, "secret", "", "覆盖 profile 中的 secret")

	rootCmd.AddCommand(dramaCmd)
	rootCmd.AddCommand(serveCmd)
}

// resolveCredentials layers config file < WEAPP_APPID/WEAPP_SECRET env vars
// < --appid/--secret flags, per CONTEXT.md's Profile decision.
func resolveCredentials() (appid, secret string, err error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return "", "", err
	}

	if p, ok := cfg.Profile(profileName); ok {
		appid, secret = p.AppID, p.Secret
	}
	if v := os.Getenv("WEAPP_APPID"); v != "" {
		appid = v
	}
	if v := os.Getenv("WEAPP_SECRET"); v != "" {
		secret = v
	}
	if appIDOverride != "" {
		appid = appIDOverride
	}
	if secretOverride != "" {
		secret = secretOverride
	}

	if appid == "" || secret == "" {
		return "", "", fmt.Errorf(
			"未找到 profile %q 的 appid/secret（配置文件：%s）；"+
				"可通过 --profile 指定已配置的 profile，或用 --appid/--secret/WEAPP_APPID/WEAPP_SECRET 提供",
			profileName, cfgPath,
		)
	}
	return appid, secret, nil
}

// tokenManager builds the access-token manager for the active profile. Both
// the CLI commands and `weappctl serve` go through here, so the credential
// resolution and token cache wiring exist in exactly one place.
func tokenManager() (*weixin.TokenManager, error) {
	appid, secret, err := resolveCredentials()
	if err != nil {
		return nil, err
	}

	cacheDir, err := config.DefaultCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve token cache dir: %w", err)
	}

	return &weixin.TokenManager{
		Client: weixin.NewClient(),
		Cache:  weixin.NewTokenCache(cacheDir),
		AppID:  appid,
		Secret: secret,
	}, nil
}

// authedClient builds a weixin.Client plus a valid access_token for the
// active profile, sharing the credential-resolution/token-cache setup
// across every drama subcommand.
func authedClient(ctx context.Context) (*weixin.Client, string, error) {
	tm, err := tokenManager()
	if err != nil {
		return nil, "", err
	}

	token, err := tm.AccessToken(ctx, profileName)
	if err != nil {
		return nil, "", fmt.Errorf("get access_token: %w", err)
	}
	return tm.Client, token, nil
}
