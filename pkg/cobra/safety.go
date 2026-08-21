package cobra

import (
	. "fastdp/utils"
	"fmt"
)

// enforceCommandSafety 命令安全门禁：
//   - 硬拦截命令（纯破坏性）默认禁止执行，--allow-dangerous 显式放行
//   - 危险命令（有合法场景）交互确认，--yes 跳过确认
//
// 返回 false 表示禁止执行。
func enforceCommandSafety(command string, hosts []*Host, yes, allowDangerous bool) bool {
	res := CheckCommandSafety(command)
	target := HostCountDesc(hosts)

	switch res.Level {
	case SafetyBlocked:
		if allowDangerous {
			Unchangedf("⛔ 危险命令 %q 已由 --allow-dangerous 显式放行", res.Match)
			return true
		}
		Errorf("⛔ 危险命令已拦截")
		Errorf("  命令: %s", command)
		Errorf("  匹配规则: %s", res.Rule)
		Errorf("  目标: %s", target)
		Errorf("  已禁止执行。如需放行请使用 --allow-dangerous 显式覆盖（不建议）")
		return false
	case SafetyConfirm:
		if yes {
			Unchangedf("⚠️ 危险命令 %q 已由 --yes 自动确认", res.Match)
			return true
		}
		Errorf("⚠️ 检测到危险命令，需要确认")
		Errorf("  命令: %s", command)
		Errorf("  匹配规则: %s", res.Rule)
		Errorf("  目标: %s", target)
		confirmed := ConfirmExecution("确认在这些机器上执行该命令?")
		if !confirmed {
			Errorf("已取消执行")
		}
		return confirmed
	default:
		return true
	}
}

// HostCountDesc 生成 "N 台机器 [清单]" 描述
func HostCountDesc(hosts []*Host) string {
	return fmt.Sprintf("%d 台机器 %s", len(hosts), HostListDesc(hosts))
}
