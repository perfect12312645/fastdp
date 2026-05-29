package config

import (
	"github.com/spf13/viper"
	"os"
	"path/filepath"
)

type ConfigList struct {
	ConfigAbsPath      string
	HostInventory      string // 驼峰
	Concurrency        int
	LogLevel           string // 驼峰
	DefaultSSHPort     int    // 驼峰
	DefaultSSHUser     string // 驼峰
	DefaultSSHPassword string // 驼峰
	DefaultSSHTimeout int    // 驼峰
	DefaultFetchPath  string
}

var GlobalConfig *ConfigList

type Flags struct {
	Parameter     map[string]string
	HostInventory []string
	Debug         bool
	Concurrency   int
}

var GlobalFlags = &Flags{
	Parameter: make(map[string]string),
}

// 查找配置文件 【优先级：用户目录 > 系统目录 > 当前目录】
func FindConfigFile() string {
	// 1. 优先：用户家目录（最高优先级）
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(homeDir, ".fastdp", "config.toml")
		if _, err := os.Stat(userPath); err == nil {
			return userPath
		}
	}

	// 2. 其次：系统全局配置
	systemPath := "/etc/fastdp/config.toml"
	if _, err := os.Stat(systemPath); err == nil {
		return systemPath
	}

	// 3. 都没有：返回空（外部兜底当前目录）
	return ""
}
func ParseConfig(configFile string) (*ConfigList, error) {
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(absPath)
	fileName := filepath.Base(absPath)
	fileExt := ""
	if ext := filepath.Ext(fileName); len(ext) > 1 {
		fileExt = ext[1:]
	}

	viper.SetConfigName(fileName)
	viper.SetConfigType(fileExt)
	viper.AddConfigPath(dir)

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := &ConfigList{
		ConfigAbsPath:      absPath,
		HostInventory:      viper.GetString("host_inventory"),
		Concurrency:        viper.GetInt("concurrency"),
		LogLevel:           viper.GetString("log_level"),
		DefaultSSHPort:     viper.GetInt("default_ssh_port"),
		DefaultSSHUser:     viper.GetString("default_ssh_user"),
		DefaultSSHPassword: viper.GetString("default_ssh_password"),
		DefaultSSHTimeout:  viper.GetInt("default_ssh_timeout"),
		DefaultFetchPath:   viper.GetString("default_fetch_path"),
	}

	GlobalConfig = cfg
	return cfg, nil
}
