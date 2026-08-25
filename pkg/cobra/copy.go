package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"time"
)

// copy 命令
var copyCmd = &cobra.Command{
	Use:           "copy",
	Short:         "复制文件到远程主机",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		sValue, _ := cmd.Flags().GetString("source")
		dValue, _ := cmd.Flags().GetString("dest")

		if sValue == "" || dValue == "" {
			return fmt.Errorf("必须指定要copy的源文件(-s)和目标位置(-d)\n使用 --help 查看帮助信息")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		sValue, _ := cmd.Flags().GetString("source")
		dValue, _ := cmd.Flags().GetString("dest")
		// 处理主机组参数（如 web/all）
		config.GlobalFlags.HostInventory = args
		config.GlobalFlags.Parameter["source"] = sValue
		config.GlobalFlags.Parameter["dest"] = dValue

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}
		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("copy")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(exitcode.InternalError)
		}
		// 转换为Debug日志，使用%v格式化时间对象
		Debugf("copy模块，开始计算源文件md5: %v", time.Now())
		err = module.GetSource(config.GlobalFlags)
		if err != nil {
			Errorf("copy模块，获取源文件信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}
		Debugf("copy模块，计算源文件md5结束: %v", time.Now())
		os.Exit(execute(hostSessions, failedHosts, config.GlobalFlags, mod, "copy"))
	},
	Example: `  # 推送配置文件到 web 组
  fastdp copy -s app.conf -d /etc/ web

  # 推送脚本到指定主机
  fastdp copy -s run.sh -d /tmp/run.sh 192.168.1.101`,
	Args: cobra.MinimumNArgs(1),
}

func init() {
	copyCmd.Flags().StringP("source", "s", "", "源文件路径 (必需)")
	copyCmd.Flags().StringP("dest", "d", "", "目标路径 (必需)")
	_ = copyCmd.MarkFlagRequired("source")
	_ = copyCmd.MarkFlagRequired("dest")
}
