package module

import (
	"bytes"
	"fastdp/pkg/config"
	. "fastdp/pkg/log"
	. "fastdp/utils"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type CopyModule struct {
}

type ProgressReader struct {
	reader        io.Reader                               // 原始文件 Reader
	totalSize     int64                                   // 文件总大小（字节）
	current       int64                                   // 当前已传输字节（原子操作计数）
	host          string                                  // 目标主机地址
	progressCb    func(host string, current, total int64) // 进度更新回调函数
	lastPrintTime time.Time                               // 上次打印时间（初始化零值）
}

// NewProgressReader 创建 ProgressReader 实例，初始化上次打印时间为零值
func NewProgressReader(r io.Reader, totalSize int64, host string, cb func(string, int64, int64)) *ProgressReader {
	return &ProgressReader{
		reader:        r,
		totalSize:     totalSize,
		host:          host,
		progressCb:    cb,
		lastPrintTime: time.Time{}, // 零值时间，确保第一次调用必打印
	}
}

// Read 实现 io.Reader 接口，每次读取后触发进度回调
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p) // 读取数据
	if n > 0 {
		atomic.AddInt64(&pr.current, int64(n))           // 原子累加已传输字节
		pr.progressCb(pr.host, pr.current, pr.totalSize) // 触发回调
	}
	return n, err
}

func NewCopyModule() Module {
	return &CopyModule{}
}

func GetSource(flags *config.Flags) (*config.Flags, error) {
	// （原有逻辑不变）
	src := flags.Parameter["source"]
	dest := flags.Parameter["dest"]

	if !filepath.IsAbs(dest) {
		return nil, fmt.Errorf("目标文件位置必须输入绝对路径，当前输入为:%s", dest)
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return nil, fmt.Errorf("获取源文件失败%s", err.Error())
	}
	srcf, err := os.Stat(srcAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("源文件不存在: %s", srcAbs)
		}
		return nil, fmt.Errorf("获取源文件信息失败: %s", err.Error())
	}
	Logger.Sugar().Debugf("源文件名%s", srcf.Name())
	if srcf.IsDir() {
		return nil, fmt.Errorf("不支持目录复制: %s", srcAbs)
	}

	srcMd5, err := FileMD5(srcAbs)
	if err != nil {
		return nil, fmt.Errorf("计算源文件md5失败:%s", err.Error())
	}
	flags.Parameter["srcFileSize"] = fmt.Sprintf("%d", srcf.Size())
	flags.Parameter["srcAbsPath"] = srcAbs
	flags.Parameter["md5"] = srcMd5
	flags.Parameter["srcFileName"] = srcf.Name()
	flags.Parameter["srcFileMode"] = fmt.Sprintf("%03o", srcf.Mode()&os.ModePerm)

	return flags, nil
}

