package module

import (
	"bytes"
	. "fastdp/pkg/flags"
	. "fastdp/utils"
	"strings"
)

type PingModule struct {
	command string // 要执行的命令（从参数解析）
}

// NewShellModule 创建 Shell 模块实例
func NewPingModule() Module {
	return &PingModule{}
}

// SetParams 解析参数（格式：直接是命令字符串，如 "ls -l /tmp"）
func (m *PingModule) SetParams(params string) error {
	m.command = params // 简单直接，参数就是命令本身
	return nil
}
func (m *PingModule) Run(hs HostSession, flags *Flags) Result {
	// 存储命令输出的缓冲区
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout // 将命令的标准输出重定向到缓冲区
	hs.Session.Stderr = &stderr // 将命令的标准错误输出重定向到缓冲区
	cmd := "echo pong"
	if err := hs.Session.Run(cmd); err != nil {
		return Result{
			Success: false,
			Output:  stdout.String(),
			Error:   stderr.String() + "\n" + err.Error(),
			Change:  false,
		}
	}
	output := stdout.String()
	if !strings.Contains(strings.TrimSpace(output), "pong") {
		return Result{
			Success: false,
			Output:  output,
			Error:   "Unexpected output: " + output,
			Change:  false,
		}
	}

	return Result{
		Success: true,
		Output:  "pong",
		Error:   "",
		Change:  false,
	}
}
func init() {
	Register("ping", NewPingModule) // 注册 "shell" 模块
}
