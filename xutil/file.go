package xutil

import "os"

// FileExist 文件是否存在
func FileExist(filePath string) bool {
	stat, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return !stat.IsDir()
}

// DirExist 目录是否存在
func DirExist(filePath string) bool {
	stat, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	return stat.IsDir()
}
