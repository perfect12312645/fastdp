package module

import (
	"bytes"
	. "fastdp/pkg/flags"
	. "fastdp/utils"
)

// ShellModule 实现 Module 接口，用于执行命令
type ShellModule struct {
	command string // 要执行的命令（从参数解析）
}

// NewShellModule 创建 Shell 模块实例
func NewShellModule() Module {
	return &ShellModule{}
}

// SetParams 解析参数（格式：直接是命令字符串，如 "ls -l /tmp"）
func (m *ShellModule) SetParams(params string) error {
	m.command = params // 简单直接，参数就是命令本身
	return nil
}

func (m *ShellModule) Run(hs HostSession, flags *Flags) Result {
	// 存储命令输出的缓冲区
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout // 将命令的标准输出重定向到缓冲区
	hs.Session.Stderr = &stderr // 将命令的标准错误输出重定向到缓冲区

	// 执行命令（*flags.Parameter是命令内容）
	if err := hs.Session.Run(*flags.Parameter); err != nil {
		Errorf("在 %s 上执行命令失败: %v", hs.Addr, err)
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
