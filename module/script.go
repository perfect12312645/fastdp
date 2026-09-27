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

	// switch 模式（交换机/网络设备）：命令清单文件按行逐条执行
	// 交换机无 bash，不能 heredoc 执行；命令有状态依赖（system-view→vlan 需同一会话），走交互式 shell
	if config.GlobalConfig.Mode == "switch" {
		return m.runSwitchMode(hs, contentStr, flags)
	}

	// linux 模式（默认）：heredoc 交给 bash 执行
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

// runSwitchMode 交换机命令清单执行：按行解析（跳过空行和 # 注释行），复用公共 PTY 执行器
func (m *ScriptModule) runSwitchMode(hs HostSession, content string, flags *config.Flags) Result {
	var commands []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		commands = append(commands, line)
	}
	if len(commands) == 0 {
		return Result{Success: false, Error: "命令清单为空（无有效命令行）", Change: false}
	}
	return runSwitchCommands(hs, commands, flags)
}

func init() {
	Register("script", NewScriptModule)
}
