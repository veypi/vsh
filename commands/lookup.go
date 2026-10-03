package commands

import "context"

// CommandLocation is either a registered name (Path empty), or a real file.
type CommandLocation struct{ Name, Path string }
type CommandLookupRequest struct {
	Name      string
	Env       map[string]string
	WorkDir   string
	All       bool
	FilesOnly bool
}

// LookupCommandFunc discovers targets without executing them or granting access.
type LookupCommandFunc func(context.Context, CommandLookupRequest) ([]CommandLocation, error)
