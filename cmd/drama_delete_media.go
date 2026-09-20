package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var dramaDeleteMediaCmd = &cobra.Command{
	Use:   "delete-media <media_id>",
	Short: "删除指定媒资（deleteMedia，不可恢复，不做二次确认）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mediaID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("media_id 必须是数字: %w", err)
		}

		ctx := cmd.Context()

		client, token, err := authedClient(ctx)
		if err != nil {
			return err
		}

		if err := client.DeleteMedia(ctx, token, mediaID); err != nil {
			return fmt.Errorf("delete media: %w", err)
		}

		return nil
	},
}
