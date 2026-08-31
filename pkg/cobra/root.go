package cobra

import (
	"encoding/json"
	"fastdp/module"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

var (
	Version = "v6.1.0"
)
var rootCmd = &cobra.Command{
	Use:   "fastdp",
	Short: "轻量级批量运维工具（Ansible 风格，单二进制无依赖）",
	Long: `fastdp 在指定主机组上批量执行运维操作。特点：单二进制无依赖、Go 协程并发、SSH 协议。
支持模块：shell（执行命令）、copy（文件传输）、fetch（远程拉取）、script（批量脚本）、ping（连通性）、check（环境巡检）。
主机组通过配置文件中的 host_inventory 指定，支持 [N:M:step] 区间展开。`,
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
		config.GlobalFlags.NoHistory, _ = cmd.Flags().GetBool("no-history")
		config.GlobalFlags.Timeout, _ = cmd.Flags().GetInt("timeout")
		config.GlobalFlags.RetryFile, _ = cmd.Flags().GetString("retry-file")
		config.GlobalFlags.Limit, _ = cmd.Flags().GetString("limit")
		config.GlobalFlags.Output, _ = cmd.Flags().GetString("output")
		config.GlobalFlags.DryRun, _ = cmd.Flags().GetBool("dry-run")
		inventoryPath, _ := cmd.Flags().GetString("inventory")
		if inventoryPath != "" {
			// 命令行传了，覆盖配置文件
			config.GlobalConfig.HostInventory = inventoryPath
		}

		Debugf("初始化完成 debug=%v concurrency=%d timeout=%v retry-file=%s limit=%s host文件=%s",
			config.GlobalFlags.Debug, config.GlobalFlags.Concurrency, config.GlobalFlags.Timeout,
			config.GlobalFlags.RetryFile, config.GlobalFlags.Limit, config.GlobalConfig.HostInventory)
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
		fmt.Println("警告：未找到全局配置，将使用当前目录 config.toml")
		configPath = "config.toml" // 兜底
	}

	// 解析配置
	_, err := config.ParseConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析配置失败：%v\n", err)
		os.Exit(exitcode.ParamError)
	}

	// 并发数兜底：配置文件为空/0 → 强制使用默认值 50
	concurrencyDefault := config.GlobalConfig.Concurrency
	if concurrencyDefault <= 0 {
		concurrencyDefault = 50
	}

	// 全局标志
	rootCmd.PersistentFlags().IntP("concurrency", "c", concurrencyDefault, "并发连接数（客户端同时连接服务端的数量）")
	rootCmd.PersistentFlags().BoolP("debug", "v", false, "开启调试模式（输出 SSH 连接、命令执行等详细日志到 stderr）")
	rootCmd.PersistentFlags().Bool("no-history", false, "本次执行不记录到历史日志")
	rootCmd.PersistentFlags().StringP("inventory", "i", "", "指定主机清单文件（优先于配置文件）")
	rootCmd.PersistentFlags().BoolP("version", "V", false, "显示版本信息")
	rootCmd.PersistentFlags().IntP("timeout", "t", 0, "单台执行超时秒数（默认0=不限制，超时主机标记失败不拖垮整批）")
	rootCmd.PersistentFlags().String("retry-file", "", "将失败主机写入文件，便于 --limit @file 重跑")
	rootCmd.PersistentFlags().String("limit", "", "从文件读取目标主机列表（@file，常用于对失败主机重跑）")
	rootCmd.PersistentFlags().StringP("output", "o", "text", "输出格式：text（人类阅读友好）/ JSON（结构化，适合脚本和 AI Agent）")
	rootCmd.PersistentFlags().Bool("dry-run", false, "干跑模式：只显示将要执行的命令和目标主机，不实际执行（安全预览）")

	// 添加子命令
	rootCmd.AddCommand(shellCmd, copyCmd, pingCmd, scriptCmd, checkCmd, fetchCmd)

}

// Execute 入口函数：外部 main 调用这个方法
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(exitcode.InternalError)
	}
}

