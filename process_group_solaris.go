//go:build solaris || illumos

package vsh

import (
	"context"

	"github.com/veypi/vsh/internal/shellstate"
	"golang.org/x/sys/unix"
)

// withHostProcessGroup：solaris/illumos 的 unix.Getpgrp 返回 (int, error)，
// 见 process_group_unix.go（本仓只承诺 darwin/linux/windows，其余平台保证可编译）。
func withHostProcessGroup(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	pgid, err := unix.Getpgrp()
	if err != nil {
		return shellstate.WithProcessGroup(ctx, 0)
	}
	return shellstate.WithProcessGroup(ctx, pgid)
}
