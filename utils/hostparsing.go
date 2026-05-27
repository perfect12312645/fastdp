package utils

import (
	"bufio"
	. "fastdp/pkg/log"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// 定义配置结构：主机组包含多个主机，每个主机有参数
type Host struct {
	Address string            // 主机地址（IP/域名）
	Params  map[string]string // 主机参数（user、port、password 等）
}

type HostGroup struct {
	Name  string  // 组名（如 centos）
	Hosts []*Host // 组内主机列表
}

// 解析 fastdp 的主机配置文件
func parseHostsFile(path string) ([]*HostGroup, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	var groups []*HostGroup
	var currentGroup *HostGroup

	// 正则：匹配 [组名] 格式（如 [centos]）
	groupRegex := regexp.MustCompile(`^\[(.+)\]$`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 检查是否是组定义行（如 [centos]）
		if groupMatch := groupRegex.FindStringSubmatch(line); groupMatch != nil {
			groupName := groupMatch[1]
			// 创建新组，并设为当前组
			currentGroup = &HostGroup{
				Name:  groupName,
				Hosts: []*Host{},
			}
			groups = append(groups, currentGroup)
			continue
		}

		// 不是组行：解析主机地址和参数
		if currentGroup == nil {
			// 如果还没遇到组定义，默认放到 "default" 组
			currentGroup = &HostGroup{
				Name:  "default",
				Hosts: []*Host{},
			}
			groups = append(groups, currentGroup)
		}

		// 分割行：第一个元素是主机地址，剩下的是 key=value 参数
		parts := strings.Fields(line) // 按空格分割（自动处理多个空格）

		hostAddress := parts[0]
		hostParams := make(map[string]string)

		// 解析参数（key=value）
		for _, part := range parts[1:] {
			kv := strings.SplitN(part, "=", 2) // 只按第一个 = 分割
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.TrimSpace(kv[1])
				hostParams[key] = value
			} else {
				// 忽略无效参数（如没有=的情况）
				fmt.Printf("警告：无效参数格式 %q，已忽略\n", part)
			}
		}

		// 添加到当前组
		currentGroup.Hosts = append(currentGroup.Hosts, &Host{
			Address: hostAddress,
			Params:  hostParams,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描文件失败: %v", err)
	}

	return groups, nil
}

func HostParse(host string) ([]*HostGroup, error) {
	// 解析配置文件（假设文件名为 fastdp.hosts）
	groups, err := parseHostsFile(host)
	if err != nil {
		return nil, err
	}

	// 打印解析结果
	for _, group := range groups {
		Logger.Sugar().Debugf("主机组:%s", group.Name)
		for _, host := range group.Hosts {
			if _, ok := host.Params["port"]; !ok {
				host.Params["port"] = "22" // 默认端口
			}
			if _, ok := host.Params["user"]; !ok {
				host.Params["user"] = "root" // 默认用户
			}
			Logger.Sugar().Debugf("主机:%s,用户名:%s,ssh端口:%s,密码:%s", host.Address, host.Params["user"], host.Params["port"], host.Params["password"])
		}
	}
	return groups, err
}

// 获取将要执行的主机组
func Inventory(Args []string, groups []*HostGroup) ([]*HostGroup, []*Host) {
	if len(Args) == 0 {
		Errorf("ERROR: 请指定目标主机组")
	}

	// 优化 groupMap：键为组名，值为对应的 HostGroup 实例（便于快速查找）
	groupMap := make(map[string]*HostGroup)
	hostMap := make(map[string]*Host)
	for _, g := range groups {
		groupMap[g.Name] = g // 存储组名到 HostGroup 的映射
		for _, host := range g.Hosts {
			hostMap[host.Address] = host
		}
	}

	var inventory []*HostGroup
	var hosts []*Host
	hasAll := false

	for _, arg := range Args {
		if arg == "all" {
			hasAll = true
			break // 遇到 "all" 无需继续检查其他参数
		}

		// 从 groupMap 中查找对应的 HostGroup
		g, exists := groupMap[arg]
		if !exists {
			host, exists := hostMap[arg]
			if !exists {
				Errorf("忽略未知的主机组或主机: %s", arg)
				continue
			}
			hosts = append(hosts, host)
		}

		// 添加找到的 HostGroup 到结果中
		inventory = append(inventory, g)
	}

	// 如果指定了 "all"，直接返回所有主机组
	if hasAll {
		return groups, nil
	}

	return inventory, hosts
}
func Filter(groups []*HostGroup, hosts []*Host) []*Host {
	var allHosts []*Host
	if len(groups) > 0 {
		for _, group := range groups {
			if group == nil {
				continue // 跳过 nil 元素，避免访问 group.Hosts
			}
			allHosts = append(allHosts, group.Hosts...)
		}
	}
	if len(hosts) > 0 {
		allHosts = append(allHosts, hosts...)
	}
	// 对最终的主机列表进行去重
	allHosts = deduplicateHosts(allHosts)
	return allHosts
}

func deduplicateHosts(hosts []*Host) []*Host {
	seen := make(map[string]bool)
	var result []*Host

	for _, host := range hosts {
		if host != nil && !seen[host.Address] {
			seen[host.Address] = true
			result = append(result, host)
		}
	}

	return result
}