func GetInfo() ([]*Host, error) {

	hostInventory := config.GlobalConfig.HostInventory
	// 规则：配置文件必须显式配置 host_inventory，否则直接失败
	if hostInventory == "" {
		return nil, fmt.Errorf("配置文件中未配置 host_inventory（主机清单路径），请先配置")
	}

	Debugf("生效的配置文件路径: %s", config.GlobalConfig.ConfigAbsPath)
	Debugf("生效的机组文件路径: %s", hostInventory)
	groups, err := ParseHostsFile(hostInventory)
	if err != nil {
		return nil, fmt.Errorf("解析host文件失败: %w", err)
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("主机组为空，请配置host文件：%s", hostInventory)
	}
	for _, group := range groups {
		Debugf("主机组:%s", group.Name)
	}

	// 获取将要执行的主机组
	inventory, hosts, err := Inventory(config.GlobalFlags.HostInventory, groups)
	if err != nil {
		return nil, err
	}
	execHosts := Filter(inventory, hosts)

	if config.GlobalFlags.Limit != "" {
		execHosts = applyLimit(execHosts, strings.TrimPrefix(config.GlobalFlags.Limit, "@"))
	}

	// 打印调试日志
	groupNames := make([]string, 0, len(inventory))
	for _, g := range inventory {
		if g != nil {
			groupNames = append(groupNames, g.Name)
		}
	}
	hostAddrs := make([]string, 0, len(execHosts))
	for _, h := range execHosts {
		hostAddrs = append(hostAddrs, h.Address)
	}
	Debugf("执行信息 主机组=%v 主机列表=%v 参数=%v", groupNames, hostAddrs, config.GlobalFlags.Parameter)
	return execHosts, nil
}

func applyLimit(hosts []*Host, limitPath string) []*Host {
	data, err := os.ReadFile(limitPath)
	if err != nil {
		Errorf("读取 limit 文件失败: %v", err)
		return hosts
	}
	whitelistSet := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			whitelistSet[line] = true
		}
	}
	if len(whitelistSet) == 0 {
		return hosts
	}
	var filtered []*Host
	for _, h := range hosts {
		if whitelistSet[h.Address] {
			filtered = append(filtered, h)
		}
	}
	return filtered
}

func execute(hostSessions []HostSession, failedHosts map[string]ConnError, flags *config.Flags, mod module.Module, modName string) int {
	start := time.Now()
	// 使用并发执行命令
	var wg sync.WaitGroup                     // 用于等待所有goroutine完成
	var resultsMutex sync.Mutex               // 用于保护results的并发写入
	results := make(map[string]module.Result) // 存储每个主机的执行结果（key:主机地址，value:命令输出）
	// 将连接失败的主机合并到 results
	for addr, ce := range failedHosts {
		results[addr] = module.Result{
			Success: false,
			Output:  "",
			Error:   ce.Msg,
			Change:  false,
		}
	}
	// 创建有缓冲的channel来控制最大并发数
	maxConcurrency := config.GlobalFlags.Concurrency
	semaphore := make(chan struct{}, maxConcurrency)
	timeout := time.Duration(config.GlobalFlags.Timeout) * time.Second
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
			result := runWithTimeout(mod, hs, flags, timeout)

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

	// 执行历史：每次执行写一条 JSON 元数据（不包含命令输出）
	if config.GlobalConfig.HistoryEnabled && !config.GlobalFlags.NoHistory {
		ok, failed, changed, unchanged := 0, 0, 0, 0
		for _, r := range results {
			if r.Success {
				ok++
				if r.Change {
					changed++
				} else {
					unchanged++
				}
			} else {
				failed++
			}
		}
		hosts := make([]string, 0, len(hostSessions))
		for _, hs := range hostSessions {
			hosts = append(hosts, hs.Addr)
		}
		command := buildCommandDesc(modName, flags.Parameter)
		WriteHistory(config.GlobalConfig.HistoryLog,
			BuildHistoryEntry(command, hosts, ok, failed, changed, unchanged, time.Since(start)))
	}

	// check 模块触发表格美化
	if modName == "check" {
		PolishOutput(addrs, results)
		writeRetryFile(flags, results, addrs)
		return computeExitCode(failedHosts, results)
	}

	// 汇总模式：只显示失败主机详情，成功主机折叠为一行
	isSummary := flags.Parameter["summary"] == "true"

	// 	JSON 输出模式
	if flags.Output == "json" {
		outputJSON(addrs, results)
		writeRetryFile(flags, results, addrs)
		return exitcode.Success
	}

	if isSummary {
		// 汇总模式输出
		okCount := 0
		for _, addr := range addrs {
			result := results[addr]
			if !result.Success {
				if config.GlobalFlags.Quiet {
					fmt.Print(result.Error)
				} else {
					Errorf("host:%s 执行失败\nSTDOUT:%s\nSTDERR:%s\n",
						addr, result.Output, result.Error)
				}
			} else {
				okCount++
			}
		}
		if okCount > 0 && !config.GlobalFlags.Quiet {
			fmt.Printf("\033[32m✅ %d/%d 成功\033[0m\n", okCount, len(addrs))
		}
		writeRetryFile(flags, results, addrs)
		return computeExitCode(failedHosts, results)
	}

	if flags.Parameter["aggregate"] == "" || !config.GlobalFlags.Quiet {
		outputResults(addrs, results, flags.DryRun)
	}
	writeRetryFile(flags, results, addrs)
	computeAggregate(flags, addrs, results)
	return computeExitCode(failedHosts, results)
}

