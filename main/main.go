package main

import (
	"bytes"
	"fastdp/utils"
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"os"
	"sync"
)

func main() {
	groups, err := utils.HostParse("host")
	if err != nil {
		panic(err)
	}

	// 使用结构体保存每个主机的客户端和会话
	type HostSession struct {
		Client  *ssh.Client
		Session *ssh.Session
		Addr    string
	}

	var hostSessions []HostSession
	for _, group := range groups {
		for _, host := range group.Hosts {
			config := &ssh.ClientConfig{
				User: host.Params["user"],
				Auth: []ssh.AuthMethod{
					ssh.Password(host.Params["password"]),
				},
				HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 注意：生产环境不要使用此选项
			}

			client, err := ssh.Dial("tcp", host.Address+":"+host.Params["port"], config)
			if err != nil {
				log.Printf("连接 %s 失败: %v", host, err)
				continue // 跳过当前主机，继续处理其他主机
			}

			session, err := client.NewSession()
			if err != nil {
				log.Printf("为 %s 创建会话失败: %v", host.Address, err)
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
	// 建立连接

	// 使用并发执行命令
	var wg sync.WaitGroup
	var resultsMutex sync.Mutex
	results := make(map[string]string)

	for _, hs := range hostSessions {
		wg.Add(1)
		go func(hs HostSession) {
			defer wg.Done()
			defer hs.Client.Close()
			defer hs.Session.Close()

			var stdoutBuf bytes.Buffer
			hs.Session.Stdout = &stdoutBuf

			if err := hs.Session.Run(os.Args[1]); err != nil {
				log.Printf("在 %s 上执行命令失败: %v", hs.Addr, err)
				return
			}

			resultsMutex.Lock()
			results[hs.Addr] = stdoutBuf.String()
			resultsMutex.Unlock()
		}(hs)
	}

	wg.Wait() // 等待所有 goroutine 完成

	// 输出结果
	fmt.Println("命令输出:")
	for addr, output := range results {
		fmt.Printf("主机 %s 的输出:\n%s\n", addr, output)
	}

}
