package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"
)

// SafetyLevel 命令安全等级
type SafetyLevel int

const (
	SafetySafe    SafetyLevel = iota // 安全，直接执行
	SafetyConfirm                    // 危险但有合法应用场景，需确认后执行
	SafetyBlocked                    // 纯破坏性命令，硬拦截
)

// SafetyResult 安全检查结果
type SafetyResult struct {
	Level SafetyLevel
	Rule  string // 命中的规则描述
	Match string // 命中的原文片段
}

// 硬拦截规则（无合法应用场景、纯破坏性命令）
var blockedPatterns = []struct {
	rule  string
	regex *regexp.Regexp
}{
	// rm -rf / 及变体（rm -rf /、rm -fr /*、rm -rf / *、rm -rf /.*、rm -rf --no-preserve-root /）
	{"根目录递归删除", regexp.MustCompile(`\brm\s+(?:-(?:[a-zA-Z]*[rf][a-zA-Z]*|-[a-z-]+)\s+)*?(?:/|/\*|/\.\*?)(?:\s|$|[|;&\n])`)},
	// fork 炸弹
	{"fork炸弹", regexp.MustCompile(`:\(\)\s*\{`)},
	// dd 直接写入块设备（磁盘直写，不可恢复）
	{"dd写入块设备", regexp.MustCompile(`\bdd\b[^|;&\n]*\bof=/dev/(?:sd|nvme|vd|hd|mmcblk|xvd|disk/)[a-zA-Z0-9_./-]*`)},
	// 重定向到块设备（echo x > /dev/sda 等）
	{"重定向到块设备", regexp.MustCompile(`(?m)(?:^|[;&|()\s])(?:[0-9]?&?>>?|[0-9]?>)+\s*/dev/(?:sd|nvme|vd|hd|mmcblk|xvd|disk/)[a-zA-Z0-9_./-]*`)},
	// 对根目录递归 chmod / chown
	{"对根目录递归chmod", regexp.MustCompile(`\bchmod\s+-R\s*[0-7]{3,4}\s*(?:/|/\*)(?:\s|$|[|;&\n])`)},
	{"对根目录递归chown", regexp.MustCompile(`\bchown\s+-R\s+\S+\s*(?:/|/\*)(?:\s|$|[|;&\n])`)},
	// kill -9 1（杀死 init）
	{"kill PID 1", regexp.MustCompile(`\bkill\s+(?:-[0-9A-Za-z]+\s+)*?1\b`)},
}

// 需确认规则（危险但有合法应用场景）
var confirmPatterns = []struct {
	rule  string
	regex *regexp.Regexp
}{
	// rm -rf 递归删除（非根级，如 rm -rf /tmp/*）
	{"rm递归删除", regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*r[a-zA-Z]*\s*|--recursive\s*)`)},
	// 关机 / 重启 / 切换运行级别
	{"关机/重启", regexp.MustCompile(`\b(?:shutdown|reboot|poweroff|halt)\b|\b(?:init|telinit)\s+[06]\b|\bsystemctl\s+(?:poweroff|reboot|halt|suspend|hibernate)\b`)},
}

// CheckCommandSafety 检查命令的安全等级（先查硬拦截，再查需确认）
func CheckCommandSafety(command string) SafetyResult {
	for _, p := range blockedPatterns {
		if loc := p.regex.FindStringIndex(command); loc != nil {
			return SafetyResult{
				Level: SafetyBlocked,
				Rule:  p.rule,
				Match: strings.TrimSpace(command[loc[0]:loc[1]]),
			}
		}
	}
	for _, p := range confirmPatterns {
		if loc := p.regex.FindStringIndex(command); loc != nil {
			return SafetyResult{
				Level: SafetyConfirm,
				Rule:  p.rule,
				Match: strings.TrimSpace(command[loc[0]:loc[1]]),
			}
		}
	}
	return SafetyResult{Level: SafetySafe}
}

// ConfirmExecution 交互确认，返回是否确认执行
func ConfirmExecution(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}

// HostListDesc 生成主机列表描述（超过3台折叠为前3台 + 省略号）
func HostListDesc(hosts []*Host) string {
	if len(hosts) == 0 {
		return "(无)"
	}
	names := make([]string, 0, 3)
	for _, h := range hosts {
		names = append(names, h.Address)
		if len(names) == 3 {
			break
		}
	}
	list := "[" + strings.Join(names, ", ")
	if len(hosts) > 3 {
		list += ", ..."
	}
	return list + "]"
}

// WriteHistory 写入单条执行历史日志（JSON 元数据，不包含命令输出），写入失败不影响主流程
func WriteHistory(path, entry string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry + "\n")
}

// BuildHistoryEntry 构造单行 JSON 执行历史条目（不包含命令输出）
func BuildHistoryEntry(command string, hosts []string, ok, failed, changed, unchanged int, duration time.Duration) string {
	userName := "unknown"
	if u, err := user.Current(); err == nil {
		userName = u.Username
	} else if envUser := os.Getenv("USER"); envUser != "" {
		userName = envUser
	}

	entry := struct {
		Time       string `json:"time"`
		User       string `json:"user"`
		Command    string `json:"command"`
		Hosts      string `json:"hosts"`
		OK         int    `json:"ok"`
		Failed     int    `json:"failed"`
		Changed    int    `json:"changed"`
		Unchanged  int    `json:"unchanged"`
		DurationMS int64  `json:"duration_ms"`
	}{
		Time:       time.Now().Format(time.RFC3339),
		User:       userName,
		Command:    command,
		Hosts:      strings.Join(hosts, ","),
		OK:         ok,
		Failed:     failed,
		Changed:    changed,
		Unchanged:  unchanged,
		DurationMS: duration.Milliseconds(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return ""
	}
	return string(data)
}
