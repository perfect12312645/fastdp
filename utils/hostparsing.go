package utils

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
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
func ParseHostsFile(path string) ([]*HostGroup, error) {
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

		// 支持主机区间展开：node-[100:105] → node-100 ~ node-105
		expandedAddrs, hasRange := ExpandHostRange(hostAddress)
		if hasRange {
			for _, addr := range expandedAddrs {
				currentGroup.Hosts = append(currentGroup.Hosts, &Host{
					Address: addr,
					Params:  copyHostParams(hostParams),
				})
			}
		} else {
			// 添加到当前组
			currentGroup.Hosts = append(currentGroup.Hosts, &Host{
				Address: hostAddress,
				Params:  hostParams,
			})
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("扫描文件失败: %v", err)
	}

	return groups, nil
}

// 获取将要执行的主机组
func Inventory(Args []string, groups []*HostGroup) ([]*HostGroup, []*Host, error) {
	if len(Args) == 0 {
		return nil, nil, fmt.Errorf("请指定目标主机组")
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

	// 参数支持区间展开：node-[100:105] / 192.168.10.[100:101]
	args := ExpandHostRanges(Args)

	for _, arg := range args {
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
		return groups, nil, nil
	}

	return inventory, hosts, nil
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

// deduplicateHosts 去重，后者覆盖前者：
// 区间展开（node-[100:105] password=a）后，可单独一行声明例外主机覆盖参数
// （node-103 password=b），后声明优先
func deduplicateHosts(hosts []*Host) []*Host {
	result := make([]*Host, 0, len(hosts))
	index := make(map[string]int)
	for _, host := range hosts {
		if host == nil {
			continue
		}
		if i, ok := index[host.Address]; ok {
			result[i] = host
		} else {
			index[host.Address] = len(result)
			result = append(result, host)
		}
	}
	return result
}

// hostRangeRegex 匹配 [start:end:step] 区间表达式，仅支持数字
// 示例: [100:105] [100:105:2] [01:04]
var hostRangeRegex = regexp.MustCompile(`\[(\d+):(\d+)(?::(\d+))?\]`)

// maxRangeExpand 单个区间的最大展开数量（防止误输入造成爆炸性展开）
const maxRangeExpand = 10000

// ExpandHostRanges 展开参数列表中的区间表达式，如:
//
//	fastdp shell -a 'uptime' 'node-[100:105]'   → node-100 node-101 ... node-105
//	fastdp shell -a 'uptime' '192.168.10.[100:101]' → 192.168.10.100 192.168.10.101
//	fastdp shell -a 'uptime' 'node-[01:04]'        → node-01 node-02 node-03 node-04
//
// 不含区间的参数原样返回。
func ExpandHostRanges(args []string) []string {
	var result []string
	for _, arg := range args {
		expanded, ok := ExpandHostRange(arg)
		if ok {
			result = append(result, expanded...)
		} else {
			result = append(result, arg)
		}
	}
	return result
}

// ExpandHostRange 展开单个参数中的区间表达式。
// 支持一个参数内多个区间（按笛卡尔积展开），如 node-[1:2]-[3:4]。
// 返回值: (展开后的主机列表, 是否包含区间表达式)
func ExpandHostRange(arg string) ([]string, bool) {
	matches := hostRangeRegex.FindAllStringSubmatchIndex(arg, -1)
	if len(matches) == 0 {
		return nil, false
	}

	// 解析每个区间的展开值
	values := make([][]string, 0, len(matches))
	for _, m := range matches {
		stepStr := "1"
		if m[6] >= 0 {
			stepStr = arg[m[6]:m[7]]
		}
		vs, ok := expandRangeValues(arg[m[2]:m[3]], arg[m[4]:m[5]], stepStr)
		if !ok {
			// 非法区间（如 [105:100] 或 [1:5:0]），按字面量处理
			return []string{arg}, true
		}
		values = append(values, vs)
	}

	// 按区间在原串中的位置做笛卡尔积替换
	var result []string
	var build func(pos int, acc string)
	build = func(pos int, acc string) {
		if pos == len(matches) {
			result = append(result, acc+arg[matches[len(matches)-1][1]:])
			return
		}
		segStart := 0
		if pos > 0 {
			segStart = matches[pos-1][1]
		}
		seg := arg[segStart:matches[pos][0]]
		for _, v := range values[pos] {
			build(pos+1, acc+seg+v)
		}
	}
	build(0, "")
	return result, true
}

// expandRangeValues 解析 [start:end:step] 的展开值列表。
// 零填充：起始值带前导零时按起始值宽度填充（01 → 01,02,...）。
// 非法区间（start>end 或 step<=0）返回 ok=false。
func expandRangeValues(startStr, endStr, stepStr string) ([]string, bool) {
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	step, err3 := strconv.Atoi(stepStr)
	if err1 != nil || err2 != nil || err3 != nil || step <= 0 || end < start {
		return nil, false
	}

	width := 0
	if len(startStr) > 1 && startStr[0] == '0' {
		width = len(startStr)
	}

	count := (end-start)/step + 1
	if count > maxRangeExpand {
		count = maxRangeExpand
	}
	vs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		v := start + i*step
		if width > 0 {
			vs = append(vs, fmt.Sprintf("%0*d", width, v))
		} else {
			vs = append(vs, strconv.Itoa(v))
		}
	}
	return vs, true
}

// copyHostParams 深拷贝主机参数表
func copyHostParams(params map[string]string) map[string]string {
	cp := make(map[string]string, len(params))
	for k, v := range params {
		cp[k] = v
	}
	return cp
}
