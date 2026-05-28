package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/pkg/log"
	. "fastdp/utils"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var (
	Version = "v6.0.0"
)
var rootCmd = &cobra.Command{
	Use:   "fastdp",
	Short: "轻量级 Ansible 风格运维工具",
	Long:  `在指定主机组上执行运维操作，支持多模块管理。`,
	Example: `
  # 在 web 组执行命令
  fastdp shell -a "uptime" web

  # 同时指定多个主机组 + IP（非常灵活）
  fastdp shell -a "lsblk" master node 192.168.10.100

  # 批量检查主机信息
  fastdp check all

  # 本地文件复制到远程主机
  fastdp copy -s config.toml -d /tmp/ all

  # 批量拉取文件
  fastdp fetch -r "/remote/logs/*" all

  # 批量远程脚本
  fastdp script -f /tmp/check.sh all
`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		showVersion, _ := cmd.Flags().GetBool("version")
		if showVersion {
			fmt.Printf("fastdp version %s %s/%s\n",
				Version,
				runtime.GOOS,   // 自动获取：linux/darwin/windows
				runtime.GOARCH, // 自动获取：amd64/arm64
			)
			os.Exit(0)
		}
		// 读取全局参数
		config.GlobalFlags.Concurrency, _ = cmd.Flags().GetInt("concurrency")
		config.GlobalFlags.Debug, _ = cmd.Flags().GetBool("debug")
		inventoryPath, _ := cmd.Flags().GetString("inventory")
		if inventoryPath != "" {
			// 命令行传了，覆盖配置文件
			config.GlobalConfig.HostInventory = inventoryPath
		}

		// 初始化日志
		Logger = InitLogger(config.GlobalFlags.Debug)

		Logger.Debug("初始化完成",
			zap.Bool("debug", config.GlobalFlags.Debug),
			zap.Int("concurrency", config.GlobalFlags.Concurrency),
			zap.String("host文件", config.GlobalConfig.HostInventory))
	},
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help() // 显式输出帮助信息
		os.Exit(0)
	},
}

func init() {
	// 查找配置文件
	configPath := config.FindConfigFile()
	if configPath == "" {
		// 处理用户目录获取失败等错误（如无权限）
		fmt.Println("警告：获取配置路径/etc/fastdp/config.toml失败,将使用默认配置config.toml")
		configPath = "config.toml" // 兜底
	}

	// 解析配置
	_, err := config.ParseConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析配置失败：%v\n", err)
		os.Exit(1)
	}

	// 并发数兜底：配置文件为空/0 → 强制使用默认值 10
	concurrencyDefault := config.GlobalConfig.Concurrency
	if concurrencyDefault <= 0 {
		concurrencyDefault = 10
	}

	var debug bool
	if config.GlobalConfig.LogLevel == "debug" {
		debug = true
	}
	// 全局标志
	rootCmd.PersistentFlags().IntP("concurrency", "c", concurrencyDefault, "并发连接数")
	rootCmd.PersistentFlags().BoolP("debug", "v", debug, "是否开启调试模式")
	rootCmd.PersistentFlags().StringP("inventory", "i", "", "指定主机清单文件 (优先于配置文件)")
	rootCmd.PersistentFlags().BoolP("version", "V", false, "显示版本信息")

	// 添加子命令
	rootCmd.AddCommand(shellCmd, copyCmd, pingCmd, scriptCmd, checkCmd, fetchCmd)

}

// Execute 入口函数：外部 main 调用这个方法
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func GetInfo() ([]*Host, error) {

	hostInventory := config.GlobalConfig.HostInventory
	// 规则：配置文件必须显式配置 host_inventory，否则直接失败
	if hostInventory == "" {
		return nil, fmt.Errorf("配置文件中未配置 host_inventory（主机清单路径），请先配置")
	}

	Logger.Sugar().Debugf("生效的配置文件路径: %s", config.GlobalConfig.ConfigAbsPath)
	Logger.Sugar().Debugf("生效的机组文件路径: %s", hostInventory)

	// 主机清单解析
	groups, err := HostParse(hostInventory)
	if err != nil {
		return nil, fmt.Errorf("解析host文件失败: %w", err) // 错误往上抛
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("主机组为空，请配置host文件：%s", hostInventory)
	}

	// 获取将要执行的主机组
	inventory, hosts := Inventory(config.GlobalFlags.HostInventory, groups)
	execHosts := Filter(inventory, hosts)

	// 打印调试日志
	Logger.Debug("执行信息",
		zap.Any("主机组", inventory),
		zap.Any("主机列表", execHosts),
		zap.Any("参数", config.GlobalFlags.Parameter),
	)
	return execHosts, nil
}

