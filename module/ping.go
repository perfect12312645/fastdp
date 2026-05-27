package module

import (
	"bytes"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"strings"
)

type PingModule struct {
}

// NewShellModule 创建 Shell 模块实例
func NewPingModule() Module {
	return &PingModule{}
}

func (m *PingModule) Run(hs HostSession, flags *config.Flags) Result {
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
