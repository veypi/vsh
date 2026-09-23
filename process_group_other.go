//go:build !unix

package vsh

import "context"

func withHostProcessGroup(ctx context.Context) context.Context {
	return ctx
}
