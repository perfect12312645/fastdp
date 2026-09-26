package cobra

import (
	"encoding/json"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

var copyIdCmd = &cobra.Command{
	Use:           "copy-id",
	Short:         "批量推送 SSH 公钥到远程主机（替代 ssh-copy-id）",
	SilenceErrors: true,
	SilenceUsage:  true,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			Errorf("请指定目标主机组或主机\n示例:\n  fastdp copy-id all\n  fastdp copy-id -p ~/.ssh/id_ed25519.pub web")
			os.Exit(exitcode.ParamError)
		}
		config.GlobalFlags.HostInventory = args

		// 读取公钥
		pubPath, _ := cmd.Flags().GetString("pub-key")
		pubKey, pubResolvedPath, err := loadPublicKey(pubPath)
		if err != nil {
			Errorf("%v", err)
			os.Exit(exitcode.ParamError)
		}
		Debugf("使用公钥: %s", strings.Fields(pubKey)[1])

		// 密码处理：--password > --ask-pass > host 文件 > 默认配置
		password, _ := cmd.Flags().GetString("password")
		askPass, _ := cmd.Flags().GetBool("ask-pass")
		if password == "" && askPass {
			password, err = promptPassword()
			if err != nil {
				Errorf("读取密码失败: %v", err)
				os.Exit(exitcode.ParamError)
			}
		}

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}

		// 干跑模式：只显示预览，不实际推送
		if config.GlobalFlags.DryRun {
			fmt.Printf("将推送公钥到 %d 台主机:\n", len(execHosts))
			for _, h := range execHosts {
				fmt.Printf("  %s\n", h.Address)
			}
			fmt.Printf("公钥(%s): %s %s\n", pubResolvedPath, strings.Fields(pubKey)[0], strings.Fields(pubKey)[1])
			os.Exit(exitcode.Success)
		}

		// 并发推送
		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make(map[string]string)
		timeout := time.Duration(config.GlobalFlags.Timeout) * time.Second

		// --interactive 模式：逐台输入密码，必须串行
		interactive, _ := cmd.Flags().GetBool("interactive")
		concurrency := config.GlobalFlags.Concurrency
		if interactive {
			concurrency = 1
		}
		semaphore := make(chan struct{}, concurrency)

		for _, h := range execHosts {
			wg.Add(1)
			go func(h *Host) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				status := pushPublicKeyInteractive(h, password, pubKey, timeout, interactive)
				mu.Lock()
				results[h.Address] = status
				mu.Unlock()
			}(h)
		}

		wg.Wait()

		// 输出结果
		ok, failed := 0, 0

		if config.GlobalFlags.Output == "json" {
			type copyIdResult struct {
				Host   string `json:"host"`
				Status string `json:"status"`
				OK     bool   `json:"ok"`
			}
			jsonResults := make([]copyIdResult, 0, len(execHosts))
			for _, addr := range sortedKeys(results) {
				s := results[addr]
				isOK := s == "已添加" || s == "已存在，跳过"
				if isOK {
					ok++
				} else {
					failed++
				}
				jsonResults = append(jsonResults, copyIdResult{Host: addr, Status: s, OK: isOK})
			}
			jsonData, _ := json.MarshalIndent(jsonResults, "", "  ")
			fmt.Println(string(jsonData))
		} else {
			fmt.Println()
			// 成功主机在前（IP 排序），失败主机聚合到末尾
			var successAddrs, failedAddrs []string
			for _, addr := range sortedKeys(results) {
				s := results[addr]
				if s == "已跳过" {
					failedAddrs = append(failedAddrs, addr)
				} else if strings.HasPrefix(s, "已") {
					successAddrs = append(successAddrs, addr)
				} else {
					failedAddrs = append(failedAddrs, addr)
				}
			}
			for _, addr := range successAddrs {
				s := results[addr]
				if s == "已添加" {
					ok++
					Changedf("%s: %s", addr, s)
				} else {
					ok++
					Unchangedf("%s: %s", addr, s)
				}
			}
			if len(failedAddrs) > 0 {
				Errorf("─── 失败主机（%d/%d） ───", len(failedAddrs), len(results))
				for _, addr := range failedAddrs {
					if results[addr] == "已跳过" {
						Unchangedf("%s: %s", addr, results[addr])
					} else {
						failed++
						Errorf("%s: %s", addr, results[addr])
					}
				}
			}
			fmt.Println()
			fmt.Println(SummaryLine(ok, failed, len(results)))
		}

		// 写入失败主机到 retry 文件，便于 --limit @file 重跑
		if config.GlobalFlags.RetryFile != "" && failed > 0 {
			var failedAddrs []string
			for _, addr := range sortedKeys(results) {
				if !strings.HasPrefix(results[addr], "已") {
					failedAddrs = append(failedAddrs, addr)
				}
			}
			content := strings.Join(failedAddrs, "\n") + "\n"
			if werr := os.WriteFile(config.GlobalFlags.RetryFile, []byte(content), 0644); werr != nil {
				Errorf("写入失败主机列表失败: %v", werr)
			} else {
				Debugf("失败主机列表已写入: %s (%d 台)", config.GlobalFlags.RetryFile, len(failedAddrs))
			}
		}

		if failed > 0 {
			os.Exit(exitcode.PartialFail)
		}
	},
	Example: `
  # 自动发现公钥并批量推送（需要host文件配置好密码）
  fastdp copy-id all

  # 指定公钥文件
  fastdp copy-id -p ~/.ssh/id_ed25519.pub web

  # 交互式输入密码（所有机器同一密码）
  fastdp copy-id --ask-pass all

  # 统一密码（CI/CD）
  fastdp copy-id --password "xxx" all

  # 逐台输入密码（机器密码不同，串行执行，失败可重试）
  fastdp copy-id --interactive all

  # 干跑模式：只显示预览，不实际推送
  fastdp copy-id --dry-run all

  # JSON 输出 + 失败主机写入文件
  fastdp copy-id -o json --retry-file /tmp/failed.txt all
`,
}

