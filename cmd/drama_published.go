package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xsh/weappctl/internal/dramaquery"
	"github.com/xsh/weappctl/internal/mcpserver"
)

var dramaPublishedMaxItems int

var dramaPublishedCmd = &cobra.Command{
	Use:   "published",
	Short: "获取已上架短剧（developerGetPublishedDrama）",
	Long: `获取当前已上架的短剧清单与数量，输出为信封：matched 就是"有多少短剧上架"的答案。

"上架"指显式执行过短剧上架动作，与"审核通过"是两回事：审核通过（drama list --state approved）只代表有资格上架。
回答上架数量请用本命令。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		if err := dramaquery.ValidateMaxItems(dramaPublishedMaxItems); err != nil {
			return err
		}

		client, token, err := authedClient(ctx)
		if err != nil {
			return err
		}

		pubs, err := client.GetPublishedDramas(ctx, token)
		if err != nil {
			return fmt.Errorf("get published dramas: %w", err)
		}

		env, err := dramaquery.BuildPublished(pubs, dramaPublishedMaxItems)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	},
}

func init() {
	dramaPublishedCmd.Flags().IntVar(&dramaPublishedMaxItems, "max-items", mcpserver.DefaultMaxItems,
		"最多返回多少条，0 表示不限（matched 始终是真实总数）")
}
