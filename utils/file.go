package utils

import "os"

// IsTextFile 判断是否为纯文本文件（非二进制）
func IsTextFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	// 读取前 1024 字节判断
	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil || n == 0 {
		return false, err
	}

	// 检查是否有不可打印字符（二进制特征）
	for _, b := range buf[:n] {
		// 0x00-0x08, 0x0B, 0x0E-0x1F 属于控制字符，文本文件不会出现
		if (b >= 0x00 && b <= 0x08) ||
			(b == 0x0B) ||
			(b >= 0x0E && b <= 0x1F) {
			return false, nil
		}
	}

	return true, nil
}
