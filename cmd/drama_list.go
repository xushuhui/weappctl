package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xsh/weappctl/internal/dramaquery"
	"github.com/xsh/weappctl/internal/mcpserver"
	"github.com/xsh/weappctl/internal/weixin"
)

// listPageSize is the page size used when auto-paginating (no explicit
// --offset/--limit) and the default --limit when a user opts into manual
// paging without setting one. WeChat caps limit at 100.
const listPageSize = 100

var (
	dramaListOffset   int
	dramaListLimit    int
	dramaListStates   []string
	dramaListFields   string
	dramaListMaxItems int
)

var dramaListCmd = &cobra.Command{
	Use:   "list",
	Short: "获取剧目列表（listDramas，全部提审记录，不限上架状态）",
	Long: `获取剧目列表，输出为信封：matched（过滤后总数）、counts（全量统计，不受 --state 影响）、
dramas（默认只含 ` + strings.Join(dramaquery.DefaultFields, ",") + `，最多 ` + fmt.Sprint(mcpserver.DefaultMaxItems) + ` 条）。

审核阶段看 audit_status：in-review 审核中 / returned 退回修改 / rejected 终审拒绝 / approved 审核通过（只代表有资格上架）。
taken_down 是平台下架标记，与审核阶段是两个独立维度，一部剧可以同时是 approved 且 taken_down。
原始 status、audit_detail、description 等字段随时可用 --fields 取回。

要查"有多少短剧上架"请用 weappctl drama published——审核通过不等于已上架。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		states, err := dramaquery.ParseStates(dramaListStates)
		if err != nil {
			return err
		}
		fields, err := dramaquery.ParseFields(dramaListFields)
		if err != nil {
			return err
		}
		if err := dramaquery.ValidateMaxItems(dramaListMaxItems); err != nil {
			return err
		}

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
			dramas, err = dramaquery.FetchAllDramas(ctx, client, token)
			if err != nil {
				return err
			}
		}

		env, err := dramaquery.Build(dramas, states, fields, dramaListMaxItems)
		if err != nil {
			return err
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	},
}

func init() {
	dramaListCmd.Flags().IntVar(&dramaListOffset, "offset", 0, "起始偏移量（传了此项或 --limit 即关闭自动翻页，只查这一页）")
	dramaListCmd.Flags().IntVar(&dramaListLimit, "limit", listPageSize, "单页返回数量，最大 100（传了此项或 --offset 即关闭自动翻页，只查这一页）")
	dramaListCmd.Flags().StringArrayVar(&dramaListStates, "state", nil,
		"按状态过滤，可重复（OR）："+stateWords())
	dramaListCmd.Flags().StringVar(&dramaListFields, "fields", "",
		"逗号分隔的输出字段，默认 "+strings.Join(dramaquery.DefaultFields, ",")+"；可选："+strings.Join(dramaquery.FieldNames(), ", "))
	dramaListCmd.Flags().IntVar(&dramaListMaxItems, "max-items", mcpserver.DefaultMaxItems,
		"最多返回多少条，0 表示不限")
}

func stateWords() string {
	words := make([]string, 0, len(dramaquery.States))
	for _, s := range dramaquery.States {
		words = append(words, string(s))
	}
	return strings.Join(words, "/")
}
