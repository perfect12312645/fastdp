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
	DefaultSSHPort     int    // 驼峰
	DefaultSSHUser     string // 驼峰
	DefaultSSHPassword string // 驼峰
	DefaultSSHTimeout  int    // 驼峰
	DefaultFetchPath   string
	HistoryEnabled     bool   // 执行历史日志开关
	HistoryLog         string // 执行历史日志路径
}

var GlobalConfig *ConfigList

type Flags struct {
	Parameter     map[string]string
	HostInventory []string
	Debug         bool
	Concurrency   int
	NoHistory     bool   // 本次执行不记录执行历史
	Timeout       int    // 单台执行超时（秒，0=不限制）
	RetryFile     string // 失败主机列表输出文件路径（空=不输出）
	Limit         string // 目标主机限制: @file (如 --limit @/tmp/failed.txt)
	Output        string // 输出格式: text(默认) / json
	Quiet         bool   // 静默模式：只输出命令 stdout，无装饰文本
	DryRun        bool   // 干跑模式：只显示将要执行的命令和目标，不实际执行
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
		DefaultSSHPort:     viper.GetInt("default_ssh_port"),
		DefaultSSHUser:     viper.GetString("default_ssh_user"),
		DefaultSSHPassword: viper.GetString("default_ssh_password"),
		DefaultSSHTimeout:  viper.GetInt("default_ssh_timeout"),
		DefaultFetchPath:   viper.GetString("default_fetch_path"),
	}

	// 执行历史日志开关：未配置时默认开启（viper.GetBool 对未设置项返回 false，需用 IsSet 判断）
	if viper.IsSet("history_enabled") {
		cfg.HistoryEnabled = viper.GetBool("history_enabled")
	} else {
		cfg.HistoryEnabled = true // 默认开启
	}

	// 执行历史日志路径：为空则自动跟随生效配置文件目录（如 /etc/fastdp/history.log）
	cfg.HistoryLog = viper.GetString("history_log")
	if cfg.HistoryLog == "" {
		cfg.HistoryLog = filepath.Join(filepath.Dir(absPath), "history.log")
	}

	GlobalConfig = cfg
	return cfg, nil
}
