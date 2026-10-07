//go:build !windows && !js && !wasip1 && !plan9

package fs

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// lchtimesAt 设置符号链接自身的时间戳（不跟随）。plan9 无对应 API，
// 见 lchtimes_plan9.go（本仓只承诺 darwin/linux/windows，其余平台保证可编译）。
func lchtimesAt(parent *os.File, base string, atime, mtime time.Time) error {
	times := []unix.Timespec{
		unix.NsecToTimespec(atime.UnixNano()),
		unix.NsecToTimespec(mtime.UnixNano()),
	}
	return unix.UtimesNanoAt(int(parent.Fd()), base, times, unix.AT_SYMLINK_NOFOLLOW)
}
