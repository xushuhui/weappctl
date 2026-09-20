package cmd

import "github.com/spf13/cobra"

var dramaCmd = &cobra.Command{
	Use:   "drama",
	Short: "短剧相关接口",
}

func init() {
	dramaCmd.AddCommand(dramaListCmd)
	dramaCmd.AddCommand(dramaPublishedCmd)
	dramaCmd.AddCommand(dramaGetCmd)
	dramaCmd.AddCommand(dramaDeleteMediaCmd)
}
