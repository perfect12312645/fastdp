package utils

import (
	. "fastdp/pkg/flags"
	"fmt"
	"golang.org/x/crypto/ssh"
	"os"
	"path/filepath"
)

// 使用结构体保存每个主机的客户端和会话
type HostSession struct {
	Client  *ssh.Client  // SSH 客户端
	Session *ssh.Session // 一次 SSH 会话
	Addr    string       // 主机地址（如 "192.168.1.1:22"）
}

func SshConnect(groups []*HostGroup) []HostSession {
	var hostSessions []HostSession
	for _, group := range groups {
		for _, host := range group.Hosts {
			// 动态选择认证方式
			var authMethods []ssh.AuthMethod
			pwd := host.Params["password"]
			if pwd != "" {
				// 有密码时使用密码认证
				authMethods = []ssh.AuthMethod{ssh.Password(pwd)}
			} else {
				// 无密码时使用公钥认证
				keyAuth, err := publicKeyAuth()
				if err != nil {
					Errorf("主机 %s 公钥认证初始化失败: %v", host, err)
					continue
				}
				authMethods = []ssh.AuthMethod{keyAuth}
			}
			config := &ssh.ClientConfig{
				User:            host.Params["user"],
				Auth:            authMethods,
				HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 注意：生产环境不要使用此选项
			}

			client, err := ssh.Dial("tcp", host.Address+":"+host.Params["port"], config)
			if err != nil {
				Errorf("连接 %s 失败: %v", host.Address, err)
				continue // 跳过当前主机，继续处理其他主机
			}

			session, err := client.NewSession()
			if err != nil {
				Errorf("为 %s 创建会话失败: %v", host.Address, err)
				client.Close() // 关闭客户端连接
				continue
			}

			hostSessions = append(hostSessions, HostSession{
				Client:  client,
				Session: session,
				Addr:    host.Address,
			})
		}
	}
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