func execute(hostSessions []HostSession, flags *config.Flags, mod module.Module) {
	// 使用并发执行命令
	var wg sync.WaitGroup                     // 用于等待所有goroutine完成
	var resultsMutex sync.Mutex               // 用于保护results的并发写入
	results := make(map[string]module.Result) // 存储每个主机的执行结果（key:主机地址，value:命令输出）
	// 创建有缓冲的channel来控制最大并发数
	maxConcurrency := config.GlobalFlags.Concurrency
	semaphore := make(chan struct{}, maxConcurrency)
	for _, hs := range hostSessions {
		wg.Add(1) // 每启动一个goroutine，WaitGroup计数器+1
		// 启动goroutine，传入当前的hs（主机会话）
		go func(hs HostSession) {
			defer wg.Done() // goroutine结束时，WaitGroup计数器-1（等价于wg.Add(-1)）
			// 确保会话资源释放
			defer hs.Client.Close()  // 关闭SSH客户端
			defer hs.Session.Close() // 关闭SSH会话
			// 控制并发数量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			result := mod.Run(hs, flags)

			// 写入结果到共享map（需要加锁保护）
			resultsMutex.Lock()       // 加锁：独占访问results
			results[hs.Addr] = result // 存储输出结果
			resultsMutex.Unlock()     // 解锁：允许其他goroutine访问
		}(hs) // 注意：这里必须传入当前循环的hs作为参数，否则goroutine可能拿到循环变量的最后一个值
	}

	wg.Wait() // 等待所有 goroutine 完成
	// 将字典键转换为切片并按照IP地址排序
	addrs := make([]string, 0, len(results))
	for addr := range results {
		addrs = append(addrs, addr)
	}

	// 按照IP地址排序（字符串排序）
	sort.Strings(addrs)

	if flags.Parameter["script_file"] == "/etc/fastdp/fastdp-check.sh" {
		PolishOutput(addrs, results)
		return
	}

	// 输出
	for _, addr := range addrs {
		result := results[addr]
		if result.Success {
			if result.Change {
				Changedf("host:%s 执行成功 output:\n%s", addr, result.Output)
			} else {
				Unchangedf("host:%s 执行成功 output:\n%s", addr, result.Output)
			}
		} else {
			Errorf("host:%s 执行失败\nSTDOUT:\n%sSTDERR:%s\n",
				addr, result.Output, result.Error)
		}
	}
}

