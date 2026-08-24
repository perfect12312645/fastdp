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
		summary, _ := cmd.Flags().GetBool("summary")
		if summary {
			config.GlobalFlags.Parameter["summary"] = "true"
		}
		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(-2)
		}

		yes, _ := cmd.Flags().GetBool("yes")
		allowDangerous, _ := cmd.Flags().GetBool("allow-dangerous")
		if !enforceCommandSafety(aValue, execHosts, yes, allowDangerous) {
			os.Exit(1)
		}

		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("shell")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(-3)
		}
		execute(hostSessions, failedHosts, config.GlobalFlags, mod, "shell")
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

  # 危险命令（如 rm -rf /tmp/*）默认交互确认，--yes 跳过（CI）
  fastdp shell -a 'rm -rf /tmp/*' master
  fastdp shell -a 'rm -rf /tmp/*' master --yes
`,
	Args: cobra.MinimumNArgs(1),
}

func init() {
	shellCmd.Flags().StringP("args", "a", "", "要执行的 shell 命令 (必需)")
	_ = shellCmd.MarkFlagRequired("args")
	shellCmd.Flags().BoolP("yes", "y", false, "危险命令自动确认（CI场景）")
	shellCmd.Flags().Bool("allow-dangerous", false, "显式放行硬拦截的破坏性命令（不建议）")
	shellCmd.Flags().BoolP("summary", "s", false, "汇总模式：只显示失败主机，成功主机折叠为一行")
}
