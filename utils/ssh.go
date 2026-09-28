package utils

import (
	"fastdp/pkg/config"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// 使用结构体保存每个主机的客户端和会话
type HostSession struct {
	Client  *ssh.Client  // SSH 客户端
	Session *ssh.Session // 一次 SSH 会话
	Addr    string       // 主机地址（如 "192.168.1.1:22"）
	Params  map[string]string // 主机参数（user/port/password 及扩展参数，如 paging_disable）
}

type ConnError struct {
	Kind string // "auth" | "connect" | "session"
	Msg  string
}

func SshConnect(allHosts []*Host, moduleName string) ([]HostSession, map[string]ConnError) {
	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		hostSessions []HostSession
		failedHosts  = make(map[string]ConnError)
	)

	for _, host := range allHosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()
			// 1. 用户名
			user := h.Params["user"]
			if user == "" {
				user = config.GlobalConfig.DefaultSSHUser
			}
			if user == "" {
				user = "root" // 最终兜底
			}

			// 2. 密码
			password := h.Params["password"]
			if password == "" {
				password = config.GlobalConfig.DefaultSSHPassword
			}
			// 密码无兜底，为空就是空

			// 3. 端口
			port := h.Params["port"]
			if port == "" {
				port = strconv.Itoa(config.GlobalConfig.DefaultSSHPort)
			}
			if port == "" {
				port = "22" // 最终兜底
			}

			// 单行输出该主机全部连接信息（避免每台多行刷屏；分页禁用命令仅 switch 模式有意义）
			if moduleName == "shell" && config.GlobalConfig.Mode == "switch" {
				pd := config.GlobalFlags.Parameter["paging_disable"]
				if pd == "" {
					pd = host.Params["paging_disable"]
				}
				if pd == "" {
					pd = config.GlobalConfig.PagingDisable
				}
				if pd == "" {
					pd = "screen-length disable"
				}
				Debugf("主机:%s,用户名:%s,ssh端口:%s,密码:%s,分页禁用:%s", host.Address, host.Params["user"], host.Params["port"], host.Params["password"], pd)
			} else {
				Debugf("主机:%s,用户名:%s,ssh端口:%s,密码:%s", host.Address, host.Params["user"], host.Params["port"], host.Params["password"])
			}

			// 认证方式选择：免密优先（key），密码兜底，SSH 按顺序自动尝试
			var authMethods []ssh.AuthMethod
			if keyAuth, err := publicKeyAuth(config.GlobalFlags.SSHKeyPath, config.GlobalFlags.AllKeys); err == nil {
				authMethods = append(authMethods, keyAuth)
			}
			if password != "" {
				authMethods = append(authMethods, ssh.Password(password))
			}
			if len(authMethods) == 0 {
				mu.Lock()
				failedHosts[h.Address] = ConnError{Kind: "auth", Msg: "未找到任何可用的 SSH 认证方式（私钥和密码都不可用）"}
				mu.Unlock()
				return
			}
			sshConfig := &ssh.ClientConfig{
				User:            user,
				Auth:            authMethods,
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         time.Duration(config.GlobalConfig.DefaultSSHTimeout) * time.Second,
			}

			client, err := ssh.Dial("tcp", h.Address+":"+port, sshConfig)
			if err != nil {
				mu.Lock()
				failedHosts[h.Address] = ConnError{Kind: "connect", Msg: err.Error()}
				mu.Unlock()
				return
			}

			// fetch/copy 模块不需要预建 Session（避免 H3C MaxSessions=1 冲突）
			var session *ssh.Session
			if moduleName != "fetch" && moduleName != "copy" {
				session, err = client.NewSession()
				if err != nil {
					mu.Lock()
					failedHosts[h.Address] = ConnError{Kind: "session", Msg: err.Error()}
					mu.Unlock()
					client.Close()
					return
				}
			}

			// 将结果添加到切片（需要加锁）
			mu.Lock()
			hostSessions = append(hostSessions, HostSession{
				Client:  client,
				Session: session,
				Addr:    h.Address,
				Params:  h.Params,
			})
			mu.Unlock()
		}(host) // 将当前host作为参数传入
	}

	wg.Wait() // 等待所有goroutine完成
	return hostSessions, failedHosts
}

// publicKeyAuth 生成 SSH 私钥认证方法
// keyPath 非空时优先使用指定私钥；allKeys 为 true 时收集所有可用私钥由 SSH 依次尝试
func publicKeyAuth(keyPath string, allKeys bool) (ssh.AuthMethod, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户主目录失败: %v", err)
	}

	// 1. 指定私钥路径（优先级最高）
	if keyPath != "" {
		signer, err := loadSigner(keyPath)
		if err != nil {
			return nil, fmt.Errorf("加载指定私钥失败 %s: %v", keyPath, err)
		}
		return ssh.PublicKeys(signer), nil
	}

	// 2. 自动发现 ~/.ssh/ 下的常见私钥
	privateKeys := []string{
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa",
	}

	if !allKeys {
		// 默认模式：返回第一个可用的私钥（最快）
		for _, keyName := range privateKeys {
			keyPath := filepath.Join(homeDir, ".ssh", keyName)
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				continue
			}
			signer, err := loadSigner(keyPath)
			if err != nil {
				return nil, fmt.Errorf("解析私钥失败 %s: %v", keyPath, err)
			}
			return ssh.PublicKeys(signer), nil
		}
	} else {
		// --all-keys 模式：收集所有可用私钥，SSH 客户端依次尝试
		var signers []ssh.Signer
		for _, keyName := range privateKeys {
			keyPath := filepath.Join(homeDir, ".ssh", keyName)
			if _, err := os.Stat(keyPath); os.IsNotExist(err) {
				continue
			}
			signer, err := loadSigner(keyPath)
			if err != nil {
				continue // 单个私钥解析失败不影响其他
			}
			signers = append(signers, signer)
		}
		if len(signers) > 0 {
			return ssh.PublicKeys(signers...), nil
		}
	}

	return nil, fmt.Errorf("未找到任何可用的SSH私钥")
}

// loadSigner 读取并解析单个私钥文件
func loadSigner(keyPath string) (ssh.Signer, error) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(key)
}
