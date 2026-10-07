//go:build !windows && !js && !wasip1 && !plan9

package fs

import "syscall"

// errnoFileTooLarge 是「文件过大」的底层 errno。本仓只承诺 darwin/linux/windows；
// 其余平台保证可编译（见 errno_plan9.go 一类的桩）。
var errnoFileTooLarge = syscall.EFBIG
