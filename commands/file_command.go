package commands

import "context"

// FileCommandRequest identifies an executable file resolved by the virtual
// shell. Path is absolute within the runtime filesystem; InvokedName is the
// command token written by the caller.
type FileCommandRequest struct {
	Path        string
	InvokedName string
}

// FileCommandResolver constructs a regular Command for a resolved executable
// file. Returning (nil, nil) means that no implementation is available.
// Resolution must not execute the file or perform the command's side effects.
type FileCommandResolver interface {
	ResolveFileCommand(ctx context.Context, req FileCommandRequest) (Command, error)
}

// FileCommandResolverFunc adapts a function into a FileCommandResolver.
type FileCommandResolverFunc func(context.Context, FileCommandRequest) (Command, error)

func (f FileCommandResolverFunc) ResolveFileCommand(ctx context.Context, req FileCommandRequest) (Command, error) {
	return f(ctx, req)
}
