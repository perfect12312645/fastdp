package module

import (
	"bytes"
	"fastdp/pkg/config"
	. "fastdp/utils"
)

// ShellModule 实现 Module 接口，用于执行命令
type ShellModule struct {
}

// NewShellModule 创建 Shell 模块实例
func NewShellModule() Module {
	return &ShellModule{}
}

func (m *ShellModule) Run(hs HostSession, flags *config.Flags) Result {
	// 存储命令输出的缓冲区
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout // 将命令的标准输出重定向到缓冲区
	hs.Session.Stderr = &stderr // 将命令的标准错误输出重定向到缓冲区

	// 替换模板变量 {{.ip}} {{.port}} {{.user}}
	cmd := ReplaceTemplate(flags.Parameter["args"], hs)

	// 执行命令
	if err := hs.Session.Run(cmd); err != nil {
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
