package config

import (
	. "fastdp/pkg/flags"
	"fmt"
	"github.com/spf13/viper"
	"path/filepath"
	"strings"
)

type Config struct {
	Host_inventory string
}

func ParseConfig(configFile string) Config {
	// 确保配置路径有效
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		panic(fmt.Errorf("获取配置文件绝对路径失败: %w", err))
	}

	// 提取目录和文件名
	dir := filepath.Dir(absPath)
	Logger.Sugar().Debugf("配置文件所在目录: %s", dir)
	fileName := filepath.Base(absPath)
	Logger.Sugar().Debugf("配置文件名: %s", fileName)
	fileExt := strings.Split(filepath.Ext(fileName), ".")[1]
	Logger.Sugar().Debugf("配置文件格式: %s", fileExt)
	viper.SetConfigName(fileName)
	viper.SetConfigType(fileExt)
	viper.AddConfigPath(dir)
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("读取配置失败: %v", err))
	}

	return Config{
		Host_inventory: viper.GetString("Host_inventory"),
	}

}
