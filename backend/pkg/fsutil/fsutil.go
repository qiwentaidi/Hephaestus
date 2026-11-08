// Package fsutil 提供跨模块复用的文件系统小工具。
package fsutil

import (
	"io"
	"os"
)

// CopyFile 把 src 的内容复制到 dst，覆盖已有文件，不保留源文件权限位。
// 调用方需要可执行权限时请自行 Chmod。
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
