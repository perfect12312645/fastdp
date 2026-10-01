package utils

import (
	"net/netip"
	"sort"
	"strings"
)

// CompareAddr 比较两个主机地址，用于结果/列表排序。
// 规则（保证全序传递性）：
//  1. 双方均为合法 IP → netip 数值比较（192.168.1.2 < 192.168.1.19，IPv4 < IPv6）
//  2. 双方均非 IP（域名等）→ 自然排序（node-2 < node-10，数字段按数值比）
//  3. 一方为 IP 一方为域名 → IP 在前（与旧字典序"数字<字母"的观感一致）
// 规则 3 存在的必要性：若混合时回退字典序会出现 a<b、b<c、a>c 的非传递比较环。
func CompareAddr(a, b string) int {
	aa, aerr := netip.ParseAddr(a)
	bb, berr := netip.ParseAddr(b)
	switch {
	case aerr == nil && berr == nil:
		return aa.Compare(bb)
	case aerr == nil:
		return -1 // IP 在域名前
	case berr == nil:
		return 1
	default:
		return naturalCompare(a, b)
	}
}

// SortAddrs 按 IP 数值语义排序（域名按自然排序排在其后）
func SortAddrs(addrs []string) {
	sort.SliceStable(addrs, func(i, j int) bool {
		return CompareAddr(addrs[i], addrs[j]) < 0
	})
}

// naturalCompare 自然排序：连续数字段按数值比较（a2 < a10），其余按字节序
func naturalCompare(a, b string) int {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		if isDigit(a[ia]) && isDigit(b[ib]) {
			ea := ia
			for ea < len(a) && isDigit(a[ea]) {
				ea++
			}
			eb := ib
			for eb < len(b) && isDigit(b[eb]) {
				eb++
			}
			if c := compareNumRun(a[ia:ea], b[ib:eb]); c != 0 {
				return c
			}
			ia, ib = ea, eb
			continue
		}
		if a[ia] != b[ib] {
			if a[ia] < b[ib] {
				return -1
			}
			return 1
		}
		ia++
		ib++
	}
	return len(a[ia:]) - len(b[ib:])
}

// compareNumRun 比较两段纯数字：先比数值（去前导零后比位数再比字典），数值相同则前导零少的在前
func compareNumRun(x, y string) int {
	sx, sy := strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
	if len(sx) != len(sy) {
		if len(sx) < len(sy) {
			return -1
		}
		return 1
	}
	if sx != sy {
		if sx < sy {
			return -1
		}
		return 1
	}
	// 数值相同（如 "2" vs "02"）：前导零少的在前
	if len(x) != len(y) {
		if len(x) < len(y) {
			return -1
		}
		return 1
	}
	return 0
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
