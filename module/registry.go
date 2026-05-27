package module

import (
	"fastdp/utils"
	"fmt"
)

// 模块注册表：程序启动时初始化，运行时只读
var registry = make(map[string]func() Module)

// Register 注册模块（仅在 init 中调用，无并发）
func Register(name string, factory func() Module) {
	registry[name] = factory
}

// GetModule 获取模块（仅在命令执行时调用，只读）
func GetModule(name string) (Module, error) {
	factory, ok := registry[name]
	if !ok {
		err := fmt.Errorf("未知模块: %s", name)
		utils.Errorf(err.Error())
		return nil, err
	}
	return factory(), nil
}
