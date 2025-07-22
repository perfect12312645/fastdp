package cobra

import (
	"fmt"
	"github.com/spf13/cobra"
)

// copy 命令
var copyCmd = &cobra.Command{
	Use:   "copy",
	Short: "复制文件到远程主机",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// 正确获取两个参数并判断（修复语法错误）
		sValue, _ := cmd.Flags().GetString("source")
		dValue, _ := cmd.Flags().GetString("dest")

		if sValue == "" || dValue == "" {
			// 输出到错误流（符合规范）
			fmt.Fprintln(cmd.ErrOrStderr(), "错误: 必须指定要copy的源文件（-s）和目标位置（-d）")
			fmt.Fprintln(cmd.ErrOrStderr(), "使用 --help 查看帮助信息")
			return fmt.Errorf("缺少必需参数")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		sValue, _ := cmd.Flags().GetString("source")
		dValue, _ := cmd.Flags().GetString("dest")
		GlobalFlags.Module = "copy"
		// 处理主机组参数（如 web/all）
		GlobalFlags.HostInventory = args
		GlobalFlags.Parameter["source"] = sValue
		GlobalFlags.Parameter["dest"] = dValue
	},
	Example: `  fastdp copy -s config.conf -d /etc/app/ web
  fastdp copy -s script.sh -d /tmp/test.sh db 192.168.1.101`,
	Args: cobra.MinimumNArgs(1),
}

var _ = copyCmd.Flags().StringP("source", "s", "", "源文件路径 (必需)")
var _ = copyCmd.Flags().StringP("dest", "d", "", "目标路径 (必需)")
var _ = copyCmd.MarkFlagRequired("source")
var _ = copyCmd.MarkFlagRequired("dest")
