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
	contentStr := flags.Parameter["script_content"]
	if contentStr == "" {
		return Result{Success: false, Error: "脚本内容为空", Change: false}
	}

	// 替换模板变量 {{.ip}} {{.port}} {{.user}}
	contentStr = ReplaceTemplate(contentStr, hs)

	// 准备输出
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout
	hs.Session.Stderr = &stderr

	// 构建执行命令（支持参数和环境变量）
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
		heredocContent.WriteString("set --")
		for _, arg := range strings.Fields(scriptArgs) {
			heredocContent.WriteString(" '" + strings.ReplaceAll(arg, "'", "'\\''") + "'")
		}
		heredocContent.WriteString("\n")
	}
	heredocContent.WriteString(contentStr)

	cmd := fmt.Sprintf("bash <<'%s'\n%s\n%s", delimiter, heredocContent.String(), delimiter)

	if err := hs.Session.Run(cmd); err != nil {
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
