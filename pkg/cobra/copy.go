package cobra

import (
	"encoding/json"
	"fastdp/module"
	"fastdp/pkg/config"
	"fastdp/pkg/exitcode"
	. "fastdp/utils"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// copy 命令
var copyCmd = &cobra.Command{
	Use:           "copy",
	Short:         "复制文件到远程主机",
	SilenceErrors: true,
	SilenceUsage:  true,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		sValues, _ := cmd.Flags().GetStringSlice("source")
		rValues, _ := cmd.Flags().GetStringSlice("recursive")
		dValue, _ := cmd.Flags().GetString("dest")

		if len(sValues) == 0 && len(rValues) == 0 {
			return fmt.Errorf("必须指定源文件(-s)或目录(-r)\n使用 --help 查看帮助信息")
		}
		if dValue == "" {
			return fmt.Errorf("必须指定目标位置(-d)\n使用 --help 查看帮助信息")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			Errorf("请指定目标主机组或主机\n示例:\n  fastdp copy -s app.conf -d /etc/ web\n  fastdp copy -r ./configs/ -d /etc/app/ all")
			os.Exit(exitcode.ParamError)
		}
		sValues, _ := cmd.Flags().GetStringSlice("source")
		rValues, _ := cmd.Flags().GetStringSlice("recursive")
		dValue, _ := cmd.Flags().GetString("dest")

		// 验证目标路径是绝对路径
		if !filepath.IsAbs(dValue) {
			Errorf("目标位置必须输入绝对路径，当前输入为:%s", dValue)
			os.Exit(exitcode.ParamError)
		}

		// 源含目录时，目标必须是目录
		if len(rValues) > 0 && !strings.HasSuffix(dValue, "/") {
			Errorf("复制目录时，目标路径必须是目录（以 / 结尾），当前输入为:%s", dValue)
			os.Exit(exitcode.ParamError)
		}

 		noKeepDir, _ := cmd.Flags().GetBool("no-keep-dir")
		quiet, _ := cmd.Flags().GetBool("quiet")

		// 收集所有源文件
		fileList, err := collectSourceFiles(sValues, rValues, noKeepDir)
		if err != nil {
			Errorf("收集源文件失败: %v", err)
			os.Exit(exitcode.ParamError)
		}

 		// 处理主机组参数
		config.GlobalFlags.HostInventory = args
		config.GlobalFlags.Parameter["dest"] = dValue
		config.GlobalFlags.Parameter["quiet"] = strconv.FormatBool(quiet)
		// 将文件列表 JSON 编码后传入
		jsonData, _ := json.Marshal(fileList)
		config.GlobalFlags.Parameter["file_list"] = string(jsonData)

		execHosts, err := GetInfo()
		if err != nil {
			Errorf("获取配置信息失败: %v", err)
			os.Exit(exitcode.ParamError)
		}
		hostSessions, failedHosts := SshConnect(execHosts)
		mod, err := module.GetModule("copy")
		if err != nil {
			Errorf("获取模块失败: %v", err)
			os.Exit(exitcode.InternalError)
		}
		os.Exit(execute(hostSessions, failedHosts, config.GlobalFlags, mod, "copy"))
	},
	Example: `  # 单文件复制
  fastdp copy -s app.conf -d /etc/ web
  fastdp copy -s run.sh -d /tmp/run.sh 192.168.1.101

  # 多文件复制
  fastdp copy -s a.conf -s b.sh -s c.py -d /tmp/ all

  # 目录递归复制（默认保留源目录名）
  fastdp copy -r ./configs/ -d /etc/app/ all
  # 结果：/etc/app/configs/xxx.yml

  # 目录递归复制（平铺，不保留源目录名）
  fastdp copy -r ./configs/ -d /etc/app/ --no-keep-dir all
  # 结果：/etc/app/xxx.yml

  # 混合使用
  fastdp copy -s app.conf -r ./scripts/ -d /opt/ all`,
}

func init() {
	copyCmd.Flags().StringSliceP("source", "s", nil, "源文件路径（可多次指定）")
	copyCmd.Flags().StringSliceP("recursive", "r", nil, "源目录路径（递归复制，可多次指定）")
	copyCmd.Flags().StringP("dest", "d", "", "目标路径 (必需)")
	copyCmd.Flags().Bool("no-keep-dir", false, "不保留源顶层目录，平铺复制目录内容到目标（默认保留目录结构）")
	copyCmd.Flags().BoolP("quiet", "q", false, "静默模式：不显示进度信息")
	_ = copyCmd.MarkFlagRequired("dest")
}

// fileInfo 文件元数据（管理端计算一次，所有目标机复用）
type fileInfo struct {
	AbsPath      string `json:"abs_path"`      // 本机绝对路径
	RelativePath string `json:"relative_path"` // 相对于源根目录的路径
	FileName     string `json:"file_name"`     // 文件名
	Size         string `json:"size"`
	Md5          string `json:"md5"`
	Mode         string `json:"mode"`
}

// collectSourceFiles 收集所有源文件并计算元数据（MD5/大小/权限）
func collectSourceFiles(sources []string, recursive []string, noKeepDir bool) ([]fileInfo, error) {
	fileList := make([]fileInfo, 0)

	// 添加单文件
	for _, src := range sources {
		abs, err := filepath.Abs(src)
		if err != nil {
			return nil, fmt.Errorf("获取绝对路径失败 %s: %v", src, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("源文件不存在: %s", abs)
			}
			return nil, fmt.Errorf("获取文件信息失败 %s: %v", abs, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s 是目录，请使用 -r 参数", src)
		}
		md5, err := FileMD5(abs)
		if err != nil {
			return nil, fmt.Errorf("计算MD5失败 %s: %v", abs, err)
		}
		fileList = append(fileList, fileInfo{
			AbsPath:      abs,
			RelativePath: info.Name(),
			FileName:     info.Name(),
			Size:         fmt.Sprintf("%d", info.Size()),
			Md5:          md5,
			Mode:         fmt.Sprintf("%03o", info.Mode()&os.ModePerm),
		})
	}

	// 递归遍历目录
	for _, dir := range recursive {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("获取绝对路径失败 %s: %v", dir, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("目录不存在: %s", abs)
			}
			return nil, fmt.Errorf("获取目录信息失败 %s: %v", abs, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("%s 不是目录", dir)
		}

 		// 计算相对路径时的根目录
		// keepDir=true (默认): 相对路径包含源目录名（如 test_keep/sub/file.txt）
		// no-keep-dir: 相对路径不包含源目录名（如 sub/file.txt）
		relRoot := filepath.Dir(abs)
		if noKeepDir {
			relRoot = abs
		}

		err = filepath.Walk(abs, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			md5, err := FileMD5(path)
			if err != nil {
				return fmt.Errorf("计算MD5失败 %s: %v", path, err)
			}
			rel, err := filepath.Rel(relRoot, path)
			if err != nil {
				return fmt.Errorf("计算相对路径失败 %s: %v", path, err)
			}
			fileList = append(fileList, fileInfo{
				AbsPath:      path,
				RelativePath: filepath.ToSlash(rel),
				FileName:     info.Name(),
				Size:         fmt.Sprintf("%d", info.Size()),
				Md5:          md5,
				Mode:         fmt.Sprintf("%03o", info.Mode()&os.ModePerm),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("遍历目录失败 %s: %v", dir, err)
		}
	}

	if len(fileList) == 0 {
		return nil, fmt.Errorf("未找到任何源文件")
	}
	return fileList, nil
}