func PolishOutput(addrs []string, results map[string]module.Result) {
	// 判断是否启用竖向模式
	isVertical := config.GlobalFlags.Parameter["vertical"] == "true"
	outputFormat := config.GlobalFlags.Parameter["format"]

	// 预定义所有要展示的字段（顺序固定）
	standardFields := []struct {
		key  string
		name string
	}{
		{"hostname", "主机名"},
		{"virt", "虚拟化"},
		{"os", "系统版本"},
		{"kernel", "内核"},
		{"cpu_cores", "CPU核心"},
		{"cpu_model", "CPU型号"},
		{"arch", "架构"},
		{"mem", "内存"},
		{"net", "网卡"},
		{"gateway", "网关"},
		{"disk", "磁盘"},
		{"firewall", "防火墙"},
		{"selinux", "SELinux"},
		{"swap", "Swap"},
		{"timezone", "时区"},
		{"sys_time", "系统时间"},
		{"hw_time", "硬件时间"},
		{"gpu", "GPU"},
	}
	// 标准字段 key 集合（用于快速判断）
	standardKeyMap := make(map[string]bool)
	for _, f := range standardFields {
		standardKeyMap[f.key] = true
	}
	hostDataCache := make(map[string]map[string]string) // IP => 解析后的KV
	customFieldSet := make(map[string]bool)             // 所有自定义字段

	for _, ip := range addrs {
		res := results[ip]
		if !res.Success {
			continue
		}
		data := parseCheckOutput(res.Output)
		hostDataCache[ip] = data // 缓存

		// 收集自定义字段
		for k := range data {
			if !standardKeyMap[k] {
				customFieldSet[k] = true
			}
		}
	}

	// 排序自定义字段（保证表格列对齐，必须要）
	var sortedCustomFields []string
	for k := range customFieldSet {
		sortedCustomFields = append(sortedCustomFields, k)
	}
	sort.Strings(sortedCustomFields)

	// --------------------------
	// 竖向模式（-g 参数）
	// --------------------------
	if isVertical {
		for _, ip := range addrs {
			res := results[ip]
			if !res.Success {
				fmt.Printf("\n===== %s =====\n执行失败\n", ip)
				continue
			}

			data := hostDataCache[ip]
			fmt.Printf("\n\033[1;34m===== %s =====\033[0m\n", ip)

			// 1. 输出标准字段
			for _, f := range standardFields {
				val := data[f.key]
				if val == "" {
					val = "-"
				}
				fmt.Printf("%-10s: %s\n", f.name, val)
			}
			//  2. 收集并输出【自定义字段】
			var customFields []string
			for k := range data {
				if !standardKeyMap[k] {
					customFields = append(customFields, k)
				}
			}
			sort.Strings(customFields)

			if len(customFields) > 0 {
				for _, k := range customFields {
					val := data[k]
					if val == "" {
						val = "-"
					}
					fmt.Printf("%-10s: %s\n", k, val)
				}
			}
		}
		return
	}

	// --------------------------
	// 默认横向表格（美观）
	// --------------------------
	t := table.NewWriter()

	// 表头
	header := table.Row{"主机IP"}
	for _, f := range standardFields {
		header = append(header, f.name)
	}
	for _, k := range sortedCustomFields {
		header = append(header, k)
	}
	t.AppendHeader(header)

	// 表格内容
	for _, ip := range addrs {
		res := results[ip]
		if !res.Success {
			row := table.Row{ip}
			for i := 0; i < len(standardFields)+len(sortedCustomFields); i++ {
				row = append(row, "执行失败")
			}
			t.AppendRow(row)
			continue
		}

		data := hostDataCache[ip]
		row := table.Row{ip}
		// 标准字段
		for _, f := range standardFields {
			val := data[f.key]
			if val == "" {
				val = "-"
			}
			row = append(row, val)
		}

		// 自定义字段
		for _, k := range sortedCustomFields {
			val := data[k]
			if val == "" {
				val = "-"
			}
			row = append(row, val)
		}
		t.AppendRow(row)
	}
	switch outputFormat {
	case "csv":
		fmt.Println(t.RenderCSV())
	case "md":
		fmt.Println(t.RenderMarkdown())
	case "html":
		htmlContent := renderHTML(t)
		fmt.Println(htmlContent)
	default:
		// 默认终端表格（圆角样式）
		t.SetOutputMirror(os.Stdout)
		t.SetOutputMirror(os.Stdout)
		t.SetStyle(table.StyleRounded)
		t.Style().Format.Header = text.FormatDefault
		t.Render()
	}
}
func renderHTML(t table.Writer) string {
	htmlTable := t.RenderHTML()
	// 加上完整的 HTML 文档头，指定 UTF-8 编码
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>fastdp 巡检报告</title>
    <style>
        table { border-collapse: collapse; width: 100%%; }
        th, td { border: 1px solid #ccc; padding: 6px 10px; text-align: left; }
        th { background-color: #f2f2f2; }
    </style>
</head>
<body>
%s
</body>
</html>`, htmlTable)
}

// 解析 key=value 格式
func parseCheckOutput(output string) map[string]string {
	data := make(map[string]string)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		data[k] = v
	}
	return data
}
