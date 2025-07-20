package module

import (
	. "fastdp/pkg/flags"
	. "fastdp/utils"
)

// Result 模块执行结果的统一格式
type Result struct {
	Success bool   // 是否执行成功
	Output  string // 标准输出
	Error   string // 错误信息（如果失败）
	Change  bool   // 是否改变
}

// Module 模块接口，所有模块必须实现这些方法
type Module interface {
	// SetParams 解析模块参数（如 "path=/tmp/file mode=0644"）
	SetParams(params string) error

	// Run 在目标主机上执行模块逻辑（通过 SSH 会话）
	Run(hostSessions HostSession, flags *Flags) Result
}
