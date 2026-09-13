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

// runMultiFile 多文件复制模式（两阶段单 channel，兼容 H3C MaxSessions=1）
func (m *CopyModule) runMultiFile(hs HostSession, flags *config.Flags, jsonList string) Result {
	var fileList []fileInfo
	if err := json.Unmarshal([]byte(jsonList), &fileList); err != nil {
		return Result{Success: false, Error: "解析文件列表失败: " + err.Error(), Change: false}
	}

	destRoot := strings.TrimRight(flags.Parameter["dest"], "/")
	dryRun := config.GlobalFlags.DryRun
	quiet := flags.Parameter["quiet"] == "true"

	// 关闭预建 Session 释放 SSH channel（H3C Comware 等限制 MaxSessions=1）
	if hs.Session != nil {
		hs.Session.Close()
	}

	type fileTarget struct {
		fi         fileInfo
		targetPath string
		targetDir  string
	}
	var targets []fileTarget
	for _, fi := range fileList {
		var targetPath string
		if len(fileList) == 1 {
			dest := flags.Parameter["dest"]
			if strings.HasSuffix(dest, "/") {
				targetPath = strings.TrimRight(dest, "/") + "/" + fi.FileName
			} else {
				targetPath = dest
			}
		} else {
			targetPath = destRoot + "/" + fi.RelativePath
		}
		targets = append(targets, fileTarget{
			fi:         fi,
			targetPath: targetPath,
			targetDir:  filepath.Dir(targetPath),
		})
	}

	// 单文件模式：SSH 检测 dest 是否为已存在的目录（末尾无 / 的情况）
	if len(fileList) == 1 {
		dest := flags.Parameter["dest"]
		if !strings.HasSuffix(dest, "/") {
			checkSession, err := hs.Client.NewSession()
			if err == nil {
				var out bytes.Buffer
				checkSession.Stdout = &out
				if err := checkSession.Run(fmt.Sprintf(`test -d %q && echo IS_DIR || echo NOT_DIR`, dest)); err == nil {
					if strings.TrimSpace(out.String()) == "IS_DIR" {
						fi := fileList[0]
						targets[0].targetPath = dest + "/" + fi.FileName
						targets[0].targetDir = filepath.Dir(targets[0].targetPath)
					}
				}
				checkSession.Close()
			}
		}
	}

	// Phase 1: MD5 校验（使用 SSH Session，每次创建后立即关闭）
	type md5Result struct {
		ft          fileTarget
		needCopy    bool
		skipReason  string
	}
	var md5Results []md5Result
	skipCount := 0
	failCount := 0
	var failMsgs []string
	var dryRunMsgs []string

	for _, ft := range targets {
		checkCmd := fmt.Sprintf(`read destMd5 _ <<< "$(md5sum %q 2>/dev/null)" && echo "$destMd5" || echo "NOT_FOUND"`, ft.targetPath)

		var checkOut, checkErr bytes.Buffer
		checkSession, err := hs.Client.NewSession()
		if err != nil {
			Debugf("copy模块 | %s: 创建检查会话失败 %v", ft.fi.FileName, err)
			failCount++
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 创建检查会话失败 %v", ft.fi.FileName, err))
			continue
		}
		checkSession.Stdout = &checkOut
		checkSession.Stderr = &checkErr
		if err := checkSession.Run(checkCmd); err != nil {
			Debugf("copy模块 | %s: MD5 检查失败 %v", ft.fi.FileName, err)
			failCount++
			failMsgs = append(failMsgs, fmt.Sprintf("%s: MD5 检查失败 %v", ft.fi.FileName, err))
			checkSession.Close()
			continue
		}
		checkSession.Close()

		remoteMd5 := strings.TrimSpace(checkOut.String())
		if remoteMd5 == ft.fi.Md5 {
			skipCount++
			md5Results = append(md5Results, md5Result{ft: ft, needCopy: false})
			dryRunMsgs = append(dryRunMsgs, fmt.Sprintf("%s → %s（内容一致，将跳过）", ft.fi.AbsPath, ft.targetPath))
		} else {
			md5Results = append(md5Results, md5Result{ft: ft, needCopy: true})
			dryRunMsgs = append(dryRunMsgs, fmt.Sprintf("%s → %s（将复制）", ft.fi.AbsPath, ft.targetPath))
		}
	}

	if dryRun {
		return Result{
			Success: true,
			Output:  strings.Join(dryRunMsgs, "\n"),
			Change:  false,
		}
	}

	// Phase 2: SFTP 传输（创建 SFTP 客户端，此时无 SSH Session 占用 channel）
	var totalSize int64
	for _, r := range md5Results {
		if r.needCopy {
			size, _ := strconv.ParseInt(r.ft.fi.Size, 10, 64)
			totalSize += size
		}
	}

	multiProgress := NewMultiFileProgress(totalSize)
	showProgress := !quiet && totalSize >= progressThreshold

	sftpClient, err := sftp.NewClient(hs.Client)
	if err != nil {
		return Result{Success: false, Error: "创建 SFTP 客户端失败: " + err.Error(), Change: false}
	}
	defer sftpClient.Close()

	successCount := 0
	for _, r := range md5Results {
		if !r.needCopy {
			continue
		}
		ft := r.ft

		if err := sftpClient.MkdirAll(ft.targetDir); err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 创建目录失败 %v", ft.fi.FileName, err))
			failCount++
			continue
		}

		Debugf("copy模块 | %s: 开始传输到 %s", ft.fi.FileName, ft.targetPath)
		srcFile, err := os.Open(ft.fi.AbsPath)
		if err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 打开源文件失败 %v", ft.fi.FileName, err))
			failCount++
			continue
		}

		dstFile, err := sftpClient.OpenFile(ft.targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		if err != nil {
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 创建远程文件失败 %v", ft.fi.FileName, err))
			srcFile.Close()
			failCount++
			continue
		}

		fileSize, _ := strconv.ParseInt(ft.fi.Size, 10, 64)
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
			failMsgs = append(failMsgs, fmt.Sprintf("%s: 传输失败 %v", ft.fi.FileName, err))
			failCount++
			continue
		}

		mode, _ := strconv.ParseUint(ft.fi.Mode, 8, 32)
		if err := sftpClient.Chmod(ft.targetPath, os.FileMode(mode)); err != nil {
			Debugf("copy模块 | %s: 设置权限失败 %v", ft.fi.FileName, err)
		}

		successCount++
	}

	// 单文件时保持原有输出格式
	if len(fileList) == 1 {
		fi := fileList[0]
		targetPath := md5Results[0].ft.targetPath
		if successCount == 1 {
			return Result{Success: true, Output: fmt.Sprintf("已成功复制 %s 到 %s（内容有更新）", fi.AbsPath, targetPath), Change: true}
		} else if skipCount == 1 {
			return Result{Success: true, Output: fmt.Sprintf("文件 %s 与远程 %s 内容一致，无需复制", fi.AbsPath, targetPath), Change: false}
		}
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
