//go:build solaris || illumos

package host

import "golang.org/x/sys/unix"

// processGroupID：solaris/illumos 的 unix.Getpgrp 返回 (int, error)。
func processGroupID() int {
	pgid, err := unix.Getpgrp()
	if err != nil {
		return 0
	}
	return pgid
}
