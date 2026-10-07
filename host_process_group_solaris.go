//go:build solaris || illumos

package vsh

import "golang.org/x/sys/unix"

// currentVirtualProcessGroup：solaris/illumos 的 unix.Getpgrp 返回 (int, error)，
// 见 host_process_group_unix.go（本仓只承诺 darwin/linux/windows，其余平台保证可编译）。
func currentVirtualProcessGroup() int {
	pgid, err := unix.Getpgrp()
	if err != nil {
		return 0
	}
	return pgid
}
