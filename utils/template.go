package utils

import (
	"net"
	"strings"
)

// ReplaceTemplate 替换命令中的模板变量为实际值
// 支持 {{.addr}} {{.ip}} {{.port}} {{.user}}
func ReplaceTemplate(s string, hs HostSession) string {
	// addr = host 文件原值
	addr := hs.Addr

	// ip = 实际 IP（已是 IP 直接返回，否则 DNS 解析）
	ip := resolveIP(hs.Addr)

	// port
	port := "22"
	if h, p, err := net.SplitHostPort(hs.Addr); err == nil && p != "" {
		port = p
		_ = h
	} else if idx := strings.LastIndex(hs.Addr, ":"); idx >= 0 && strings.Count(hs.Addr, ":") == 1 {
		port = hs.Addr[idx+1:]
	}

	// user
	user := getCurrentUser(hs)

	s = strings.ReplaceAll(s, "{{.addr}}", addr)
	s = strings.ReplaceAll(s, "{{.ip}}", ip)
	s = strings.ReplaceAll(s, "{{.port}}", port)
	s = strings.ReplaceAll(s, "{{.user}}", user)

	return s
}

// resolveIP 解析地址为 IP：已是 IP 直接返回，否则查 /etc/hosts + DNS
func resolveIP(addr string) string {
	// 分离 host:port
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	} else if idx := strings.LastIndex(addr, ":"); idx >= 0 && strings.Count(addr, ":") == 1 {
		host = addr[:idx]
	}

	// 已是 IP（IPv4/IPv6）
	if net.ParseIP(host) != nil {
		return host
	}

	// 查 /etc/hosts + DNS
	ips, err := net.LookupIP(host)
	if err == nil && len(ips) > 0 {
		return ips[0].String()
	}

	// 解析失败，返回原值
	return host
}

// getCurrentUser 从 SSH 配置中获取用户名
func getCurrentUser(hs HostSession) string {
	if hs.Client != nil && hs.Client.User() != "" {
		return hs.Client.User()
	}
	return "root"
}
