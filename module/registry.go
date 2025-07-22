package module

import (
	"fastdp/pkg/cobra"
	"fmt"
	"sync"
)

// 模块注册表：key 是模块名（如 "shell"），value 是模块构造函数
var (
	registry = make(map[string]func() Module)
	mu       sync.RWMutex // 保护 registry 的并发安全
)

// Register 注册模块（在 init 函数中调用，每个模块自己注册）
func Register(name string, factory func() Module) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = factory
}

// GetModule 通过模块名获取实例（主程序调用）
func GetModule(name string) (Module, error) {
	mu.RLock()
	defer mu.RUnlock()
	factory, ok := registry[name]
	if !ok {
		err := fmt.Errorf("未知模块: %s", name)
		cobra.Errorf(err.Error()) // 输出错误
		return nil, err           // 返回错误，不再执行 factory()
	}
	return factory(), nil // 调用构造函数创建实例
}
