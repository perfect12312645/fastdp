package utils

import "testing"

func TestCleanTerminalOutput(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bracketed paste 序列", "\x1b[?2004hline1\x1b[?2004l\r\n", "line1\n"},
		{"CRLF 规范化", "a\r\nb\r\n", "a\nb\n"},
		{"裸 CR", "a\rb", "a\nb"},
		{"ANSI 颜色", "\x1b[31mred\x1b[0m", "red"},
		{"光标移动", "\x1b[2J\x1b[Hclean", "clean"},
		{"OSC 序列", "\x1b]0;title\x07text", "text"},
		{"混合", "x\x1b[?2004h\n\ryz\x1b[0m\r\n", "x\n\nyz\n"},
		{"空输入", "", ""},
		{"无控制字符", "plain text\n", "plain text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanTerminalOutput(tt.in); got != tt.want {
				t.Errorf("CleanTerminalOutput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}