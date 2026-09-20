package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xsh/weappctl/internal/config"
	"github.com/xsh/weappctl/internal/mcpserver"
)

var (
	serveListen   string
	serveTokens   string
	serveDailyCap int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "以内网只读 MCP 服务的形式提供短剧查询（不含删除能力）",
	Long: `以只读 MCP 服务的形式提供短剧查询，供同事的 agent（如 Codex）直接提问。

只暴露 3 个只读工具：list_dramas、published_dramas、get_drama。
delete-media 不会出现在这里——agent 侧根本不该有这把刀（见 docs/adr/0002）。

部署要点：
  * 默认只监听 127.0.0.1:8787；要让同事访问，必须显式 --listen <内网IP>:8787。
  * 每个同事一个 bearer token（--tokens 指向的名单文件），可单独吊销。
  * appid/secret 只留在服务端：用 --profile/--config 或 WEAPP_APPID/WEAPP_SECRET 提供。
  * 服务走明文 HTTP，只应部署在内网/VPN 可达的机器上，切勿做公网端口映射。
  * 出网需要能访问 api.weixin.qq.com；需要代理时设置 HTTPS_PROXY。

同事那一侧的 Codex 配置见 docs/colleague-setup.md。`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		tm, err := tokenManager()
		if err != nil {
			return err
		}

		tokens, err := mcpserver.LoadTokenStore(serveTokens)
		if err != nil {
			return err
		}

		srv, err := mcpserver.New(mcpserver.Options{
			Client:         tm.Client,
			AccessToken:    func(ctx context.Context) (string, error) { return tm.AccessToken(ctx, profileName) },
			Tokens:         tokens,
			DailyCallLimit: serveDailyCap,
			Version:        version,
		})
		if err != nil {
			return err
		}

		httpSrv := &http.Server{
			Addr:              serveListen,
			Handler:           srv.Handler(),
			ReadHeaderTimeout: 10 * time.Second,
		}

		errCh := make(chan error, 1)
		go func() {
			cmd.PrintErrf("weappctl serve: 监听 %s（profile %s），只读工具 list_dramas / published_dramas / get_drama\n",
				serveListen, profileName)
			if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()

		select {
		case err := <-errCh:
			return err
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return httpSrv.Shutdown(shutdownCtx)
		}
	},
}

func init() {
	tokensPath := filepath.Join(".weappctl", "tokens.yaml")
	if dir, err := config.DefaultDir(); err == nil {
		tokensPath = filepath.Join(dir, "tokens.yaml")
	}

	serveCmd.Flags().StringVar(&serveListen, "listen", "127.0.0.1:8787",
		"监听地址；默认只监听本机，要对同事开放必须显式写内网 IP")
	serveCmd.Flags().StringVar(&serveTokens, "tokens", tokensPath,
		"bearer token 名单文件（YAML：tokens: [{name, token}]）")
	serveCmd.Flags().IntVar(&serveDailyCap, "daily-call-limit", mcpserver.DefaultDailyCallLimit,
		"每人每天的调用上限，0 表示不限")
}