// pushPublicKey Interactive 单台推送
// interactive=true 时：先尝试 host 文件已有密码，失败则逐台提示（最多3次，回车跳过）
// interactive=false 时：使用全局/host 密码单次尝试，带 -t 超时控制
func pushPublicKeyInteractive(h *Host, globalPwd, pubKey string, timeout time.Duration, interactive bool) string {
	user, port := resolveAuthParams(h)

	if !interactive {
		// 非交互：单次尝试 + 超时控制
		done := make(chan string, 1)
		go func() {
			status, _ := pushPublicKey(h, globalPwd, pubKey)
			done <- status
		}()
		if timeout <= 0 {
			return <-done
		}
		select {
		case status := <-done:
			return status
		case <-time.After(timeout):
			return "失败: 执行超时"
		}
	}

	// 交互模式：先尝试 host 文件/全局已有的密码，失败再提示输入
	hostPwd := h.Params["password"]
	if hostPwd == "" {
		hostPwd = config.GlobalConfig.DefaultSSHPassword
	}
	if globalPwd != "" {
		hostPwd = globalPwd // 命令行 --password 优先级最高
	}
	if hostPwd != "" {
		fmt.Printf("[%s@%s:%s] 使用已有密码尝试... ", user, h.Address, port)
		status, _ := pushPublicKey(h, hostPwd, pubKey)
		if status == "已添加" || status == "已存在，跳过" {
			fmt.Printf("%s\n", status)
			return status
		}
		fmt.Printf("失败，需要手动输入\n")
	}

	// 逐台提示密码，失败重试（最多 3 次），回车跳过
	for attempt := 1; attempt <= 3; attempt++ {
		fmt.Printf("[%s@%s:%s] 请输入密码 (回车跳过): ", user, h.Address, port)
		bytePwd, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "密码读取失败: " + err.Error()
		}
		pwd := string(bytePwd)
		if pwd == "" {
			return "已跳过"
		}
		status, isAuthErr := pushPublicKey(h, pwd, pubKey)
		if status == "已添加" || status == "已存在，跳过" {
			return status
		}
		if !isAuthErr {
			// 非认证错误（连接/超时等）：重试密码无意义，直接结束
			Errorf("[%s@%s:%s] %s", user, h.Address, port, status)
			return status
		}
		Errorf("[%s@%s:%s] 认证失败，请确认密码后重试", user, h.Address, port)
	}
	return "失败: 认证失败 (已尝试 3 次)"
}

// resolveAuthParams 解析主机认证参数（用户/端口），供交互提示使用
func resolveAuthParams(h *Host) (user, port string) {
	user = h.Params["user"]
	if user == "" {
		user = config.GlobalConfig.DefaultSSHUser
	}
	if user == "" {
		user = "root"
	}
	port = h.Params["port"]
	if port == "" {
		port = strconv.Itoa(config.GlobalConfig.DefaultSSHPort)
	}
	if port == "" {
		port = "22"
	}
	return user, port
}

