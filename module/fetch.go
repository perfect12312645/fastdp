package module

import (
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/pkg/sftp"
)

// 进度条显示阈值：单文件 < 1MB 不显示进度条
const fetchProgressThreshold = 1 * 1024 * 1024 // 1MB

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
	remotePath := flags.Parameter["remote"]
	localDest := flags.Parameter["dest"]
	noIpDir := flags.Parameter["no_ip_dir"]
	recursive := flags.Parameter["recursive"] == "true"

	if localDest == "" {
		localDest = config.GlobalConfig.DefaultFetchPath
	}
	if localDest == "" {
		localDest = "./fastdp-fetch"
	}
	dryRun := config.GlobalFlags.DryRun

	// fetch 模块不创建预建 Session（SshConnect 中已跳过），直接用 Client 创建 SFTP
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

	isRecursive := recursive || strings.HasSuffix(remotePath, "/")

	var files []string
	if isRecursive {
		files, err = walkSFTPDir(sftpClient, remotePath)
		if err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("遍历远程目录失败: %v", err),
				Change:  false,
			}
		}
	} else {
		files, err = sftpClient.Glob(remotePath)
		if err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("匹配文件失败: %v", err),
				Change:  false,
			}
		}
	}

	if !isRecursive {
		var filtered []string
		for _, f := range files {
			if stat, statErr := sftpClient.Stat(f); statErr == nil && stat.IsDir() {
				continue
			}
			filtered = append(filtered, f)
		}
		if len(filtered) == 0 && len(files) > 0 {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("路径 %q 是一个目录，请使用递归模式拉取：路径末尾加 / 或 --recursive", remotePath),
				Change:  false,
			}
		}
		files = filtered
	}

	if len(files) == 0 {
		return Result{
			Success: true,
			Output:  "未匹配到任何文件",
			Error:   "",
			Change:  false,
		}
	}

	if dryRun {
		var preview []string
		for _, f := range files {
			localFile := m.buildLocalPath(f, remotePath, localDest, hs.Addr, isRecursive, noIpDir)
			preview = append(preview, fmt.Sprintf("%s → %s", f, localFile))
		}
		return Result{
			Success: true,
			Output:  strings.Join(preview, "\n"),
			Error:   "",
			Change:  false,
		}
	}

	var downloadedFiles []string
	for _, f := range files {
		localFile := m.buildLocalPath(f, remotePath, localDest, hs.Addr, isRecursive, noIpDir)
		localDir := filepath.Dir(localFile)

		if err := os.MkdirAll(localDir, 0755); err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("创建目录失败 %s: %v", localDir, err),
				Change:  false,
			}
		}

		srcFile, err := sftpClient.Open(f)
		if err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("打开远程文件失败 %s: %v", f, err),
				Change:  false,
			}
		}

		stat, err := srcFile.Stat()
		if err != nil {
			srcFile.Close()
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("获取文件信息失败 %s: %v", f, err),
				Change:  false,
			}
		}

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

		if stat.Size() >= fetchProgressThreshold {
			pw := getFetchProgressWriter()
			tracker := &progress.Tracker{
				Message: fmt.Sprintf("%s %s", hs.Addr, f),
				Total:   stat.Size(),
				Units:   progress.UnitsBytes,
			}
			pw.AppendTracker(tracker)
			err = copyWithProgress(srcFile, dstFile, tracker)
			if err != nil {
				tracker.MarkAsErrored()
				srcFile.Close()
				dstFile.Close()
				return Result{Success: false, Output: "", Error: fmt.Sprintf("下载失败 %s: %v", f, err), Change: false}
			}
			tracker.MarkAsDone()
		} else {
			err = copySimple(srcFile, dstFile)
			if err != nil {
				srcFile.Close()
				dstFile.Close()
				return Result{Success: false, Output: "", Error: fmt.Sprintf("下载失败 %s: %v", f, err), Change: false}
			}
		}

		srcFile.Close()
		dstFile.Close()
		downloadedFiles = append(downloadedFiles, localFile)
	}

	return Result{
		Success: true,
		Output:  fmt.Sprintf("下载成功: %v", downloadedFiles),
		Error:   "",
		Change:  true,
	}
}

func (m *FetchModule) buildLocalPath(remoteFile, remoteRoot, localDest, addr string, recursive bool, noIpDir string) string {
	if recursive {
		// 递归模式：保留从 / 开始的完整路径
		// remoteRoot = "/var/log/app/", remoteFile = "/var/log/app/sub/b.log"
		// → localDest/addr/var/log/app/sub/b.log
		rel := strings.TrimPrefix(remoteFile, "/")
		return filepath.Join(localDest, addr, rel)
	}

	filename := filepath.Base(remoteFile)
	if noIpDir == "true" {
		return filepath.Join(localDest, fmt.Sprintf("%s_%s", addr, filename))
	}
	return filepath.Join(localDest, addr, filename)
}

func walkSFTPDir(client *sftp.Client, root string) ([]string, error) {
	var files []string
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := client.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			fullPath := path.Join(dir, entry.Name())
			if entry.IsDir() {
				if err := walk(fullPath); err != nil {
					return err
				}
			} else {
				files = append(files, fullPath)
			}
		}
		return nil
	}
	err := walk(root)
	return files, err
}

func copyWithProgress(src io.Reader, dst io.Writer, tracker *progress.Tracker) error {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
			tracker.Increment(int64(n))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func copySimple(src io.Reader, dst io.Writer) error {
	_, err := io.Copy(dst, src)
	return err
}

// 注册模块
func init() {
	Register("fetch", NewFetchModule)
}
