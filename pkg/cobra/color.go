package cobra

import "fmt"

const (
	ColorReset     = "\033[0m"  // 重置颜色（必须在每个带颜色的输出后使用）
	ColorError     = "\033[31m" // 红色：表示错误/失败（如命令执行报错）
	ColorChanged   = "\033[32m" // 绿色：表示状态变更（如文件被修改、服务被重启）
	ColorUnchanged = "\033[33m" // 黄色：表示无变更（如文件未修改、服务已运行）
)

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
