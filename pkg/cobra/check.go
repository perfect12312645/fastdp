package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
)

// getCheckScriptPath 获取巡检脚本路径：用户目录优先，系统目录兜底
func getCheckScriptPath() string {
	// 1. 优先：用户家目录
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userScript := filepath.Join(homeDir, ".fastdp", "fastdp-check.sh")
		if _, err := os.Stat(userScript); err == nil {
			return userScript
		}
	}

	// 2. 兜底：系统全局路径
	return "/etc/fastdp/fastdp-check.sh"
}

var checkCmd = &cobra.Command{
	Use:           "check",
	Short:         "批量主机环境检查",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// 获取最终脚本路径
		checkScript := getCheckScriptPath()
		// 固定判断脚本是否存在
		stat, err := os.Stat(checkScript)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("巡检脚本不存在 → %s", checkScript)
			}
			return fmt.Errorf("读取脚本失败: %v", err)
		}
		if stat.IsDir() {
			return fmt.Errorf("错误：%s 是目录，必须是脚本文件", checkScript)
		}

		vertical, _ := cmd.Flags().GetBool("vertical")
		format, _ := cmd.Flags().GetString("format")

		// 3. 互斥判断：-g 和 -f 不能同时存在（中文提示）
		if vertical && format != "" {
			return fmt.Errorf("参数冲突：-g（竖向输出）和 -f（导出格式）不能同时使用")
		}
		if format != "" {
			validFormats := map[string]bool{"csv": true, "md": true, "html": true}
			if !validFormats[format] {
				return fmt.Errorf("无效格式：%s，合法值：csv|md|html", format)
			}
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// 获取最终脚本路径
		checkScript := getCheckScriptPath()

		vertical, _ := cmd.Flags().GetBool("vertical")
		if vertical {
			config.GlobalFlags.Parameter["vertical"] = "true"
		}
		outputFormat, _ := cmd.Flags().GetString("format")
		config.GlobalFlags.Parameter["format"] = outputFormat
		// 主机组参数
		config.GlobalFlags.HostInventory = args

		config.GlobalFlags.Parameter["script_file"] = checkScript

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(-2)
		}

		hostSessions := SshConnect(execHosts)
		mod, err := module.GetModule("script") // ✅ 直接复用 script 模块
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(-3)
		}
		execute(hostSessions, config.GlobalFlags, mod)
	},
	Example: `
  # 对所有主机执行环境检查（默认表格输出）
  # 巡检脚本路径优先级：~/.fastdp/fastdp-check.sh > /etc/fastdp/fastdp-check.sh 可自行编辑自定义输出内容
  fastdp check all

  # 竖向格式化输出（类似 mysql \\G）
  fastdp check all -g

  # 导出巡检报告并保存为文件
  fastdp check all -f csv  > report.csv
  fastdp check all -f md   > report.md
  fastdp check all -f html > report.html
`,
	Args: cobra.MinimumNArgs(1),
}

// 👇 注册参数
func init() {
	checkCmd.Flags().BoolP("vertical", "g", false, "竖向格式化输出 (类似 mysql \\G)")
	checkCmd.Flags().StringP("format", "f", "", "输出格式: csv|md|html")
}
