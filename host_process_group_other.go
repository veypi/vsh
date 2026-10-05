//go:build !unix

package vsh

func currentVirtualProcessGroup() int {
	return 0
}
