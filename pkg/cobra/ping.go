package cobra

import "github.com/spf13/cobra"

// ping 命令
var pingCmd = &cobra.Command{
	Use:   "ping",
	Short: "测试主机连通性",
	Run: func(cmd *cobra.Command, args []string) {
		GlobalFlags.Module = "ping"
		// 处理主机组参数（如 web/all）
		GlobalFlags.HostInventory = args
		tValue, _ := cmd.Flags().GetString("timeout")
		GlobalFlags.Parameter["timeout"] = tValue
	},
	Example: `  ansible-tool ping web
  ansible-tool ping all --timeout 3`,
}
var _ = pingCmd.Flags().IntP("timeout", "t", 5, "连接超时时间(秒)")
