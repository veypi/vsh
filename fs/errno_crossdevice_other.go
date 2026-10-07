//go:build !plan9

package fs

import "syscall"

// errnoCrossDevice 是「跨设备 link/rename」的底层 errno。
var errnoCrossDevice = syscall.EXDEV
