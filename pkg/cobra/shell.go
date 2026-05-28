package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var shellCmd = &cobra.Command{
	Use:           "shell",
	Short:         "在远程主机执行 shell 命令",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if aValue, _ := cmd.Flags().GetString("args"); aValue == "" {
			return fmt.Errorf("必须指定要执行的 shell 命令\n使用 --help 查看帮助信息")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// 处理主机组参数（如 web/all）
		config.GlobalFlags.HostInventory = args
		aValue, _ := cmd.Flags().GetString("args")
		config.GlobalFlags.Parameter["args"] = aValue
		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(-2)
		}
		hostSessions := SshConnect(execHosts)
		mod, err := module.GetModule("shell")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(-3)
		}
		execute(hostSessions, config.GlobalFlags, mod)
	},
	Example: `
  # 基础用法：单组执行命令
  fastdp shell -a "df -h" web

  # 混合指定：组 + IP 同时执行
  fastdp shell -a "free -h" master node 192.168.10.100

  # 引号使用：命令内部无冲突可自由使用
  fastdp shell -a 'ls -l /root' all
  fastdp shell -a "echo hello world" all

  # 高阶用法：批量输出 IP + 主机名（过滤无用输出）
  fastdp shell -a 'echo $(hostname -I|awk '\''{print $1}'\'') $(hostname)' all | grep -v 'output:'
`,

	Args: cobra.MinimumNArgs(1),
}

// 在包级别初始化时注册标志
var _ = shellCmd.Flags().StringP("args", "a", "", "要执行的 shell 命令 (必需)")
var _ = shellCmd.MarkFlagRequired("args")