func (m *CopyModule) Run(hs HostSession, flags *config.Flags) Result {
	srcAbs := flags.Parameter["srcAbsPath"]
	dest := flags.Parameter["dest"]
	srcMd5 := flags.Parameter["md5"]

	// （目标文件检查逻辑不变）
	checkCmd := fmt.Sprintf(`
dest_path=%q
src_filename=%q
dest_path=${dest_path%%/}
if [ -d "$dest_path" ]; then
  target_path="$dest_path/$src_filename"
else
  target_path="$dest_path"
fi
if [ -f "$target_path" ]; then
  destMd5=$(md5sum "$target_path" | awk '{print $1}')
  if [ "$destMd5" = '%s' ]; then
    echo -n "SAME"
  else
    echo -n "DIFFER"
  fi
else
  echo -n "FILE_NOT_FOUND"
fi
`, dest, flags.Parameter["srcFileName"], srcMd5)
	Logger.Sugar().Debugf("copy模块 | 主机: %s | 开始计算目标文件MD5 | 时间: %v", hs.Addr, time.Now())
	var checkOut, checkErr bytes.Buffer
	hs.Session.Stdout = &checkOut
	hs.Session.Stderr = &checkErr

	if err := hs.Session.Run(checkCmd); err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("Failed to check destination file: %v\n%s", err, checkErr.String()),
			Change:  false,
		}
	}

	destContent := checkOut.String()
	destContentTrimmed := strings.TrimSpace(destContent)
	Logger.Sugar().Debugf("copy模块 | 主机: %s | 远程文件状态返回: %s", hs.Addr, destContentTrimmed)
	changed := false
	Logger.Sugar().Debugf("copy模块 | 主机: %s | 完成计算目标文件MD5 | 时间: %v", hs.Addr, time.Now())

	if destContentTrimmed == "FILE_NOT_FOUND" || destContentTrimmed == "DIFFER" {
		changed = true
		permissionStr := flags.Parameter["srcFileMode"]
		writeCmd := fmt.Sprintf(`
dest_path=%q
src_filename=%q
dest_path=${dest_path%%/}
if [ -d "$dest_path" ]; then
  target_path="$dest_path/$src_filename"
else
  target_path="$dest_path"
fi
target_dir=$(dirname "$target_path")
tmpFile="$target_dir/fastdp_copy_$(date +%%s%%N)"
mkdir -p "$target_dir"
cat > "$tmpFile" && chmod %s "$tmpFile" && mv "$tmpFile" "$target_path"
`, dest, flags.Parameter["srcFileName"], permissionStr)
		Logger.Sugar().Debugf("copy模块 | 主机: %s | 开始传输文件 | 源: %s | 目标: %s", hs.Addr, srcAbs, dest)

		// 打开源文件
		srcFile, err := os.Open(srcAbs)
		if err != nil {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("打开源文件失败: %v", err),
			}
		}
		defer srcFile.Close()

		// 解析文件总大小
		totalSize, err := strconv.ParseInt(flags.Parameter["srcFileSize"], 10, 64)
		if err != nil {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("解析文件大小失败: %v", err),
			}
		}

		// 创建带时间间隔控制的进度阅读器
		progressReader := NewProgressReader(srcFile, totalSize, hs.Addr, nil)
		// 定义基于3秒间隔的回调函数（闭包捕获progressReader，用于更新lastPrintTime）
		progressReader.progressCb = func(host string, current, total int64) {
			if total == 0 {
				return
			}
			now := time.Now()
			// 满足以下条件之一则打印：
			// 1. 距离上次打印已超过3秒；2. 文件传输完成（current == total）
			if now.Sub(progressReader.lastPrintTime) >= 3*time.Second || current == total {
				percent := float64(current) / float64(total) * 100
				Logger.Sugar().Infof("复制进度 | 主机: %s | 已传输: %.2f%% (%d/%d bytes)",
					host, percent, current, total)
				progressReader.lastPrintTime = now // 更新上次打印时间
			}
		}

		// 执行SSH传输
		session, err := hs.Client.NewSession()
		if err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("Failed to create new SSH session: %v", err),
				Change:  false,
			}
		}
		defer session.Close()

		stdin, err := session.StdinPipe()
		if err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("Failed to get stdin pipe: %v", err),
				Change:  false,
			}
		}

		var outBuf, errBuf bytes.Buffer
		session.Stdout = &outBuf
		session.Stderr = &errBuf

		if err := session.Start(writeCmd); err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("Failed to start write command: %v", err),
				Change:  false,
			}
		}

		// 流式传输文件内容（触发进度回调）
		_, err = io.Copy(stdin, progressReader)
		if err != nil {
			return Result{
				Success: false,
				Error:   fmt.Sprintf("写入文件内容失败: %v", err),
			}
		}
		stdin.Close()

		Logger.Sugar().Debugf("copy模块 | 主机: %s | 完成文件传输写入 | 等待远程处理结果", hs.Addr)
		if err := session.Wait(); err != nil {
			return Result{
				Success: false,
				Output:  outBuf.String(),
				Error:   fmt.Sprintf("%s\n%s", errBuf.String(), err.Error()),
				Change:  false,
			}
		}
		return Result{
			Success: true,
			Output:  fmt.Sprintf("已成功复制 %s 到 %s（内容有更新）", srcAbs, dest),
			Error:   "",
			Change:  changed,
		}
	} else if destContentTrimmed == "SAME" {
		return Result{
			Success: true,
			Output:  fmt.Sprintf("文件 %s 与远程 %s 内容一致，无需复制", srcAbs, dest),
			Error:   "",
			Change:  changed,
		}
	}
	return Result{
		Success: false,
		Output:  "",
		Error:   "未预期的远程文件状态: " + destContent,
		Change:  false,
	}
}

func init() {
	Register("copy", NewCopyModule)
}
