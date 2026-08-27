package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
		aggregate, _ := cmd.Flags().GetString("aggregate")
		if aggregate != "" {
			config.GlobalFlags.Parameter["aggregate"] = aggregate
		}
		summary, _ := cmd.Flags().GetBool("summary")
		if summary {
			config.GlobalFlags.Parameter["summary"] = "true"
		}
		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}

		// 干跑模式：显示命令和目标主机后退出
		if config.GlobalFlags.DryRun {
			fmt.Println("命令:", aValue)
			fmt.Printf("目标主机 (%d 台):\n", len(execHosts))
			for _, h := range execHosts {
				fmt.Printf("  %s\n", h.Address)
			}
			os.Exit(exitcode.Success)
		}

		yes, _ := cmd.Flags().GetBool("yes")
		allowDangerous, _ := cmd.Flags().GetBool("allow-dangerous")
		if !enforceCommandSafety(aValue, execHosts, yes, allowDangerous) {
			os.Exit(exitcode.ParamError)
		}

		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("shell")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(exitcode.InternalError)
		}
		os.Exit(execute(hostSessions, failedHosts, config.GlobalFlags, mod, "shell"))
	},
	Example: `
  # 基础用法：单组执行命令
  fastdp shell -a "df -h" web

  # 混合指定：组 + IP 同时执行
  fastdp shell -a "free -h" master node 192.168.10.100

  # 引号使用：命令内部无冲突可自由使用
  fastdp shell -a 'ls -l /root' all
  fastdp shell -a "echo hello world" all

  # 批量输出 IP + 主机名（模板变量 + 静默模式）结果可直接重定向到 /etc/hosts（集群初始化场景）
  fastdp shell -a 'echo {{.ip}} $(hostname)' all -q

  # 聚合统计：计算集群平均 CPU 使用率
  fastdp shell -a "mpstat 1 1 | awk '/Average/{print 100-\$NF}'" all --aggregate avg

  # 中位数、P95、标准差
  fastdp shell -a "mpstat 1 1 | awk '/Average/{print 100-\$NF}'" all --aggregate median
  fastdp shell -a "curl -o /dev/null -s -w '%{time_total}' http://example.com" all --aggregate p95

  # 危险命令（如 rm -rf /tmp/*）默认交互确认，--yes 跳过（CI）
  fastdp shell -a 'rm -rf /tmp/*' master
  fastdp shell -a 'rm -rf /tmp/*' master --yes
`,
	Args: cobra.MinimumNArgs(1),
}

func init() {
	shellCmd.Flags().StringP("args", "a", "", "要执行的 shell 命令 (必需)")
	_ = shellCmd.MarkFlagRequired("args")
	shellCmd.Flags().String("aggregate", "", "聚合函数：avg/max/min/sum/median/p95/p99/stddev")
	shellCmd.Flags().BoolP("yes", "y", false, "危险命令自动确认（CI场景）")
	shellCmd.Flags().Bool("allow-dangerous", false, "显式放行硬拦截的破坏性命令（不建议）")
	shellCmd.Flags().BoolP("summary", "s", false, "汇总模式：只显示失败主机，成功主机折叠为一行")
}