// pushPublicKey 单台主机推送公钥
// 返回 (状态描述, 是否认证失败)。isAuthErr=true 表示密码错误（可重试）；
// 其他错误（连接/超时/执行）为 false，重试密码无意义。
func pushPublicKey(h *Host, globalPwd, pubKey string) (string, bool) {
	// 解析认证参数（优先 host 文件密码，其次全局密码）
	hostPwd := h.Params["password"]
	if hostPwd == "" {
		hostPwd = config.GlobalConfig.DefaultSSHPassword
	}
	authPwd := globalPwd
	if authPwd == "" {
		authPwd = hostPwd
	}

	user, port := resolveAuthParams(h)

	// 无密码则无法认证（copy-id 场景必须密码）
	if authPwd == "" {
		return "错误：未提供密码（host 文件无 password，也无指定 --password/--ask-pass）", true
	}

	// SSH 连接超时：-t 已由外层整体控制，这里用配置的连接超时
	connTimeout := time.Duration(config.GlobalConfig.DefaultSSHTimeout) * time.Second
	if connTimeout <= 0 {
		connTimeout = 10 * time.Second
	}
	client, err := ssh.Dial("tcp", h.Address+":"+port, &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(authPwd)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         connTimeout,
	})
	if err != nil {
		isAuthErr := strings.Contains(err.Error(), "unable to authenticate") ||
			strings.Contains(err.Error(), "Password auth failed")
		return "连接失败: " + err.Error(), isAuthErr
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "创建会话失败: " + err.Error(), false
	}
	defer session.Close()

	// 推送公钥（去重 + 权限设置），公钥内容直接内嵌避免注入
	// 注意：不能用 2>/dev/null 重定向（部分受限环境 /dev/null 不可写会导致 grep 恒失败）
	cmdStr := fmt.Sprintf(`umask 077 && mkdir -p ~/.ssh && chmod 700 ~/.ssh && grep -qF %q ~/.ssh/authorized_keys || { echo %q >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys; echo "ADDED"; }`, pubKey, pubKey)

	output, err := session.Output(cmdStr)
	if err != nil {
		return "执行失败: " + err.Error(), false
	}

	outStr := strings.TrimSpace(string(output))
	if strings.Contains(outStr, "ADDED") {
		return "已添加", false
	}
	return "已存在，跳过", false
}

// loadPublicKey 读取公钥文件，未指定时自动发现 ~/.ssh/*.pub
// 返回 (公钥内容, 实际加载路径, 错误)
func loadPublicKey(pubPath string) (string, string, error) {
	if pubPath != "" {
		abs, err := filepath.Abs(pubPath)
		if err != nil {
			return "", "", fmt.Errorf("获取公钥路径失败: %v", err)
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return "", "", fmt.Errorf("读取公钥失败 %s: %v", abs, err)
		}
		content, err := validatePublicKey(string(data))
		if err != nil {
			return "", "", fmt.Errorf("%s: %v", abs, err)
		}
		return content, abs, nil
	}

	// 自动发现 ~/.ssh/*.pub
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("获取用户主目录失败: %v", err)
	}
	pattern := filepath.Join(homeDir, ".ssh", "*.pub")
	matches, _ := filepath.Glob(pattern)
	// 优先 id_rsa.pub > id_ed25519.pub > id_ecdsa.pub，再按字母序兜底
	priority := []string{"id_rsa.pub", "id_ed25519.pub", "id_ecdsa.pub"}
	for _, name := range priority {
		for _, m := range matches {
			if filepath.Base(m) == name {
				data, err := os.ReadFile(m)
				if err == nil {
					content, verr := validatePublicKey(string(data))
					if verr == nil {
						return content, m, nil
					}
					// 格式无效则继续尝试下一个，不立即报错
				}
			}
		}
	}
	if len(matches) > 0 {
		data, err := os.ReadFile(matches[0])
		if err == nil {
			content, verr := validatePublicKey(string(data))
			if verr == nil {
				return content, matches[0], nil
			}
			return "", "", fmt.Errorf("%s: %v", matches[0], verr)
		}
	}
	return "", "", fmt.Errorf("未找到有效公钥，请使用 -p 指定公钥文件（~/.ssh/*.pub）")
}

// validatePublicKey 校验公钥内容格式，返回去除首尾空白的有效公钥
func validatePublicKey(content string) (string, error) {
	content = strings.TrimSpace(content)
	fields := strings.Fields(content)
	if len(fields) < 2 {
		return "", fmt.Errorf("不是有效公钥（格式应为: keytype base64 [comment]）")
	}
	keyType := fields[0]
	validKeyType := keyType == "ssh-rsa" || keyType == "ssh-ed25519" || keyType == "ssh-dss" ||
		strings.HasPrefix(keyType, "ecdsa-sha2-") || strings.HasPrefix(keyType, "sk-")
	if !validKeyType {
		return "", fmt.Errorf("包含未知公钥类型 %q（支持: ssh-rsa/ssh-ed25519/ssh-dss/ecdsa-sha2-*/sk-*）", keyType)
	}
	return content, nil
}

// promptPassword 交互式输入密码（不回显）
func promptPassword() (string, error) {
	fmt.Print("请输入 SSH 密码: ")
	fd := int(os.Stdin.Fd())
	bytePwd, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(bytePwd), nil
}

// sortedKeys 返回 map 的排序键列表
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	copyIdCmd.Flags().StringP("pub-key", "p", "", "本地公钥文件路径（默认自动发现 ~/.ssh/*.pub）")
	copyIdCmd.Flags().String("password", "", "SSH 密码（统一密码场景）")
	copyIdCmd.Flags().Bool("ask-pass", false, "交互式输入密码（所有机器同一密码）")
	copyIdCmd.Flags().Bool("interactive", false, "逐台输入密码（机器密码不同时使用，串行执行，失败可重试）")
}
