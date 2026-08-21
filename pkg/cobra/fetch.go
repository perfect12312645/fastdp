package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

var fetchCmd = &cobra.Command{
	Use:           "fetch",
	Short:         "批量从远程主机拉取文件（支持通配符*、?、[]）",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// 必须指定远程文件路径
		if remoteFile, _ := cmd.Flags().GetString("remote"); remoteFile == "" {
			return fmt.Errorf("必须指定远程文件路径\n使用 --help 查看帮助信息")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// 处理主机组参数（如 web/all）
		config.GlobalFlags.HostInventory = args

		// 获取远程文件 & 本地保存目录
		remoteFile, _ := cmd.Flags().GetString("remote")
		localDir, _ := cmd.Flags().GetString("dest")
		noIpDir, _ := cmd.Flags().GetBool("no-ip-dir")

		config.GlobalFlags.Parameter["remote"] = remoteFile
		config.GlobalFlags.Parameter["dest"] = localDir
		config.GlobalFlags.Parameter["no_ip_dir"] = fmt.Sprintf("%v", noIpDir)

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(-2)
		}

		hostSessions := SshConnect(execHosts)
		mod, err := module.GetModule("fetch")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(-3)
		}

		execute(hostSessions, config.GlobalFlags, mod, "fetch")
	},
	Example: `
  # 批量拉取所有主机 /tmp/sec* 文件
  # 【重要】远程路径含 * ? 等通配符时，必须用 " 或 ' 包裹，避免本地shell提前解析
  fastdp fetch --remote "/tmp/sec*" all

  # 拉取指定组/IP 的日志文件
  fastdp fetch -r "/var/log/messages" master
  fastdp fetch -r "/root/*.txt" 192.168.1.101

  # 指定本地保存目录（默认 ./fastdp-fetch）
  fastdp fetch -r "/tmp/sec?" --dest ./my-download all

  # 不创建IP目录，文件名为：IP_文件名（避免重名）
  fastdp fetch -r "/tmp/*.log" --no-ip-dir all

  # 支持通配符 * ?
  fastdp fetch -r "/tmp/sec-*.log" all
`,

	Args: cobra.MinimumNArgs(1),
}

// 注册标志
func init() {
	// 远程文件路径（支持通配符）
	fetchCmd.Flags().StringP("remote", "r", "", "远程文件路径（支持 * ? [] 通配符，必需）")
	_ = fetchCmd.MarkFlagRequired("remote")

	// 本地保存目录（默认 ./fastdp-fetch）
	fetchCmd.Flags().StringP("dest", "d", "", "本地保存目录,默认./fastdp-fetch")

	fetchCmd.Flags().BoolP("no-ip-dir", "", false, "拉取文件时不创建IP目录，文件名为 主机IP_文件名")
}
