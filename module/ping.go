package module

import (
	"bytes"
	"errors"
	"fastdp/pkg/config"
	. "fastdp/utils"

	"golang.org/x/crypto/ssh"
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

	// switch 模式：交换机 CLI 无 echo 命令，改用 display clock
	cmd := "echo pong"
	if config.GlobalConfig.Mode == "switch" {
		cmd = "display clock"
	}

	err := hs.Session.Run(cmd)
	if err != nil {
		// 命令执行失败（如交换机不认识该命令返回 % Unrecognized / Linux ExitError 127）
		// 不算连通失败——设备有响应即已连通；仅连接层错误（ExitMissing）才算失败
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return Result{
				Success: true,
				Output:  "pong",
				Error:   "",
				Change:  false,
			}
		}
		return Result{
			Success: false,
			Output:  stdout.String(),
			Error:   stderr.String() + "\n" + err.Error(),
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
