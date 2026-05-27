package module

import (
	"bytes"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"os"
	"time"
)

type ScriptModule struct{}

func NewScriptModule() Module {
	return &ScriptModule{}
}

func (m *ScriptModule) Run(hs HostSession, flags *config.Flags) Result {
	scriptPath := flags.Parameter["script_file"]

	// 1. 读取本地脚本
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return Result{
			Success: false,
			Error:   "读取脚本失败: " + err.Error(),
			Change:  false,
		}
	}

	// 2. 准备输出
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout
	hs.Session.Stderr = &stderr

	// ===================== 【核心修复】随机分隔符，永远不冲突 =====================
	// 生成一个几乎不可能重复的结束标记
	delimiter := fmt.Sprintf("__FASTDP_SCRIPT_EOF_%d_%d", time.Now().UnixNano(), os.Getpid())

	// 安全执行脚本
	cmd := fmt.Sprintf("bash <<'%s'\n%s\n%s", delimiter, string(content), delimiter)
	// ==========================================================================
	err = hs.Session.Run(cmd)

	if err != nil {
		return Result{
			Success: false,
			Output:  stdout.String(),
			Error:   stderr.String() + "\n" + err.Error(),
			Change:  true,
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
	Register("script", NewScriptModule)
}
