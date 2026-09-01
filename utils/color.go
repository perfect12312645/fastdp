package utils

import (
	"fastdp/pkg/config"
	"fmt"
	"os"

	"golang.org/x/term"
)

const (
	ColorReset     = "\033[0m"  // 重置颜色（必须在每个带颜色的输出后使用）
	ColorError     = "\033[31m" // 红色：表示错误/失败（如命令执行报错）
	ColorChanged   = "\033[32m" // 绿色：表示状态变更（如文件被修改、服务被重启）
	ColorUnchanged = "\033[33m" // 黄色：表示无变更（如文件未修改、服务已运行）
)

// isTerminal 检测 stdout 是否为终端（管道/重定向时返回 false，自动禁用颜色）
func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Errorf 打印红色错误信息
func Errorf(format string, v ...interface{}) {
	if isTerminal() {
		fmt.Printf(ColorError+format+ColorReset+"\n", v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

// Changedf 打印绿色变更信息
func Changedf(format string, v ...interface{}) {
	if isTerminal() {
		fmt.Printf(ColorChanged+format+ColorReset+"\n", v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

// Unchangedf 打印黄色无变更信息
func Unchangedf(format string, v ...interface{}) {
	if isTerminal() {
		fmt.Printf(ColorUnchanged+format+ColorReset+"\n", v...)
	} else {
		fmt.Printf(format+"\n", v...)
	}
}

// Debugf 输出调试信息（仅 -v 开启时，输出到 stderr 避免污染 stdout 管道）
func Debugf(format string, v ...interface{}) {
	if config.GlobalFlags.Debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", v...)
	}
}

// SummaryLine 输出成功/失败汇总行（带颜色，非终端时自动去除）
func SummaryLine(okCount, failCount, total int) string {
	if isTerminal() {
		line := fmt.Sprintf("\033[32m[OK] %d/%d 成功\033[0m", okCount, total)
		if failCount > 0 {
			line += fmt.Sprintf("  \033[31m[FAIL] %d/%d 失败\033[0m", failCount, total)
		}
		return line
	}
	line := fmt.Sprintf("[OK] %d/%d 成功", okCount, total)
	if failCount > 0 {
		line += fmt.Sprintf("  [FAIL] %d/%d 失败", failCount, total)
	}
	return line
}

// HostHeader 输出主机头部信息（带颜色，非终端时自动去除）
func HostHeader(host string) string {
	if isTerminal() {
		return fmt.Sprintf("\n\033[1;34m===== %s =====\033[0m\n", host)
	}
	return fmt.Sprintf("\n===== %s =====\n", host)
}