func computeExitCode(failedHosts map[string]ConnError, results map[string]module.Result) int {
	if len(results) == 0 {
		return exitcode.Success
	}

	allSuccess := true
	hasAuthFail := false
	hasConnectFail := false
	hasSessionFail := false
	hasTimeout := false

	for _, r := range results {
		if r.Success {
			continue
		}
		allSuccess = false
		if r.TimedOut {
			hasTimeout = true
		}
	}

	for _, ce := range failedHosts {
		switch ce.Kind {
		case "auth":
			hasAuthFail = true
		case "connect":
			hasConnectFail = true
		case "session":
			hasSessionFail = true
		}
	}

	// 优先级：认证 > 连接 > 超时 > 部分失败
	switch {
	case allSuccess:
		return exitcode.Success
	case hasAuthFail:
		return exitcode.AuthFail
	case hasConnectFail, hasSessionFail:
		return exitcode.ConnectFail
	case hasTimeout:
		return exitcode.Timeout
	default:
		return exitcode.PartialFail
	}
}

func computeAggregate(flags *config.Flags, addrs []string, results map[string]module.Result) {
	aggType := flags.Parameter["aggregate"]
	if aggType == "" {
		return
	}
	if aggType != "avg" && aggType != "max" && aggType != "min" && aggType != "sum" &&
		aggType != "median" && aggType != "p95" && aggType != "p99" && aggType != "stddev" {
		Errorf("未知的聚合函数: %s（支持 avg/max/min/sum/median/p95/p99/stddev）", aggType)
		return
	}

	var values []float64
	errorCount := 0

	for _, addr := range addrs {
		r := results[addr]
		if !r.Success {
			continue
		}
		v, ok := ParseNumber(r.Output)
		if !ok {
			errorCount++
			if !config.GlobalFlags.Quiet {
				Errorf("⚠️  %s: 输出 %q 非数字，已跳过", addr, strings.TrimSpace(r.Output))
			}
			continue
		}
		values = append(values, v)
	}

	value, ok := ComputeAggregate(values, aggType)
	if !ok {
		fmt.Fprintln(os.Stderr, "聚合: 无有效数据")
		return
	}

	if config.GlobalFlags.Quiet {
		fmt.Printf("%.2f\n", value)
	} else {
		Changedf("─── 聚合 ───")
		Changedf("%s: %.2f (%d 台主机参与, %d 台跳过)", aggType, value, len(values), errorCount)
	}
}

func runWithTimeout(mod module.Module, hs HostSession, flags *config.Flags, timeout time.Duration) module.Result {
	if timeout <= 0 {
		return mod.Run(hs, flags)
	}
	ch := make(chan module.Result, 1)
	go func() {
		ch <- mod.Run(hs, flags)
	}()
	select {
	case r := <-ch:
		return r
	case <-time.After(timeout):
		// 关闭会话发送 SSH channel close，远程命令会收到 SIGHUP
		hs.Session.Close()
		hs.Client.Close()
		return module.Result{
			Success:  false,
			Output:   "",
			Error:    fmt.Sprintf("执行超时（%v）", timeout),
			Change:   false,
			TimedOut: true,
		}
	}
}

