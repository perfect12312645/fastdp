package main

import (
	"fastdp/module"
	"fastdp/pkg/config"
	. "fastdp/pkg/flags"
	. "fastdp/utils"
	"go.uber.org/zap"
	"sync"
)

func execute(hostSessions []HostSession, flags *Flags, mod module.Module) {
	// 使用并发执行命令
	var wg sync.WaitGroup                     // 用于等待所有goroutine完成
	var resultsMutex sync.Mutex               // 用于保护results的并发写入
	results := make(map[string]module.Result) // 存储每个主机的执行结果（key:主机地址，value:命令输出）

	for _, hs := range hostSessions {
		wg.Add(1) // 每启动一个goroutine，WaitGroup计数器+1
		// 启动goroutine，传入当前的hs（主机会话）
		go func(hs HostSession) {
			defer wg.Done() // goroutine结束时，WaitGroup计数器-1（等价于wg.Add(-1)）
			// 确保会话资源释放
			defer hs.Client.Close()  // 关闭SSH客户端
			defer hs.Session.Close() // 关闭SSH会话
			result := mod.Run(hs, flags)

			// 写入结果到共享map（需要加锁保护）
			resultsMutex.Lock()       // 加锁：独占访问results
			results[hs.Addr] = result // 存储输出结果
			resultsMutex.Unlock()     // 解锁：允许其他goroutine访问
		}(hs) // 注意：这里必须传入当前循环的hs作为参数，否则goroutine可能拿到循环变量的最后一个值
	}

	wg.Wait() // 等待所有 goroutine 完成
	// 输出
	for addr, result := range results {
		if result.Success {
			if result.Change {
				Changedf("host:%s 执行成功 output:\n%s\n", addr, result.Output)
			} else {
				Unchangedf("host:%s 执行成功 output:\n%s\n", addr, result.Output)
			}
		} else {
			Errorf("host:%s 执行失败\nSTDOUT:\n%s\nSTDERR:\n%s\n",
				addr, result.Output, result.Error)
		}
	}
}
func main() {
	// 初始化命令行参数
	remainingArgs, flag := ParseFlags()
	// 配置文件解析
	config := config.ParseConfig("config.toml")
	hostInventory := config.Host_inventory
	// 主机清单解析
	groups, err := HostParse(hostInventory)
	if err != nil {
		panic(err)
	}
	if len(groups) == 0 {
		Errorf("主机组为空，请配置host文件：%s", hostInventory)
	}
	// 获取将要执行的主机组
	inventory := Inventory(remainingArgs, groups)
	Logger.Debug("message", zap.Any("将要执行的主机组", inventory))
	Logger.Debug("message", zap.String("将要执行的模块", *flag.Module))
	Logger.Debug("message", zap.String("将要执行的参数", *flag.Parameter))
	hostSessions := SshConnect(inventory)
	mod, err := module.GetModule(*flag.Module)
	if err := mod.SetParams(*flag.Parameter); err != nil {
		Logger.Fatal("解析模块参数失败", zap.Error(err))
	}
	execute(hostSessions, flag, mod)
}
