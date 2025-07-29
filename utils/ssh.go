package utils

import (
	. "fastdp/pkg/cobra"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 使用结构体保存每个主机的客户端和会话
type HostSession struct {
	Client  *ssh.Client  // SSH 客户端
	Session *ssh.Session // 一次 SSH 会话
	Addr    string       // 主机地址（如 "192.168.1.1:22"）
}

func SshConnect(groups []*HostGroup, hosts []*Host) []HostSession {
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
	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		hostSessions []HostSession
	)

	// 创建有缓冲的channel来控制最大并发数
	maxConcurrency := GlobalFlags.Concurrency
	semaphore := make(chan struct{}, maxConcurrency)

	for _, host := range allHosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()

			// 控制并发数量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 认证方式选择
			var authMethods []ssh.AuthMethod
			pwd := h.Params["password"]
			if pwd != "" {
				authMethods = []ssh.AuthMethod{ssh.Password(pwd)}
			} else {
				keyAuth, err := publicKeyAuth()
				if err != nil {
					mu.Lock()
					Errorf("主机 %s 公钥认证初始化失败: %v", h, err)
					mu.Unlock()
					return
				}
				authMethods = []ssh.AuthMethod{keyAuth}
			}

			port := h.Params["port"]
			if port == "" {
				port = "22"
			}

			config := &ssh.ClientConfig{
				User:            h.Params["user"],
				Auth:            authMethods,
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				Timeout:         5 * time.Second, // 添加连接超时
			}

			client, err := ssh.Dial("tcp", h.Address+":"+port, config)
			if err != nil {
				mu.Lock()
				Errorf("连接 %s 失败: %v", h.Address, err)
				mu.Unlock()
				return
			}

			session, err := client.NewSession()
			if err != nil {
				mu.Lock()
				Errorf("为 %s 创建会话失败: %v", h.Address, err)
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
	return hostSessions
}

// publicKeyAuth 生成基于默认私钥文件的认证方法
func publicKeyAuth() (ssh.AuthMethod, error) {
	// 获取用户主目录下的私钥文件路径（默认 ~/.ssh/id_rsa）
	keyPath := filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")

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

	return ssh.PublicKeys(signer), nil
}
