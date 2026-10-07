//go:build unix

package main

import "syscall"

// errTextBusy 是 ETXTBSY（程序被写打开时 exec 失败）。非 unix 平台见
// etxtbsy_other.go（本仓只承诺 darwin/linux/windows，其余平台保证可编译）。
var errTextBusy = syscall.ETXTBSY
