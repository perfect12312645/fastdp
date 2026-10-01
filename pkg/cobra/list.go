package cobra

import (
	"encoding/json"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/spf13/cobra"
)

// hostView 主机展示信息（脱敏，不含密码原文）
type hostView struct {
	Address string            `json:"address"`
	Params  map[string]string `json:"params,omitempty"`
}

// groupView 分组展示信息
type groupView struct {
	Name   string      `json:"name"`
	Count  int         `json:"count"`
	Hosts  []hostView  `json:"hosts"`
}

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "查看主机组与机器列表",
	Aliases: []string{"hosts", "groups"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		groupName, _ := cmd.Flags().GetString("group")
		showPassword := config.GlobalFlags.Debug

		// 读取 host 清单
		hostInventory := config.GlobalConfig.HostInventory
		if hostInventory == "" {
			Errorf("配置文件中未配置 host_inventory（主机清单路径），请先配置")
			os.Exit(exitcode.ParamError)
		}
		groups, err := ParseHostsFile(hostInventory)
		if err != nil {
			Errorf("解析 host 文件失败: %v", err)
			os.Exit(exitcode.ParamError)
		}

		// 过滤：只看指定组
		if groupName != "" {
			var filtered []*HostGroup
			for _, g := range groups {
				if g.Name == groupName {
					filtered = append(filtered, g)
				}
			}
			if len(filtered) == 0 {
				Errorf("未找到主机组 %q（可用分组可通过 fastdp list 查看）", groupName)
				os.Exit(exitcode.ParamError)
			}
			groups = filtered
		}

		// 统计去重后的有效主机（跨组重复只算一次）
		allHosts := make([]*Host, 0)
		for _, g := range groups {
			allHosts = append(allHosts, g.Hosts...)
		}
		deduped := deduplicateHostsForDisplay(allHosts)
		totalUnique := len(deduped)

		// === JSON 输出（复用全局 -o json）===
		if config.GlobalFlags.Output == "json" {
			views := make([]groupView, 0, len(groups))
			for _, g := range groups {
				hosts := make([]hostView, 0, len(g.Hosts))
				for _, h := range g.Hosts {
					p := make(map[string]string)
					for k, v := range h.Params {
						if k == "password" && !showPassword {
							continue // 密码默认不展示
						}
						p[k] = v
					}
					hosts = append(hosts, hostView{Address: h.Address, Params: p})
				}
				views = append(views, groupView{Name: g.Name, Count: len(g.Hosts), Hosts: hosts})
			}
			out := map[string]interface{}{
				"groups": views,
				"total_unique_hosts": totalUnique,
			}
			jsonData, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(jsonData))
			return
		}

		// === 文本输出 ===
		fmt.Printf("主机清单: %s\n\n", hostInventory)
		for _, g := range groups {
			fmt.Printf("[%s]  (%d 台)\n", g.Name, len(g.Hosts))
			// 组内按地址排序
			hosts := append([]*Host(nil), g.Hosts...)
			sort.Slice(hosts, func(i, j int) bool { return CompareAddr(hosts[i].Address, hosts[j].Address) < 0 })
			for _, h := range hosts {
				desc := h.Address
				user := h.Params["user"]
				if user == "" {
					user = config.GlobalConfig.DefaultSSHUser
				}
				if user == "" {
					user = "root"
				}
				port := h.Params["port"]
				if port == "" {
					port = strconv.Itoa(config.GlobalConfig.DefaultSSHPort)
				}
				if user != "" {
					desc += fmt.Sprintf("  (user=%s, port=%s)", user, port)
				}
				if showPassword && h.Params["password"] != "" {
					desc += fmt.Sprintf(", password=%s", h.Params["password"])
				}
				fmt.Printf("  %s\n", desc)
			}
			fmt.Println()
		}
		fmt.Printf("共 %d 个分组，去重后有效主机 %d 台\n", len(groups), totalUnique)
	},
	Example: `
  # 列出所有分组及机器
  fastdp list

  # 只看 web 组
  fastdp list -g web

  # JSON 输出（AI Agent 友好）
  fastdp list -o json

  # 调试模式显示密码
  fastdp list -g web -v
`,
}

// deduplicateHostsForDisplay 跨组去重计数（只统计唯一主机地址数，不改动分组结构）
func deduplicateHostsForDisplay(hosts []*Host) []*Host {
	seen := make(map[string]bool)
	result := make([]*Host, 0, len(hosts))
	for _, h := range hosts {
		if h == nil {
			continue
		}
		if seen[h.Address] {
			continue
		}
		seen[h.Address] = true
		result = append(result, h)
	}
	return result
}

func init() {
	listCmd.Flags().StringP("group", "g", "", "只看指定主机组")
}