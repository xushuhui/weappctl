package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xsh/weappctl/internal/weixin"
)

// listPageSize is the page size used when auto-paginating (no explicit
// --offset/--limit) and the default --limit when a user opts into manual
// paging without setting one. WeChat caps limit at 100.
const listPageSize = 100

var (
	dramaListOffset int
	dramaListLimit  int
)

var dramaListCmd = &cobra.Command{
	Use:   "list",
	Short: "获取剧目列表（listDramas，全部提审记录，不限上架状态）",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		client, token, err := authedClient(ctx)
		if err != nil {
			return err
		}

		// Explicit --offset/--limit means "just this one page"; otherwise
		// auto-paginate and return everything.
		manualPaging := cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")

		var dramas []weixin.DramaInfo
		if manualPaging {
			dramas, err = client.ListDramas(ctx, token, dramaListOffset, dramaListLimit)
			if err != nil {
				return fmt.Errorf("list dramas: %w", err)
			}
		} else {
			for offset := 0; ; offset += listPageSize {
				page, err := client.ListDramas(ctx, token, offset, listPageSize)
				if err != nil {
					return fmt.Errorf("list dramas: %w", err)
				}
				dramas = append(dramas, page...)
				if len(page) < listPageSize {
					break
				}
			}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(dramas)
	},
}

func init() {
	dramaListCmd.Flags().IntVar(&dramaListOffset, "offset", 0, "起始偏移量（传了此项或 --limit 即关闭自动翻页，只查这一页）")
	dramaListCmd.Flags().IntVar(&dramaListLimit, "limit", listPageSize, "单页返回数量，最大 100（传了此项或 --offset 即关闭自动翻页，只查这一页）")
}
