package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var dramaPublishedCmd = &cobra.Command{
	Use:   "published",
	Short: "获取已上架短剧（developerGetPublishedDrama）",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		client, token, err := authedClient(ctx)
		if err != nil {
			return err
		}

		dramas, err := client.GetPublishedDramas(ctx, token)
		if err != nil {
			return fmt.Errorf("get published dramas: %w", err)
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(dramas)
	},
}
