package cobra

import (
	"fmt"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "在远程主机执行 shell 命令",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// 检查必需参数是否提供
		if aValue, _ := cmd.Flags().GetString("args"); aValue == "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "错误: 必须指定要执行的 shell 命令")
			fmt.Fprintln(cmd.ErrOrStderr(), "使用 --help 查看帮助信息")
			return fmt.Errorf("缺少必需参数")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		GlobalFlags.Module = "shell"
		// 处理主机组参数（如 web/all）
		GlobalFlags.HostInventory = args
		aValue, _ := cmd.Flags().GetString("args")
		GlobalFlags.Parameter["args"] = aValue
	},
	Example: `  fastdp shell -a "df -h" web
  fastdp shell -a "service nginx restart" db`,
	Args: cobra.MinimumNArgs(1),
}

// 在包级别初始化时注册标志
var _ = shellCmd.Flags().StringP("args", "a", "", "要执行的 shell 命令 (必需)")
var _ = shellCmd.MarkFlagRequired("args")
