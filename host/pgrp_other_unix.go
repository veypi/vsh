//go:build unix && !solaris && !illumos

package host

import "golang.org/x/sys/unix"

// processGroupID 返回当前进程组。solaris/illumos 上 unix.Getpgrp 返回两个值，
// 见 pgrp_solaris.go（本仓只承诺 darwin/linux/windows，其余平台保证可编译）。
func processGroupID() int { return unix.Getpgrp() }
