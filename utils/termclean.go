package utils

import "strings"

// CleanTerminalOutput 清洗交互式终端（PTY）输出，使其适合管道/日志消费。
// 交换机/服务器交互 shell 的原始输出包含终端控制序列，grep/awk 等管道工具
// 会因 ESC 字符误判为二进制文件（"Binary file matches"）、因 \r 导致行粘连。
//
// 清洗项目：
//  1. ESC[...  ANSI 转义序列（颜色、光标移动等）
//  2. \r\n → \n（统一换行符）
//  3. 裸 \r → \n（部分设备只发 \r 换行）
//
// 注意：不清洗提示符（如 <H3C>、root@host:~#），命令输出本身可能含这些字符。
func CleanTerminalOutput(s string) string {
	if s == "" {
		return s
	}

	// 1. 删除所有 ANSI 转义序列（ESC + 可含参数 + 终结符）
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b { // ESC
			// OSC (ESC ] ... BEL/ST) 单独处理：读到 BEL(0x07) 或 ESC\（ST）
			if i+1 < len(s) && s[i+1] == ']' {
				i += 2
				for i < len(s) {
					if s[i] == 0x07 || (s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\') {
						break
					}
					i++
				}
				// 消耗 BEL 或 ST
				if i < len(s) {
					if s[i] == 0x1b {
						i++ // 消费 ESC（ST 的 ESC 部分，后面 \ 由下次循环跳过）
					}
				}
				continue
			}
			// CSI (ESC [ ... 字母/@) 及单字符 ESC 序列（ESC M、ESC 7、ESC 8 等）
			i++ // 跳过 ESC
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) {
					c := s[i]
					if c >= 0x40 && c <= 0x7e { // 终结符 @A-Z[\]^_`a-z{|}~
						break
					}
					i++
				}
			}
			continue
		}
		b.WriteByte(s[i])
	}

	// 2. 规范化换行符：\r\n → \n，裸 \r → \n
	cleaned := b.String()
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")

	return cleaned
}