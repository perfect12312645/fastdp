package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var scriptCmd = &cobra.Command{
	Use:   "script",
	Short: "本地脚本在远程主机批量执行",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		scriptPath, _ := cmd.Flags().GetString("file")
		if scriptPath == "" {
			fmt.Fprintln(cmd.ErrOrStderr(), "错误: 必须指定本地脚本路径")
			fmt.Fprintln(cmd.ErrOrStderr(), "示例: fastdp script -f run.sh all")
			return fmt.Errorf("缺少必需参数 -f/--file")
		}
		// ===================== 【关键：提前校验文件】=====================
		// 1. 判断文件是否存在
		fileInfo, err := os.Stat(scriptPath)
		if os.IsNotExist(err) {
			return fmt.Errorf("错误: 脚本文件不存在 → %s", scriptPath)
		}

		// 2. 判断是否是目录（防止用户传目录）
		if fileInfo.IsDir() {
			return fmt.Errorf("错误: 不能传入目录，请传入脚本文件 → %s", scriptPath)
		}

		// ========== 3. 文件大小限制：512KB (最合理) ==========
		const maxSize = 512 * 1024 // 512KB
		if fileInfo.Size() > maxSize {
			return fmt.Errorf("错误: 脚本文件过大(最大512KB) → %s", scriptPath)
		}

		isText, err := IsTextFile(scriptPath)
		if err != nil {
			return fmt.Errorf("错误: 检查文件类型失败 → %v", err)
		}
		if !isText {
			return fmt.Errorf("错误: 禁止上传二进制文件，请传入纯文本脚本 → %s", scriptPath)
		}

		// ========== 5. 可选：.sh 警告 ==========
		if !strings.HasSuffix(scriptPath, ".sh") {
			fmt.Fprintf(cmd.ErrOrStderr(), "⚠️  警告: 文件非 .sh 后缀 → %s\n", scriptPath)
		}
		// =================================================================

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		// 处理主机组参数（如 web/all）
		config.GlobalFlags.HostInventory = args
		scriptPath, _ := cmd.Flags().GetString("file")
		config.GlobalFlags.Parameter["script_file"] = scriptPath

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(-2)
		}

		hostSessions := SshConnect(execHosts)
		mod, err := module.GetModule("script")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(-3)
		}
		execute(hostSessions, config.GlobalFlags, mod)
	},
	Example: `
  # 上传本地脚本并在所有主机执行
  fastdp script -f run.sh all

  # 执行本地脚本到指定组和IP
  fastdp script -f check.sh master node 192.168.10.100

  # 脚本自动上传、执行、自动清理
  fastdp script -f init.sh db
`,
	Args: cobra.MinimumNArgs(1),
}

// 注册参数
var _ = scriptCmd.Flags().StringP("file", "f", "", "本地脚本路径 (必需)")
var _ = scriptCmd.MarkFlagRequired("file")
