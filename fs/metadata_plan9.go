//go:build plan9

package fs

// plan9 没有 syscall.Stat_t，所有权信息不可得（本仓只承诺 darwin/linux/windows；
// 其余平台保证可编译）。见 metadata_posix.go。
func OwnershipFromSys(any) (FileOwnership, bool) {
	return FileOwnership{}, false
}
