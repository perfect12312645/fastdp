package flags

import (
	"fastdp/pkg/log"

	"flag"
	"fmt"
	"go.uber.org/zap"
	"os"
)

var Logger *zap.Logger

type Flags struct {
	Module    *string
	Parameter *string
	Debug     *bool
}

const (
	ColorReset     = "\033[0m"  // 重置颜色（必须在每个带颜色的输出后使用）
	ColorError     = "\033[31m" // 红色：表示错误/失败（如命令执行报错）
	ColorChanged   = "\033[32m" // 绿色：表示状态变更（如文件被修改、服务被重启）
	ColorUnchanged = "\033[33m" // 黄色：表示无变更（如文件未修改、服务已运行）
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] [主机组1] [主机组2] ...\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "在指定主机组上执行对应模块,当前仅支持shell\n\n")
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
主机组:
  指定要操作的主机组名称，可指定多个。

主机组配置文件:
  默认从当前目录加载host格式为:
  # 不指定user默认为root，port默认为22，password不指定需要设置免密
  [web]
  192.168.1.100 user=root port=22 password=123456
  192.168.1.101  

配置文件:
  默认从当前目录加载 config.toml
  Host_inventory = "host"

示例:
  # 在 web 组上执行 shell 命令
  %s -m shell -a "ls -l /tmp" web

  # shell为默认模块可省略
  %s -a "uptime" web

  # 对所有主机执行命令
  %s -m shell -a "uptime" all
`, os.Args[0], os.Args[0], os.Args[0])

	}

}

func ParseFlags() ([]string, *Flags) {
	Flags := Flags{
		Module:    flag.String("m", "shell", "指定执行的模块"),
		Parameter: flag.String("a", "", "指定执行的命令"),
		Debug:     flag.Bool("v", false, "是否开启调试模式"),
	}
	flag.Parse()
	Logger = log.InitLogger(*Flags.Debug)
	Logger.Debug("message", zap.Bool("开启debug模式", *Flags.Debug))
	return flag.Args(), &Flags
}

// 控制是否启用颜色（默认启用，可在不支持颜色的终端中关闭）
var UseColors = true

// Errorf 打印红色错误信息（
func Errorf(format string, v ...interface{}) {
	if UseColors {
		fmt.Printf(ColorError+format+ColorReset+"\n", v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

// Changedf 打印绿色变更信息
func Changedf(format string, v ...interface{}) {
	if UseColors {
		fmt.Printf(ColorChanged+format+ColorReset+"\n", v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

// Unchangedf 打印黄色无变更信息
func Unchangedf(format string, v ...interface{}) {
	if UseColors {
		fmt.Printf(ColorUnchanged+format+ColorReset+"\n", v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}
