package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/xsh/weappctl/internal/dramaquery"
)

var dramaGetCmd = &cobra.Command{
	Use:   "get <drama_id>",
	Short: "获取剧目信息（getDrama）",
	Long: `获取单部剧的完整原始信息，并附加两个解释性字段：
  audit_status  权威审核阶段（in-review/returned/rejected/approved/invalid）
  taken_down    是否被平台下架

原始 status 字段是粗粒度的可播标记，微信控制台的口径与它不一致，判断审核阶段请以 audit_status 为准。`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dramaID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("drama_id 必须是数字: %w", err)
		}

		ctx := cmd.Context()

		client, token, err := authedClient(ctx)
		if err != nil {
			return err
		}

		info, err := client.GetDrama(ctx, token, dramaID)
		if err != nil {
			return fmt.Errorf("get drama: %w", err)
		}

		record, err := dramaquery.ProjectWithDerived(*info)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(record)
	},
}
