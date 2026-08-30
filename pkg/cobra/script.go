package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var scriptCmd = &cobra.Command{
	Use:           "script",
	Short:         "本地脚本在远程主机批量执行",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		scriptPath, _ := cmd.Flags().GetString("file")
		if scriptPath == "" {
			return fmt.Errorf("必须指定本地脚本路径\n示例: fastdp script -f run.sh all")
		}

		fileInfo, err := os.Stat(scriptPath)
		if os.IsNotExist(err) {
			return fmt.Errorf("脚本文件不存在 → %s", scriptPath)
		}

		if fileInfo.IsDir() {
			return fmt.Errorf("不能传入目录，请传入脚本文件 → %s", scriptPath)
		}

		const maxSize = 512 * 1024
		if fileInfo.Size() > maxSize {
			return fmt.Errorf("脚本文件过大(最大512KB) → %s", scriptPath)
		}

		isText, err := IsTextFile(scriptPath)
		if err != nil {
			return fmt.Errorf("检查文件类型失败 → %v", err)
		}
		if !isText {
			return fmt.Errorf("禁止上传二进制文件，请传入纯文本脚本 → %s", scriptPath)
		}

		if !strings.HasSuffix(scriptPath, ".sh") {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  警告: 文件非 .sh 后缀 → %s\n", scriptPath)
		}

		return nil
	},
 	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			Errorf("请指定目标主机组或主机\n使用 --help 查看帮助信息\n示例:\n  fastdp script -f run.sh all\n  fastdp script -f check.sh web")
			os.Exit(exitcode.ParamError)
		}
		// 处理主机组参数（如 web/all，支持区间展开）
		config.GlobalFlags.HostInventory = args

		// 读取脚本文件（本机操作，失败立即退出）
		scriptPath, _ := cmd.Flags().GetString("file")
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			Errorf("读取脚本失败: %v", err)
			os.Exit(exitcode.ParamError)
		}
		config.GlobalFlags.Parameter["script_content"] = string(content)

		// 传递脚本参数和环境变量
		scriptArgs, _ := cmd.Flags().GetString("args")
		if scriptArgs != "" {
			config.GlobalFlags.Parameter["script_args"] = scriptArgs
		}
		scriptEnv, _ := cmd.Flags().GetString("env")
		if scriptEnv != "" {
			config.GlobalFlags.Parameter["script_env"] = scriptEnv
		}
 		summary, _ := cmd.Flags().GetBool("summary")
		if summary {
			config.GlobalFlags.Parameter["summary"] = "true"
		}
		if quiet, _ := cmd.Flags().GetBool("quiet"); quiet {
			config.GlobalFlags.Quiet = true
		}

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}

		// 干跑模式：显示脚本和目标主机后退出
		if config.GlobalFlags.DryRun {
			fmt.Println("脚本:", scriptPath)
			if scriptArgs != "" {
				fmt.Println("参数:", scriptArgs)
			}
			if scriptEnv != "" {
				fmt.Println("环境变量:", scriptEnv)
			}
			fmt.Printf("目标主机 (%d 台):\n", len(execHosts))
			for _, h := range execHosts {
				fmt.Printf("  %s\n", h.Address)
			}
			os.Exit(exitcode.Success)
		}

 		// 脚本内容安全检查：硬拦截 / 确认
		yes, _ := cmd.Flags().GetBool("yes")
		allowDangerous, _ := cmd.Flags().GetBool("allow-dangerous")
		if !enforceCommandSafety(string(content), execHosts, yes, allowDangerous) {
			os.Exit(exitcode.ParamError)
		}

		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("script")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(exitcode.InternalError)
		}
		os.Exit(execute(hostSessions, failedHosts, config.GlobalFlags, mod, "script"))
	},
	Example: `
  # 基础用法
  fastdp script -f run.sh all
  fastdp script -f check.sh master node 192.168.1.100

  # 传递参数和环境变量
  fastdp script -f init.sh --args "eth0 192.168.1.1" --env "MODE=persist MTU=9000" all

  # 静默模式（只输出脚本原始 stdout）
  fastdp script -f check.sh all -q
`,
}

// 注册参数

func init() {
	scriptCmd.Flags().StringP("file", "f", "", "本地脚本路径 (必需)")
	_ = scriptCmd.MarkFlagRequired("file")
	scriptCmd.Flags().String("args", "", "传递给脚本的位置参数（空格分隔，脚本内通过 $1 $2 获取）")
	scriptCmd.Flags().String("env", "", "传递给脚本的环境变量（格式：KEY=val KEY2=val2）")
	scriptCmd.Flags().BoolP("yes", "y", false, "危险命令自动确认（CI场景）")
	scriptCmd.Flags().Bool("allow-dangerous", false, "显式放行硬拦截的破坏性命令（不建议）")
	scriptCmd.Flags().BoolP("summary", "s", false, "汇总模式：只显示失败主机，成功主机折叠为一行")
	scriptCmd.Flags().BoolP("quiet", "q", false, "静默模式：只输出命令原始 stdout，无装饰文本（适合管道和脚本）")
}
