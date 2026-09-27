package utils

import (
	"os"
	"regexp"
	"strings"
)

// ParseCheckScript 解析检查脚本，根据 --only 提取指定部分
// 返回要执行的脚本内容
func ParseCheckScript(scriptPath string, only string) (string, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", err
	}
	script := string(data)

	// 未指定 --only，返回整个脚本
	if only == "" {
		return script, nil
	}

	// 解析 --only 指定的 key
	onlySet := make(map[string]bool)
	for _, field := range strings.Split(only, ",") {
		onlySet[strings.TrimSpace(field)] = true
	}

	lines := strings.Split(script, "\n")
	var result []string
	blockRegex := regexp.MustCompile(`^#\s*BEGIN\s+(\S+)`)
	endRegex := regexp.MustCompile(`^#\s*END\s+(\S+)`)

	inBlock := false
	blockKey := ""
	includeBlock := false

	for _, line := range lines {
		// 检查 BEGIN 标记
		if m := blockRegex.FindStringSubmatch(line); m != nil {
			inBlock = true
			blockKey = m[1]
			includeBlock = onlySet[blockKey]
			continue
		}
		// 检查 END 标记
		if endRegex.MatchString(line) {
			inBlock = false
			continue
		}
		// 在块内且需要包含
		if inBlock {
			if includeBlock {
				result = append(result, line)
			}
			continue
		}
		// 不在块内：检查是否是单行 echo "key=value"
		if strings.Contains(line, "echo ") && strings.Contains(line, "=") {
			key := ExtractKeyFromEcho(line)
			if key != "" && onlySet[key] {
				result = append(result, line)
			}
			continue
		}
		// 其他行（注释、空行、非 echo 行）跳过
	}

	return strings.Join(result, "\n"), nil
}

// ExtractKeyFromEcho 从 echo "key=value" 中提取 key
func ExtractKeyFromEcho(line string) string {
	// 匹配 echo "key=value" 或 echo "key=$(...)"
	echoRegex := regexp.MustCompile(`echo\s+"([^=]+)=`)
	m := echoRegex.FindStringSubmatch(line)
	if m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// Field 巡检字段定义（key + 中文显示名）
type Field struct {
	Key  string
	Name string
}

// fieldAnnotationRegex 匹配 "# FASTDP_FIELD: key=中文名" 注解
var fieldAnnotationRegex = regexp.MustCompile(`^#\s*FASTDP_FIELD:\s*(\S+?)\s*=\s*(.+)$`)

// ParseCheckFields 从巡检脚本解析字段注解（# FASTDP_FIELD: key=中文名）
// 返回按注解出现顺序排序的字段列表。无注解的 key 不在此列表，归为自定义字段。
func ParseCheckFields(scriptPath string) ([]Field, error) {
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, err
	}

	var fields []Field
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		m := fieldAnnotationRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key := strings.TrimSpace(m[1])
		name := strings.TrimSpace(m[2])
		if key == "" || seen[key] {
			continue // 忽略空 key 或重复注解（首个生效）
		}
		seen[key] = true
		fields = append(fields, Field{Key: key, Name: name})
	}
	return fields, nil
}
