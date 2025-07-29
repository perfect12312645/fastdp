package config

import (
	"fastdp/pkg/cobra"
	"github.com/spf13/viper"
	"os"
	"path/filepath"
	"strings"
)

type ConfigList struct {
	ConfigAbsPath  string
	Host_inventory string
}

// 查找配置文件路径（按优先级）
func FindConfigFile() string {
	// 1. 系统级路径
	systemPath := "/etc/fastdp/config.toml"
	if _, err := os.Stat(systemPath); err == nil {
		return systemPath
	}

	// 2. 用户级路径（~/.fastdp/config.toml）
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(homeDir, ".fastdp", "config.toml")
		if _, err := os.Stat(userPath); err == nil {
			return userPath
		}
	}

	// 所有路径都不存在（可返回空或默认配置）
	return "" // 或返回默认配置路径，视需求而定
}
func ParseConfig(configFile string) ConfigList {
	// 确保配置路径有效
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		cobra.Errorf("获取配置文件绝对路径失败: %w", err)
		//os.Exit(6)
	}

	// 提取目录和文件名
	dir := filepath.Dir(absPath)

	fileName := filepath.Base(absPath)
	fileExt := strings.Split(filepath.Ext(fileName), ".")[1]

	viper.SetConfigName(fileName)
	viper.SetConfigType(fileExt)
	viper.AddConfigPath(dir)
	if err := viper.ReadInConfig(); err != nil {
		cobra.Errorf("读取配置失败: %v", err)
		os.Exit(5)
	}

	return ConfigList{
		ConfigAbsPath:  absPath,
		Host_inventory: viper.GetString("Host_inventory"),
	}

}
