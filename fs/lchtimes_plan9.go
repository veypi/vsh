//go:build plan9

package fs

import (
	"errors"
	"os"
	"time"
)

// lchtimesAt：plan9 无 lutimes 等价 API（本仓只承诺 darwin/linux/windows；
// 其余平台保证可编译）。见 lchtimes_unix.go。
func lchtimesAt(*os.File, string, time.Time, time.Time) error {
	return errors.New("lchtimes is unsupported on plan9")
}
