package cobra

import (
	"fastdp/module"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// getCheckScriptPath 获取巡检脚本路径：用户目录优先，系统目录兜底
func getCheckScriptPath() string {
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userScript := filepath.Join(homeDir, ".fastdp", "fastdp-check.sh")
		if _, err := os.Stat(userScript); err == nil {
			return userScript
		}
	}
	return "/etc/fastdp/fastdp-check.sh"
}

var checkCmd = &cobra.Command{
	Use:           "check",
	Short:         "批量主机环境检查",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		vertical, _ := cmd.Flags().GetBool("vertical")
		format, _ := cmd.Flags().GetString("format")

		// 互斥判断：-g 和 -f 不能同时存在
		if vertical && format != "" {
			return fmt.Errorf("参数冲突：-g（竖向输出）和 -f（导出格式）不能同时使用")
		}
		if format != "" {
			validFormats := map[string]bool{"csv": true, "md": true, "html": true, "json": true}
			if !validFormats[format] {
				return fmt.Errorf("无效格式：%s，合法值：csv|md|html|json", format)
			}
		}
		return nil
	},
 	Run: func(cmd *cobra.Command, args []string) {
		// 未指定目标主机，给出友好提示
		if len(args) == 0 {
			Errorf("请指定目标主机组或主机\n使用 --help 查看帮助信息\n示例:\n  fastdp check all\n  fastdp check web\n  fastdp check 192.168.1.100")
			os.Exit(exitcode.ParamError)
		}

		config.GlobalFlags.HostInventory = args

		vertical, _ := cmd.Flags().GetBool("vertical")
		if vertical {
			config.GlobalFlags.Parameter["vertical"] = "true"
		}
		outputFormat, _ := cmd.Flags().GetString("format")
		config.GlobalFlags.Parameter["format"] = outputFormat
		only, _ := cmd.Flags().GetString("only")
		if only != "" {
			config.GlobalFlags.Parameter["only"] = only
		}

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}

		// --list-fields / -l：列出所有可用字段后退出
		listFields, _ := cmd.Flags().GetBool("list-fields")
		if listFields {
			listCheckFields()
			os.Exit(exitcode.Success)
		}

		// 解析脚本，根据 --only 过滤，直接传入内容（无需临时文件）
		scriptPath := getCheckScriptPath()
		parsedScript, err := ParseCheckScript(scriptPath, only)
		if err != nil {
			Errorf("解析脚本失败: %v", err)
			os.Exit(exitcode.ParamError)
		}
		config.GlobalFlags.Parameter["script_content"] = parsedScript

		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("script")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(exitcode.InternalError)
		}
		os.Exit(execute(hostSessions, failedHosts, config.GlobalFlags, mod, "check"))
	},
	Example: `
  # 对所有主机执行环境检查（默认表格输出）
  fastdp check all

  # 只检查 CPU 和内存
  fastdp check all --only cpu_cores,cpu_model,mem

  # 列出所有可用字段
  fastdp check all -l

  # 竖向格式化输出
  fastdp check all -g

  # 导出巡检报告
  fastdp check all -f csv  > report.csv
  fastdp check all -f json > report.json
`,
}

func init() {
	checkCmd.Flags().BoolP("vertical", "g", false, "竖向格式化输出 (类似 mysql \\G)")
	checkCmd.Flags().StringP("format", "f", "", "输出格式: csv|md|html|json")
	checkCmd.Flags().String("only", "", "只检查指定字段（逗号分隔，如 hostname,os,mem）")
	checkCmd.Flags().BoolP("list-fields", "l", false, "列出所有可用的检查字段 key")
}

// listCheckFields 列出脚本中所有可用的字段 key
func listCheckFields() {
	scriptPath := getCheckScriptPath()
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		Errorf("读取脚本失败: %v", err)
		return
	}

	seen := make(map[string]bool)
	keys := make([]string, 0)
	lines := strings.Split(string(data), "\n")
	beginRegex := regexp.MustCompile(`^#\s*BEGIN\s+(\S+)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳过空行和纯注释行（保留 BEGIN）
		if line == "" || (strings.HasPrefix(line, "#") && !beginRegex.MatchString(line)) {
			continue
		}
		// BEGIN 标记
		if m := beginRegex.FindStringSubmatch(line); m != nil {
			key := m[1]
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
			continue
		}
		// 单行 echo "key=value"（跳过注释掉的 echo）
		if strings.HasPrefix(line, "#") {
			continue
		}
		if key := ExtractKeyFromEcho(line); key != "" && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}

	fmt.Println("可用字段 key：")
	for _, k := range keys {
		fmt.Printf("  %s\n", k)
	}
}
