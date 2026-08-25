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
}

type ConnError struct {
	Kind string // "auth" | "connect" | "session"
	Msg  string
}

func SshConnect(allHosts []*Host) ([]HostSession, map[string]ConnError) {
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

			Debugf("主机:%s,用户名:%s,ssh端口:%s,密码:%s", host.Address, host.Params["user"], host.Params["port"], host.Params["password"])

			// 认证方式选择
			var authMethods []ssh.AuthMethod

			if password != "" {
				authMethods = []ssh.AuthMethod{ssh.Password(password)}
			} else {
				keyAuth, err := publicKeyAuth()
				if err != nil {
					mu.Lock()
					failedHosts[h.Address] = ConnError{Kind: "auth", Msg: err.Error()}
					mu.Unlock()
					return
				}
				authMethods = []ssh.AuthMethod{keyAuth}
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

			session, err := client.NewSession()
			if err != nil {
				mu.Lock()
				failedHosts[h.Address] = ConnError{Kind: "session", Msg: err.Error()}
				mu.Unlock()
				client.Close()
				return
			}

			// 将结果添加到切片（需要加锁）
			mu.Lock()
			hostSessions = append(hostSessions, HostSession{
				Client:  client,
				Session: session,
				Addr:    h.Address,
			})
			mu.Unlock()
		}(host) // 将当前host作为参数传入
	}

	wg.Wait() // 等待所有goroutine完成
	return hostSessions, failedHosts
}

// publicKeyAuth 生成基于默认私钥文件的认证方法
func publicKeyAuth() (ssh.AuthMethod, error) {
	// 定义常见私钥路径（按优先级排序）
	privateKeys := []string{
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"id_dsa",
	}
	// 先获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户主目录失败: %v", err)
	}
	// 遍历尝试
	for _, keyName := range privateKeys {
		// 获取用户主目录下的私钥文件路径（默认 ~/.ssh/id_rsa）
		keyPath := filepath.Join(homeDir, ".ssh", keyName)
		// 检查文件是否存在
		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			continue // 不存在就跳过
		}
		// 读取私钥文件
		key, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("读取私钥文件失败: %v", err)
		}

		// 解析私钥
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %v", err)
		}

		// 成功解析 → 直接返回
		return ssh.PublicKeys(signer), nil

	}
	// 所有私钥都失败
	return nil, fmt.Errorf("未找到任何可用的SSH私钥")

}
