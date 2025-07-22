package module

import (
	"bytes"
	. "fastdp/pkg/cobra"
	. "fastdp/utils"
)

// ShellModule 实现 Module 接口，用于执行命令
type ShellModule struct {
}

// NewShellModule 创建 Shell 模块实例
func NewShellModule() Module {
	return &ShellModule{}
}

func (m *ShellModule) Run(hs HostSession, flags *Flags) Result {
	// 存储命令输出的缓冲区
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout // 将命令的标准输出重定向到缓冲区
	hs.Session.Stderr = &stderr // 将命令的标准错误输出重定向到缓冲区
	// 执行命令（*flags.Parameter是命令内容）
	if err := hs.Session.Run(flags.Parameter["args"]); err != nil {
		return Result{
			Success: false,
			Output:  stdout.String(),
			Error:   stderr.String() + "\n" + err.Error(),
			Change:  false,
		}
	}
	return Result{
		Success: true,
		Output:  stdout.String(),
		Error:   "",
		Change:  true,
	}
}
func init() {
	Register("shell", NewShellModule) // 注册 "shell" 模块
}
