//go:build plan9

package fs

import "errors"

// plan9 的 syscall 没有 EXDEV，用等价哨兵顶替（本仓只承诺 darwin/linux/windows；
// 其余平台保证可编译）。见 errno_crossdevice_other.go。
var errnoCrossDevice = errors.New("invalid cross-device link")
