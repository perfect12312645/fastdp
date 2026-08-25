package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"github.com/spf13/cobra"
	"os"

	"fastdp/pkg/exitcode"
)

// ping 命令
var pingCmd = &cobra.Command{
	Use:           "ping",
	Short:         "测试主机连通性",
	SilenceErrors: true,
	SilenceUsage:  true,
	Run: func(cmd *cobra.Command, args []string) {
		// 处理主机组参数（如 web/all）
		config.GlobalFlags.HostInventory = args

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}
		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("ping")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(exitcode.InternalError)
		}
		os.Exit(execute(hostSessions, failedHosts, config.GlobalFlags, mod, "ping"))
	},
	Example: `
  fastdp ping web

  fastdp ping all`,
	Args: cobra.MinimumNArgs(1),
}
