package utils

import (
	"reflect"
	"testing"
)

func TestCompareAddr(t *testing.T) {
	cases := []struct {
		a, b string
		want int // -1 / 0 / 1
	}{
		// IPv4 数值比较（netip）
		{"192.168.1.2", "192.168.1.19", -1},
		{"192.168.1.19", "192.168.1.2", 1},
		{"192.168.1.9", "192.168.1.10", -1},
		{"10.0.0.255", "10.0.1.0", -1},
		{"192.168.1.2", "192.168.1.2", 0},
		{"10.0.0.1", "192.168.1.1", -1},
		{"192.168.1.1", "192.168.0.255", 1},
		// IPv4 < IPv6（netip 按地址长度先比）
		{"10.0.0.1", "::1", -1},
		{"::1", "10.0.0.1", 1},
		// IPv6 数值比较
		{"::1", "::2", -1},
		{"2001:db8::1", "2001:db8::10", -1},
		// 域名自然排序：数字段按数值
		{"node-2", "node-10", -1},
		{"node-10", "node-9", 1},
		{"server2", "server10", -1},
		{"a.example.com", "b.example.com", -1},
		// 自然排序：数值相同则前导零少的在前
		{"node-2", "node-02", -1},
		// 混合：IP 恒在域名前
		{"192.168.1.2", "a.example.com", -1},
		{"zpf", "10.0.0.1", 1},
		// netip 拒绝的写法（前导零）归为非 IP → 域名区，排在合法 IP 后
		{"192.168.001.2", "10.0.0.1", 1},
		// 非法/空写法同归域名区，走自然排序
		{"", "10.0.0.1", 1},
		{"192.168.1", "10.0.0.1", 1},
		{"192.168.1.256", "192.168.1.299", -1}, // 均非法 → 域名区自然排序，数字段数值比较
	}
	for _, c := range cases {
		if got := CompareAddr(c.a, c.b); got != c.want {
			t.Errorf("CompareAddr(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSortAddrs(t *testing.T) {
	in := []string{
		"node-19",
		"192.168.1.19",
		"zpf",
		"192.168.1.2",
		"node-2",
		"192.168.1.10",
		"10.0.0.100",
		"192.168.1.9",
		"10.0.0.2",
		"web-10",
		"web-9",
	}
	want := []string{
		"10.0.0.2",
		"10.0.0.100",
		"192.168.1.2",
		"192.168.1.9",
		"192.168.1.10",
		"192.168.1.19",
		"node-2",
		"node-19",
		"web-9",
		"web-10",
		"zpf",
	}
	SortAddrs(in)
	if !reflect.DeepEqual(in, want) {
		t.Errorf("SortAddrs = %v, want %v", in, want)
	}
}

// TestCompareAddr_Transitivity 随机样本两两比较，验证排序全序（无 a<b、b<c、a>c 环）
func TestCompareAddr_Transitivity(t *testing.T) {
	addrs := []string{
		"192.168.1.2", "192.168.1.19", "10.0.0.1", "::1", "2001:db8::1",
		"node-2", "node-10", "zpf", "a.example.com", "192.168.001.2", "", "web9",
	}
	for _, a := range addrs {
		for _, b := range addrs {
			for _, c := range addrs {
				if CompareAddr(a, b) < 0 && CompareAddr(b, c) < 0 && CompareAddr(a, c) >= 0 {
					t.Errorf("非传递: %q < %q < %q 但 %q >= %q", a, b, c, a, c)
				}
			}
		}
	}
}
