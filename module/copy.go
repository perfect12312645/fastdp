package module

import (
	"bytes"
	"encoding/json"
	"fastdp/pkg/config"
	. "fastdp/utils"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
)

// 进度展示阈值：10MB，小于此值不展示进度
const progressThreshold = 10 * 1024 * 1024

type CopyModule struct{}

type ProgressReader struct {
	reader        io.Reader
	totalSize     int64
	current       int64
	host          string
	progressCb    func(host string, current, total int64)
	lastPrintTime time.Time
}

func NewProgressReader(r io.Reader, totalSize int64, host string, cb func(string, int64, int64)) *ProgressReader {
	return &ProgressReader{
		reader:        r,
		totalSize:     totalSize,
		host:          host,
		progressCb:    cb,
		lastPrintTime: time.Time{},
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&pr.current, int64(n))
		if pr.progressCb != nil {
			pr.progressCb(pr.host, pr.current, pr.totalSize)
		}
	}
	return n, err
}

// MultiFileProgress 多文件总进度追踪
type MultiFileProgress struct {
	totalSize     int64
	current       int64
	lastPrintTime time.Time
}

func NewMultiFileProgress(totalSize int64) *MultiFileProgress {
	return &MultiFileProgress{
		totalSize:     totalSize,
		lastPrintTime: time.Time{},
	}
}

func (mp *MultiFileProgress) AddTransferred(n int64) {
	atomic.AddInt64(&mp.current, int64(n))
}

func (mp *MultiFileProgress) TryPrint(host string) {
	now := time.Now()
	if now.Sub(mp.lastPrintTime) >= 3*time.Second || atomic.LoadInt64(&mp.current) >= mp.totalSize {
		percent := float64(atomic.LoadInt64(&mp.current)) / float64(mp.totalSize) * 100
		if percent > 100 {
			percent = 100
		}
		fmt.Printf("复制进度 | 主机: %s | 已传输: %.1f%%\n", host, percent)
		mp.lastPrintTime = now
	}
}

func NewCopyModule() Module {
	return &CopyModule{}
}

// fileInfo 文件元数据（与 cobra/copy.go 同步）
type fileInfo struct {
	AbsPath      string `json:"abs_path"`
	RelativePath string `json:"relative_path"`
	FileName     string `json:"file_name"`
	Size         string `json:"size"`
	Md5          string `json:"md5"`
	Mode         string `json:"mode"`
}

func (m *CopyModule) Run(hs HostSession, flags *config.Flags) Result {
	jsonList := flags.Parameter["file_list"]
	if jsonList == "" {
		return Result{Success: false, Error: "缺少源文件列表", Change: false}
	}
	return m.runMultiFile(hs, flags, jsonList)
}

