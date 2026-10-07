//go:build plan9

package fs

import "errors"

// plan9 的 syscall 没有 EFBIG，用等价哨兵顶替（本仓只承诺 darwin/linux/windows；
// 其余平台只保证可编译）。见 errno_posix.go。
var errnoFileTooLarge = errors.New("file too large")
