//go:build plan9

package fs

import (
	"errors"
	stdfs "io/fs"
	"os"
)

// mkfifoAt：plan9 无 mkfifo 语义（本仓只承诺 darwin/linux/windows；
// 其余平台保证可编译）。见 mkfifo_at_*.go 其余平台实现。
func mkfifoAt(*os.File, string, stdfs.FileMode) error {
	return errors.New("mkfifo is unsupported on plan9")
}