// runMultiFile 多文件复制模式（使用 SFTP）
func (m *CopyModule) runMultiFile(hs HostSession, flags *config.Flags, jsonList string) Result {
	var fileList []fileInfo
	if err := json.Unmarshal([]byte(jsonList), &fileList); err != nil {
		return Result{Success: false, Error: "解析文件列表失败: " + err.Error(), Change: false}
	}

	destRoot := strings.TrimRight(flags.Parameter["dest"], "/")

	// 计算总大小
	var totalSize int64
	for _, fi := range fileList {
		size, _ := strconv.ParseInt(fi.Size, 10, 64)
		totalSize += size
	}

	multiProgress := NewMultiFileProgress(totalSize)
	quiet := flags.Parameter["quiet"] == "true"
	showProgress := !quiet && totalSize >= progressThreshold
	dryRun := config.GlobalFlags.DryRun

	successCount := 0
	skipCount := 0
	failCount := 0
	var failMsgs []string
	var dryRunMsgs []string

	// 创建 SFTP 客户端
	sftpClient, err := sftp.NewClient(hs.Client)
	if err != nil {
		return Result{Success: false, Error: "创建 SFTP 客户端失败: " + err.Error(), Change: false}
	}
	defer sftpClient.Close()

	// 单文件时预计算目标路径（避免循环内重复判断）
	var singleFileTarget string
	if len(fileList) == 1 {
		dest := flags.Parameter["dest"]
		fi := fileList[0]
		if strings.HasSuffix(dest, "/") {
			singleFileTarget = strings.TrimRight(dest, "/") + "/" + fi.FileName
		} else if remoteInfo, err := sftpClient.Stat(dest); err == nil && remoteInfo.IsDir() {
			singleFileTarget = dest + "/" + fi.FileName
		} else {
			singleFileTarget = dest
		}
	}

	for idx, fi := range fileList {
		// 构造目标路径
		var targetPath string
		if len(fileList) == 1 {
			targetPath = singleFileTarget
		} else {
			targetPath = destRoot + "/" + fi.RelativePath
		}
		targetDir := filepath.Dir(targetPath)

		// MD5 检查
		checkCmd := fmt.Sprintf(`read destMd5 _ <<< "$(md5sum %q 2>/dev/null)" && echo "$destMd5" || echo "NOT_FOUND"`, targetPath)

		var checkOut, checkErr bytes.Buffer

		if idx == 0 {
			hs.Session.Stdout = &checkOut
			hs.Session.Stderr = &checkErr
			if err := hs.Session.Run(checkCmd); err != nil {
				Debugf("copy模块 | %s: MD5 检查失败 %v", fi.FileName, err)
				failCount++
				continue
			}
		} else {
			checkSession, err := hs.Client.NewSession()
			if err != nil {
				Debugf("copy模块 | %s: 创建检查会话失败 %v", fi.FileName, err)
				failCount++
				continue
			}
			checkSession.Stdout = &checkOut
			checkSession.Stderr = &checkErr
			if err := checkSession.Run(checkCmd); err != nil {
				Debugf("copy模块 | %s: MD5 检查失败 %v", fi.FileName, err)
				failCount++
				checkSession.Close()
				continue
			}
			checkSession.Close()
		}

		remoteMd5 := strings.TrimSpace(checkOut.String())
		if remoteMd5 == fi.Md5 {
			skipCount++
			if dryRun {
				dryRunMsgs = append(dryRunMsgs, fmt.Sprintf("%s → %s（内容一致，将跳过）", fi.AbsPath, targetPath))
			}
			continue
		}

		// dry-run 模式：只输出预览，不实际复制
		if dryRun {
			dryRunMsgs = append(dryRunMsgs, fmt.Sprintf("%s → %s（将复制）", fi.AbsPath, targetPath))
			successCount++
			continue
		}

		// 需要复制 - 使用 SFTP
		if err := sftpClient.MkdirAll(targetDir); err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 创建目录失败 %v", fi.FileName, err))
			failCount++
			continue
		}

		Debugf("copy模块 | %s: 开始传输到 %s", fi.FileName, targetPath)
		srcFile, err := os.Open(fi.AbsPath)
		if err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 打开源文件失败 %v", fi.FileName, err))
			failCount++
			continue
		}

		dstFile, err := sftpClient.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 创建远程文件失败 %v", fi.FileName, err))
			srcFile.Close()
			failCount++
			continue
		}

		fileSize, _ := strconv.ParseInt(fi.Size, 10, 64)

		var reader io.Reader = srcFile
		if showProgress {
			reader = &progressTrackerReader{
				reader:   srcFile,
				fileSize: fileSize,
				progress: multiProgress,
				host:     hs.Addr,
			}
		}

		_, err = io.Copy(dstFile, reader)
		srcFile.Close()
		dstFile.Close()

		if err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 传输失败 %v", fi.FileName, err))
			failCount++
			continue
		}

		// 设置权限
		mode, _ := strconv.ParseUint(fi.Mode, 8, 32)
		if err := sftpClient.Chmod(targetPath, os.FileMode(mode)); err != nil {
			Debugf("copy模块 | %s: 设置权限失败 %v", fi.FileName, err)
		}

		successCount++
	}

	// dry-run 模式：返回预览信息
	if dryRun {
		return Result{
			Success: true,
			Output:  strings.Join(dryRunMsgs, "\n"),
			Change:  false,
		}
	}

	// 单文件时保持原有输出格式
	if len(fileList) == 1 {
		fi := fileList[0]
		if successCount == 1 {
			return Result{Success: true, Output: fmt.Sprintf("已成功复制 %s 到 %s（内容有更新）", fi.AbsPath, singleFileTarget), Change: true}
		} else if skipCount == 1 {
			return Result{Success: true, Output: fmt.Sprintf("文件 %s 与远程 %s 内容一致，无需复制", fi.AbsPath, singleFileTarget), Change: false}
		}
		// 失败时返回具体错误
		if len(failMsgs) > 0 {
			return Result{Success: false, Output: "", Error: failMsgs[0], Change: false}
		}
	}

	return Result{
		Success: failCount == 0,
		Output:  fmt.Sprintf("复制完成：%d 个文件复制成功，%d 个跳过（内容一致），%d 个失败", successCount, skipCount, failCount),
		Error:   strings.Join(failMsgs, "\n"),
		Change:  successCount > 0,
	}
}

// progressTrackerReader 追踪多文件总进度
type progressTrackerReader struct {
	reader   io.Reader
	fileSize int64
	current  int64
	progress *MultiFileProgress
	host     string
}

func (r *progressTrackerReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		atomic.AddInt64(&r.current, int64(n))
		r.progress.AddTransferred(int64(n))
		r.progress.TryPrint(r.host)
	}
	return n, err
}

func init() {
	Register("copy", NewCopyModule)
}
