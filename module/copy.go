package module

import (
	"bytes"
	. "fastdp/pkg/flags"
	. "fastdp/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CopyModule struct {
	command string // 要执行的命令（从参数解析）
}

// NewShellModule 创建 Shell 模块实例
func NewCopyModule() Module {
	return &CopyModule{}
}

// SetParams 解析参数（格式：直接是命令字符串，如 "ls -l /tmp"）
func (m *CopyModule) SetParams(params string) error {
	m.command = params // 简单直接，参数就是命令本身
	return nil
}
func (m *CopyModule) Run(hs HostSession, flags *Flags) Result {
	// 解析参数：src=source_path dest=destination_path content="hello world"
	params := make(map[string]string)
	for _, part := range strings.Fields(*flags.Parameter) {
		if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
			params[kv[0]] = kv[1]
		}
	}

	src, srcExists := params["src"]
	dest, destExists := params["dest"]
	//content, contentExists := params["content"]
	if !filepath.IsAbs(dest) {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("目标文件位置必须输入绝对路径，当前输入为:%s", dest),
			Change:  false,
		}
	}
	if !destExists || !srcExists {
		return Result{
			Success: false,
			Output:  "",
			Error:   "缺少必需参数：dest 是必需的，且 src 或 content 至少需要一个",
			Change:  false,
		}
	}

	// 读取源文件内容
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   "获取源文件/目录失败",
			Change:  false,
		}
	}
	f, err := os.Stat(srcAbs)
	Logger.Sugar().Debugf("源文件名%s", f.Name())
	if err != nil {
		if os.IsNotExist(err) {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("源文件/目录不存在:%s", srcAbs),
				Change:  false,
			}
		}
	}
	srcContent, err := os.ReadFile(srcAbs)
	if err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("读取本地文件失败:%s", err.Error()),
			Change:  false,
		}
	}
	srcMd5, err := FileMD5(srcAbs)
	if err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("计算源文件md5失败:%s", err.Error()),
			Change:  false,
		}
	}
	// 检查目标文件是否存在且内容相同（判断是否需要变更）
	checkCmd := fmt.Sprintf(`
if [ -f '%s' ]; then
  destMd5=$(md5sum '%s' | awk '{print $1}')  # 提取MD5值（排除文件名）
  if [ "$destMd5" = '%s' ]; then
    echo "SAME"
  else
    echo "DIFFER"
  fi
else
  echo "FILE_NOT_FOUND"
fi 
`, dest, dest, srcMd5)
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
	changed := false

	// 判断是否需要变更（文件不存在或内容不同）
	if destContent == "FILE_NOT_FOUND" || destContent == "DIFFER" {
		changed = true

		// 创建临时文件并写入内容
		tmpFile := fmt.Sprintf("/tmp/fastdp_copy_%d", time.Now().UnixNano())
		// 给临时文件赋予与目标文件一致的权限
		writeCmd := fmt.Sprintf(`cat > "%s" && chmod 644 "%s" && mv "%s" "%s"`, tmpFile, tmpFile, tmpFile, dest)

		// 执行写入命令
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

		// 写入文件内容
		if _, err := stdin.Write(srcContent); err != nil {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("Failed to write to stdin: %v", err),
				Change:  false,
			}
		}
		stdin.Close() // 关闭输入流，让命令继续执行

		if err := session.Wait(); err != nil {
			return Result{
				Success: false,
				Output:  outBuf.String(),
				Error:   fmt.Sprintf("%s\n%s", errBuf.String(), err.Error()),
				Change:  false,
			}
		}
	}

	return Result{
		Success: true,
		Output:  fmt.Sprintf("Copied %s to %s", src, dest),
		Error:   "",
		Change:  changed,
	}
}
func init() {
	Register("copy", NewCopyModule) // 注册 "shell" 模块
}
