package main

import (
	"fastdp/module"
	"fastdp/pkg/config"
	"os"
	"sort"
	"time"

	. "fastdp/pkg/cobra"
	. "fastdp/utils"
	"go.uber.org/zap"

	"sync"
)

func execute(hostSessions []HostSession, flags *Flags, mod module.Module) {
	// 使用并发执行命令
	var wg sync.WaitGroup                     // 用于等待所有goroutine完成
	var resultsMutex sync.Mutex               // 用于保护results的并发写入
	results := make(map[string]module.Result) // 存储每个主机的执行结果（key:主机地址，value:命令输出）
	// 创建有缓冲的channel来控制最大并发数
	maxConcurrency := GlobalFlags.Concurrency
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
func main() {
	// 查找配置文件
	configPath := config.FindConfigFile()
	if configPath == "" {
		// 处理用户目录获取失败等错误（如无权限）
		Logger.Sugar().Debugf("警告：获取配置路径/etc/fastdp/config.toml失败,将使用默认配置config.toml")
		configPath = "config.toml" // 兜底
	}
	Logger.Sugar().Debugf("生效的配置文件路径：%s", configPath)
	configList := config.ParseConfig(configPath)
	hostInventory := configList.Host_inventory
	Logger.Sugar().Debugf("生效的主机组文件路径: %s", configList.ConfigAbsPath)
	// 主机清单解析
	groups, err := HostParse(hostInventory)
	if err != nil {
		Errorf("解析host文件失败", err)
	}
	if len(groups) == 0 {
		Errorf("主机组为空，请配置host文件：%s", hostInventory)
	}
	// 获取将要执行的主机组
	inventory, hosts := Inventory(GlobalFlags.HostInventory, groups)
	execHosts := Filter(inventory, hosts)
	Logger.Debug("message", zap.Any("将要执行的主机组", inventory))
	Logger.Debug("message", zap.Any("将要执行的所有主机", execHosts))
	Logger.Debug("message", zap.String("将要执行的模块", GlobalFlags.Module))
	Logger.Debug("message", zap.Any("将要执行的参数", GlobalFlags.Parameter))
	hostSessions := SshConnect(execHosts)
	mod, err := module.GetModule(GlobalFlags.Module)
	if err != nil {
		Errorf("获取模块失败: %v", err)
		os.Exit(1)
	}
	if GlobalFlags.Module == "copy" {
		// 转换为Debug日志，使用%v格式化时间对象
		Logger.Sugar().Debugf("copy模块，开始计算源文件md5: %v", time.Now())
		GlobalFlags, err = module.GetSource(GlobalFlags)
		if err != nil {
			Logger.Sugar().Errorf("copy模块，获取源文件信息失败: %v", err)
			os.Exit(1)
		}
		Logger.Sugar().Debugf("copy模块，计算源文件md5结束: %v", time.Now())
	}
	execute(hostSessions, GlobalFlags, mod)
}
