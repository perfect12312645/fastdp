package module

import (
	"bytes"
	. "fastdp/pkg/cobra"
	. "fastdp/utils"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CopyModule struct {
}

// NewShellModule 创建 Shell 模块实例
func NewCopyModule() Module {
	return &CopyModule{}
}

func (m *CopyModule) Run(hs HostSession, flags *Flags) Result {
	src := flags.Parameter["source"]
	dest := flags.Parameter["dest"]

	if !filepath.IsAbs(dest) {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("目标文件位置必须输入绝对路径，当前输入为:%s", dest),
			Change:  false,
		}
	}

	// 读取源文件内容
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return Result{
			Success: false,
			Output:  "",
			Error:   "获取源文件失败",
			Change:  false,
		}
	}
	srcf, err := os.Stat(srcAbs)

	if err != nil {
		// 处理所有可能的错误（文件不存在、权限不足等）
		if os.IsNotExist(err) {
			return Result{
				Success: false,
				Output:  "",
				Error:   fmt.Sprintf("源文件不存在: %s", srcAbs),
				Change:  false,
			}
		}
		// 其他错误（如权限不足）
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("获取源文件信息失败: %s", err.Error()),
			Change:  false,
		}
	}
	Logger.Sugar().Debugf("源文件名%s", srcf.Name())
	if srcf.IsDir() {
		return Result{
			Success: false,
			Output:  "",
			Error:   fmt.Sprintf("不支持目录复制: %s", srcAbs),
			Change:  false,
		} // 明确提示不支持目录
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
dest_path=%q  # 用户输入的目标路径（可能是文件或目录）
src_filename=%q  # 本地 src 的文件名（用于拼接目录路径）
dest_path=${dest_path%%/}
# 判断 dest 是否为目录，如果是，则目标文件为 dest/src_filename
if [ -d "$dest_path" ]; then
  target_path="$dest_path/$src_filename"  # 目录 -> 拼接文件名
else
  target_path="$dest_path"  # 非目录 -> 直接使用 dest 作为目标文件
fi

# 后续逻辑基于 target_path 执行
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
`, dest, srcf.Name(), srcMd5)
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
	Logger.Sugar().Debugf("远程文件状态返回", destContentTrimmed)
	changed := false

	// 判断是否需要变更（文件不存在或内容不同）
	if destContentTrimmed == "FILE_NOT_FOUND" || destContentTrimmed == "DIFFER" {
		changed = true
		permissionStr := fmt.Sprintf("%03o", srcf.Mode()&os.ModePerm)
		// 创建临时文件并写入内容
		tmpFile := fmt.Sprintf("/tmp/fastdp_copy_%d", time.Now().UnixNano())
		// 给临时文件赋予与目标文件一致的权限
		writeCmd := fmt.Sprintf(`
dest_path=%q
src_filename=%q
dest_path=${dest_path%%/}
if [ -d "$dest_path" ]; then
  target_path="$dest_path/$src_filename"
else
  target_path="$dest_path"
fi

tmpFile=%q
cat > "$tmpFile" && chmod %s "$tmpFile" && mv "$tmpFile" "$target_path"
`, dest, srcf.Name(), tmpFile, permissionStr)

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
		return Result{
			Success: true,
			Output:  fmt.Sprintf("已成功复制 %s 到 %s（内容有更新）", src, dest),
			Error:   "",
			Change:  changed,
		}
	} else if destContentTrimmed == "SAME" {
		return Result{
			Success: true,
			Output:  fmt.Sprintf("文件 %s 与远程 %s 内容一致，无需复制", src, dest),
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
	Register("copy", NewCopyModule) // 注册 "shell" 模块
}
