package utils

import (
	"os"
	"reflect"
	"testing"
)

func TestExpandHostRange(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
		hasRange bool
	}{
		{"无区间", "node-100", nil, false},
		{"普通展开", "node-[100:102]", []string{"node-100", "node-101", "node-102"}, true},
		{"IP段展开", "192.168.10.[100:101]", []string{"192.168.10.100", "192.168.10.101"}, true},
		{"步长展开", "node-[100:105:2]", []string{"node-100", "node-102", "node-104"}, true},
		{"零填充", "node-[01:03]", []string{"node-01", "node-02", "node-03"}, true},
		{"多段笛卡尔积", "node-[1:2]-[3:4]", []string{"node-1-3", "node-1-4", "node-2-3", "node-2-4"}, true},
		{"区间在中间", "node-[100:101]-db", []string{"node-100-db", "node-101-db"}, true},
		{"起止相等", "q-[5:5]", []string{"q-5"}, true},
		{"非法区间反向", "q-[105:100]", []string{"q-[105:100]"}, true},
		{"非法步长", "q-[1:5:0]", []string{"q-[1:5:0]"}, true},
		{"主机名含方括号不误判", "a[b]c", nil, false},
		{"主机名含冒号不误判", "node-1:2", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hasRange := ExpandHostRange(tt.input)
			if hasRange != tt.hasRange {
				t.Errorf("hasRange = %v, want %v", hasRange, tt.hasRange)
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ExpandHostRange(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExpandHostRanges(t *testing.T) {
	input := []string{"all", "node-[100:101]", "master"}
	got := ExpandHostRanges(input)
	expected := []string{"all", "node-100", "node-101", "master"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ExpandHostRanges(%v) = %v, want %v", input, got, expected)
	}
}

func TestParseHostsFileRange(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/host"
	content := "[master]\nnode-[100:102] user=root port=22\n10.0.0.1\n"
	if err := writeTestFile(path, content); err != nil {
		t.Fatal(err)
	}

	groups, err := ParseHostsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("组数量 = %d, want 1", len(groups))
	}
	hosts := groups[0].Hosts
	if len(hosts) != 4 {
		t.Fatalf("展开后主机数 = %d, want 4", len(hosts))
	}
	if hosts[0].Address != "node-100" || hosts[2].Address != "node-102" {
		t.Errorf("展开结果错误: %v %v %v", hosts[0].Address, hosts[1].Address, hosts[2].Address)
	}
	if hosts[0].Params["user"] != "root" {
		t.Errorf("参数未继承: %v", hosts[0].Params)
	}
}

func TestInventoryRangeExpansion(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/host"
	content := "[node]\nnode-100\nnode-101\nnode-102\n"
	if err := writeTestFile(path, content); err != nil {
		t.Fatal(err)
	}
	groups, err := ParseHostsFile(path)
	if err != nil {
		t.Fatal(err)
	}

	_, hosts, err := Inventory([]string{"node-[100:101]"}, groups)
	if err != nil {
		t.Fatal(err)
	}
	got := Filter(nil, hosts)
	if len(got) != 2 {
		t.Fatalf("过滤后主机数 = %d, want 2", len(got))
	}
	if got[0].Address != "node-100" || got[1].Address != "node-101" {
		t.Errorf("展开结果错误: %v %v", got[0].Address, got[1].Address)
	}
}

// 区间展开统一参数后，单行声明例外主机覆盖参数（后者优先）
func TestDeduplicateHostsLaterOverrides(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/host"
	content := "[master]\nnode-[100:101] user=root password=common\nnode-101 password=special\n"
	if err := writeTestFile(path, content); err != nil {
		t.Fatal(err)
	}
	groups, err := ParseHostsFile(path)
	if err != nil {
		t.Fatal(err)
	}

	inventory, _, err := Inventory([]string{"master"}, groups)
	if err != nil {
		t.Fatal(err)
	}
	got := Filter(inventory, nil)
	if len(got) != 2 {
		t.Fatalf("主机数 = %d, want 2", len(got))
	}
	for _, h := range got {
		if h.Address == "node-101" {
			if h.Params["password"] != "special" {
				t.Errorf("node-101 密码 = %q, want special（后者覆盖未生效）", h.Params["password"])
			}
		} else if h.Params["password"] != "common" {
			t.Errorf("%s 密码 = %q, want common", h.Address, h.Params["password"])
		}
	}
}

func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
