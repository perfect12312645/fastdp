package cobra

import (
	"fastdp/pkg/log"
	"fmt"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
)

var Logger *zap.Logger

// Flags 统一管理命令参数
type Flags struct {
	Module        string            // 当前使用的模块（如 shell/copy/ping）
	Parameter     map[string]string // 模块参数（键值对形式）
	HostInventory []string          // 主机组或者主机
	Debug         bool              // 调试模式
	Concurrency   int               // 并发数量
}

var (
	GlobalFlags = &Flags{
		Parameter: make(map[string]string), // 初始化参数 map
	}
)

var rootCmd = &cobra.Command{
	Use:   "fastdp",
	Short: "轻量级 Ansible 风格运维工具",
	Long:  `在指定主机组上执行运维操作，支持多模块管理。`,
	Example: `  # 在 web 组上执行 shell 命令
  fastdp shell -a "ls -l /tmp" web

  # 对所有主机执行命令 (shell为默认模块可省略)
  fastdp shell -a "uptime" all

  # 使用 copy 模块复制文件
  fastdp copy -s local.txt -d /remote/path/ db  192.168.1.106 

  # 测试主机连通性
  fastdp ping all`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		GlobalFlags.Concurrency = concurrency
		GlobalFlags.Debug, _ = cmd.Flags().GetBool("debug")
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help() // 显式输出帮助信息
		os.Exit(0) // 退出程序，避免执行后续可能导致 panic 的代码
	},
}

func init() {
	// 全局标志
	//rootCmd.PersistentFlags().StringP("args", "a", "", "shell模块参数")
	rootCmd.PersistentFlags().IntP("concurrency", "c", 10, "并发连接数")
	rootCmd.PersistentFlags().BoolP("debug", "v", false, "是否开启调试模式")

	// 添加子命令
	rootCmd.AddCommand(shellCmd, copyCmd, pingCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	Logger = log.InitLogger(GlobalFlags.Debug)
	Logger.Debug("初始化完成",
		zap.Bool("debug", GlobalFlags.Debug),
		zap.Int("concurrency", GlobalFlags.Concurrency))
}
