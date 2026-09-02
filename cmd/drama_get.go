package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

var dramaGetCmd = &cobra.Command{
	Use:   "get <drama_id>",
	Short: "获取剧目信息（getDrama）",
	Args:  cobra.ExactArgs(1),
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

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	},
}
