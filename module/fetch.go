package module

import (
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/pkg/sftp"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ===================== 全局进度条（单例，所有主机共享）=====================
var (
	fetchProgressWriter progress.Writer
	fetchProgressOnce   sync.Once
)

// 获取进度条实例（初始化一次）
func getFetchProgressWriter() progress.Writer {
	fetchProgressOnce.Do(func() {
		fetchProgressWriter = progress.NewWriter()
		fetchProgressWriter.SetOutputWriter(os.Stderr)
		fetchProgressWriter.Style().Colors = progress.StyleColorsExample
		fetchProgressWriter.Style().Visibility.ETA = true
		fetchProgressWriter.Style().Visibility.Speed = true
		fetchProgressWriter.Style().Visibility.Percentage = true
		// ✅ 限制消息部分最大宽度（字符数），避免超长导致换行
		fetchProgressWriter.SetMessageWidth(45)
		fetchProgressWriter.SetAutoStop(true)

		go fetchProgressWriter.Render()

	})
	return fetchProgressWriter
}

// FetchModule 实现 Module 接口，用于批量远程拉取文件
type FetchModule struct {
}

// NewFetchModule 创建 Fetch 模块实例
func NewFetchModule() Module {
	return &FetchModule{}
}

func (m *FetchModule) Run(hs HostSession, flags *config.Flags) Result {
	// 1. 获取参数
	remotePath := flags.Parameter["remote"] // 远程文件（支持通配符 /tmp/sec*）
	localDest := flags.Parameter["dest"]    // 本地保存目录
	noIpDir := flags.Parameter["no_ip_dir"] // 是否去掉IP目录

	if localDest == "" {
		localDest = config.GlobalConfig.DefaultFetchPath
	}
	if localDest == "" {
		localDest = "./fastdp-fetch"
	}
	// 2. 创建 SFTP 客户端
	sftpClient, err := sftp.NewClient(hs.Client)

	if err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("sftp 初始化失败: %v", err),
			Change:  false,
		}
	}
	defer sftpClient.Close()

	// 3. 匹配远程文件（支持 * ?）
	files, err := sftpClient.Glob(remotePath)
	if err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("匹配文件失败: %v", err),
			Change:  false,
		}
	}

	if len(files) == 0 {
		return Result{
			Success: true,
			Output:  "未匹配到任何文件",
			Error:   "",
			Change:  false,
		}
	}

	// 4. 逐个下载文件
	var downloadedFiles []string
	for _, f := range files {
		var localFile string
		filename := filepath.Base(f)

		if noIpDir == "true" {
			// ✅ 启用：不创建IP目录 → 文件名 = IP_原文件名
			localFile = filepath.Join(localDest, fmt.Sprintf("%s_%s", hs.Addr, filename))
		} else {
			// ✅ 默认：创建IP目录
			localFile = filepath.Join(localDest, hs.Addr, filename)
		}
		localDir := filepath.Dir(localFile)
		// 2. 获取进度条总管
		pw := getFetchProgressWriter()
		// 创建本地目录
		if err := os.MkdirAll(localDir, 0755); err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("创建目录失败 %s: %v", localDir, err),
				Change:  false,
			}
		}

		// 打开远程文件
		srcFile, err := sftpClient.Open(f)
		if err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("打开远程文件失败 %s: %v", f, err),
				Change:  false,
			}
		}

		// 获取文件大小
		stat, err := srcFile.Stat()
		if err != nil {
			srcFile.Close()
			return Result{
				Success: false,
				Error:   fmt.Sprintf("获取文件信息失败 %s: %v", f, err),
			}
		}

		// 创建本地文件
		dstFile, err := os.Create(localFile)
		if err != nil {
			srcFile.Close()
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("创建本地文件失败 %s: %v", localFile, err),
				Change:  false,
			}
		}
		// ===================== 进度条 =====================
		tracker := &progress.Tracker{
			Message: fmt.Sprintf("%s %s", hs.Addr, f),
			Total:   stat.Size(),
			Units:   progress.UnitsBytes,
		}
		pw.AppendTracker(tracker)

		// 带进度拷贝
		buf := make([]byte, 32<<10) // 32KB 缓冲
		for {
			n, err := srcFile.Read(buf)
			if n > 0 {
				_, _ = dstFile.Write(buf[:n])
				tracker.Increment(int64(n))
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				tracker.MarkAsErrored()
				srcFile.Close()
				dstFile.Close()
				return Result{
					Success: false,
					Error:   fmt.Sprintf("下载失败 %s: %v", f, err),
				}
			}
		}

		srcFile.Close()
		dstFile.Close()
		tracker.MarkAsDone()

		downloadedFiles = append(downloadedFiles, localFile)
	}

	return Result{
		Success: true,
		Output:  fmt.Sprintf("下载成功: %v", downloadedFiles),
		Error:   "",
		Change:  true,
	}
}

// 注册模块
func init() {
	Register("fetch", NewFetchModule)
}