func outputResults(addrs []string, results map[string]module.Result, dryRun bool) {
	if config.GlobalFlags.Quiet {
		for _, addr := range addrs {
			fmt.Print(results[addr].Output)
		}
		return
	}

	var successAddrs, failedAddrs []string
	for _, addr := range addrs {
		if results[addr].Success {
			successAddrs = append(successAddrs, addr)
		} else {
			failedAddrs = append(failedAddrs, addr)
		}
	}

	for _, addr := range successAddrs {
		result := results[addr]
 		if result.Change {
			Changedf("host:%s 执行成功 output:\n%s", addr, result.Output)
		} else if dryRun {
			Unchangedf("host:%s [dry-run] 预览:\n%s", addr, result.Output)
		} else {
			Unchangedf("host:%s 执行成功 output:\n%s", addr, result.Output)
		}
	}

	if len(failedAddrs) > 0 {
		Errorf("─── 失败主机（%d/%d） ───", len(failedAddrs), len(addrs))
		for _, addr := range failedAddrs {
			result := results[addr]
			Errorf("host:%s 执行失败\nSTDOUT:%s\nSTDERR:%s\n",
				addr, result.Output, result.Error)
		}
	}

	fmt.Printf("\033[32m✅ %d/%d 成功\033[0m", len(successAddrs), len(addrs))
	if len(failedAddrs) > 0 {
		fmt.Printf("  \033[31m❌ %d/%d 失败\033[0m", len(failedAddrs), len(addrs))
	}
	fmt.Println()
}

type jsonResult struct {
	Host    string `json:"host"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error"`
	Change  bool   `json:"change"`
}

func outputJSON(addrs []string, results map[string]module.Result) {
	out := make([]jsonResult, 0, len(addrs))
	for _, addr := range addrs {
		r := results[addr]
		out = append(out, jsonResult{
			Host:    addr,
			Success: r.Success,
			Output:  r.Output,
			Error:   r.Error,
			Change:  r.Change,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}

func writeRetryFile(flags *config.Flags, results map[string]module.Result, addrs []string) {
	if flags.RetryFile == "" {
		return
	}
	var failedAddrs []string
	for _, addr := range addrs {
		if !results[addr].Success {
			failedAddrs = append(failedAddrs, addr)
		}
	}
	if len(failedAddrs) == 0 {
		return
	}
	content := strings.Join(failedAddrs, "\n") + "\n"
	if err := os.WriteFile(flags.RetryFile, []byte(content), 0644); err != nil {
		Errorf("写入失败主机列表失败: %v", err)
		return
	}
	Debugf("失败主机列表已写入: %s (%d 台)", flags.RetryFile, len(failedAddrs))
}

// buildCommandDesc 拼接模块名与用户参数（跳过 _ 开头的内部键），如 "shell args=uptime"
func buildCommandDesc(modName string, params map[string]string) string {
	parts := make([]string, 0, len(params))
	for k, v := range params {
		if strings.HasPrefix(k, "_") {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return modName
	}
	return modName + " " + strings.Join(parts, " ")
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

		// 收集自定义字段（不在预定义字段中的）
		for k := range data {
			if !standardKeyMap[k] {
				customFieldSet[k] = true
			}
		}
	}

	// 根据实际返回的数据过滤 standardFields（只显示有数据的字段）
	if len(hostDataCache) > 0 {
		filtered := make([]struct{ key, name string }, 0)
		for _, f := range standardFields {
			// 检查是否有任何主机返回了该字段
			for _, data := range hostDataCache {
				if _, ok := data[f.key]; ok {
					filtered = append(filtered, f)
					break
				}
			}
		}
		standardFields = filtered
		// 同步更新 keyMap
		standardKeyMap = make(map[string]bool)
		for _, f := range standardFields {
			standardKeyMap[f.key] = true
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
	case "json":
		// JSON 输出：包含所有主机（成功主机含字段数据，失败主机含 _error）
		jsonData := make(map[string]map[string]string)
		for _, ip := range addrs {
			res := results[ip]
			if res.Success {
				jsonData[ip] = hostDataCache[ip]
			} else {
				jsonData[ip] = map[string]string{"_error": res.Error}
			}
		}
		jsonBytes, err := json.MarshalIndent(jsonData, "", "  ")
		if err != nil {
			Errorf("JSON 序列化失败: %v", err)
			return
		}
		fmt.Println(string(jsonBytes))
	default:
		// 默认终端表格（圆角样式）
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
