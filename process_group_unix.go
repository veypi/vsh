//go:build unix

package vsh

import (
	"context"

	"github.com/veypi/vsh/internal/shellstate"
	"golang.org/x/sys/unix"
)

func withHostProcessGroup(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return shellstate.WithProcessGroup(ctx, unix.Getpgrp())
}
