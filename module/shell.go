package module

import (
	"bytes"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ShellModule 实现 Module 接口，用于执行命令
type ShellModule struct {
}

// NewShellModule 创建 Shell 模块实例
func NewShellModule() Module {
	return &ShellModule{}
}

func (m *ShellModule) Run(hs HostSession, flags *config.Flags) Result {
	// 替换模板变量 {{.ip}} {{.port}} {{.user}}
	cmd := ReplaceTemplate(flags.Parameter["args"], hs)

	// switch 模式（交换机/网络设备）：交互式执行
	if config.GlobalConfig.Mode == "switch" {
		return m.runSwitchMode(hs, cmd, flags)
	}

	// linux 模式（默认）：直接执行整行
	var stdout, stderr bytes.Buffer
	hs.Session.Stdout = &stdout
	hs.Session.Stderr = &stderr

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

// runSwitchMode 交换机模式：按 ; 分割命令，交给公共 PTY 执行器
func (m *ShellModule) runSwitchMode(hs HostSession, cmd string, flags *config.Flags) Result {
	// 按 ; 分割命令（懂转义，\; 不分割）
	commands := splitSemicolon(cmd)
	if len(commands) == 0 {
		return Result{Success: false, Error: "命令为空", Change: false}
	}
	return runSwitchCommands(hs, commands, flags)
}

// runSwitchCommands 交换机模式公共执行器：交互式 Shell 逐条执行命令（处理分页）
// 复用于 shell（命令行 ; 分割）和 script（命令清单文件按行读取）模块
// 复用 SshConnect 预建的 session（H3C MaxSessions=1，不能再开新 channel）
func runSwitchCommands(hs HostSession, commands []string, flags *config.Flags) Result {
	// 1. 获取分页禁用命令，优先级：命令行 flag > host 主机参数 > config.toml > 默认 H3C 命令
	//    （混合设备场景：config 设全局默认，host 按单台覆盖，flag 临时覆盖本次执行）
	pagingDisable := flags.Parameter["paging_disable"]
	if pagingDisable == "" {
		pagingDisable = hs.Params["paging_disable"]
	}
	if pagingDisable == "" {
		pagingDisable = config.GlobalConfig.PagingDisable
	}
	if pagingDisable == "" {
		pagingDisable = "screen-length disable"
	}

	sess := hs.Session

	// 2. 获取 stdin 管道，stdout 用 buffer 直连（ssh 库内部自动消费 channel，
	//    无需 goroutine；StdoutPipe 才需要调用方自己读，否则数据积压）
	stdin, err := sess.StdinPipe()
	if err != nil {
		return Result{Success: false, Error: "获取输入管道失败: " + err.Error(), Change: false}
	}
	var outputBuf bytes.Buffer
	sess.Stdout = &outputBuf

	// 3. 请求 PTY（交换机需要终端类型）
	// ECHO=0 禁用回显：交换机 CLI 自回显命令，不依赖终端 ECHO 参数
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("vt100", 80, 40, modes); err != nil {
		return Result{Success: false, Error: "请求终端失败: " + err.Error(), Change: false}
	}

	// 4. 启动交互式 shell
	if err := sess.Shell(); err != nil {
		return Result{Success: false, Error: "启动交互式 shell 失败: " + err.Error(), Change: false}
	}

	// 5. 禁用分页（stdin 为 FIFO，pagingDisable 会先于命令被 shell 消费）
	fmt.Fprintln(stdin, pagingDisable)

	// 6. 一次性写入所有命令 + exit（不依赖固定 sleep 猜测时序；
	//    shell 处理完 exit 后退出，Wait() 返回即输出完整）
	for _, c := range commands {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		fmt.Fprintln(stdin, c)
	}
	fmt.Fprintln(stdin, "exit")

	// 7. 等待会话结束，兜底超时防止设备卡死（如命令触发交互确认）
	waitTimeout := time.Duration(config.GlobalConfig.DefaultSSHTimeout) * time.Second
	if waitTimeout <= 0 {
		waitTimeout = 10 * time.Second
	}
	waitDone := make(chan struct{})
	go func() {
		sess.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(waitTimeout):
		return Result{
			Success: false,
			Output:  CleanTerminalOutput(outputBuf.String()),
			Error:   "等待交换机输出超时",
			Change:  false,
		}
	}

	return Result{
		Success: true,
		Output:  CleanTerminalOutput(outputBuf.String()),
		Error:   "",
		Change:  true,
	}
}

// splitSemicolon 按 ; 分割命令，支持转义分号（\; 不分割）
// 例: "a\;b;c" → ["a\;b", "c"]；"a;b;c" → ["a", "b", "c"]
func splitSemicolon(cmd string) []string {
	var parts []string
	var cur strings.Builder
	escaped := false
	for i := 0; i < len(cmd); i++ {
		ch := cmd[i]
		if escaped {
			cur.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			// 保留反斜杠本身，标记转义下一个字符（避免 ";" 被分割，但保留 \; 形式给交换机）
			cur.WriteByte(ch)
			escaped = true
			continue
		}
		if ch == ';' {
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}
	parts = append(parts, strings.TrimSpace(cur.String()))
	return parts
}

func init() {
	Register("shell", NewShellModule) // 注册 "shell" 模块
}
