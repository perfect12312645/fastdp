package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"github.com/spf13/cobra"
	"os"
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
		tValue, _ := cmd.Flags().GetString("timeout")
		config.GlobalFlags.Parameter["timeout"] = tValue
		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(-2)
		}
		hostSessions := SshConnect(execHosts)
		mod, err := module.GetModule("ping")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(-3)
		}
		execute(hostSessions, config.GlobalFlags, mod)
	},
	Example: `  ansible-tool ping web
  ansible-tool ping all --timeout 3`,
}
var _ = pingCmd.Flags().IntP("timeout", "t", 5, "连接超时时间(秒)")
