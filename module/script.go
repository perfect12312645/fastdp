package module

import (
	"bytes"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"os"
	"strings"
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

	// 替换模板变量 {{.ip}} {{.port}} {{.user}}
	contentStr := ReplaceTemplate(string(content), hs)

	// 2. 准备输出
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout
	hs.Session.Stderr = &stderr

	// 3. 构建执行命令（支持参数和环境变量）
	scriptArgs := strings.TrimSpace(flags.Parameter["script_args"])
	scriptEnv := strings.TrimSpace(flags.Parameter["script_env"])

	delimiter := fmt.Sprintf("__FASTDP_SCRIPT_EOF_%d_%d", time.Now().UnixNano(), os.Getpid())

	// 在 heredoc 内设置环境变量和位置参数
	var heredocContent strings.Builder
	if scriptEnv != "" {
		for _, pair := range strings.Fields(scriptEnv) {
			heredocContent.WriteString("export " + pair + "\n")
		}
	}
	if scriptArgs != "" {
		// 使用 set -- 传递位置参数（每个参数用单引号包裹防空格问题）
		heredocContent.WriteString("set --")
		for _, arg := range strings.Fields(scriptArgs) {
			heredocContent.WriteString(" '" + strings.ReplaceAll(arg, "'", "'\\''") + "'")
		}
		heredocContent.WriteString("\n")
	}
	heredocContent.WriteString(contentStr)

	cmd := fmt.Sprintf("bash <<'%s'\n%s\n%s", delimiter, heredocContent.String(), delimiter)

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
