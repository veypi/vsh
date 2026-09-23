package builtins

import (
	"context"
	"io"
)

type True struct{}
type False struct{}

func NewTrue() *True {
	return &True{}
}

func NewFalse() *False {
	return &False{}
}

func (c *True) Name() string {
	return "true"
}

func (c *False) Name() string {
	return "false"
}

func (c *True) Run(_ context.Context, inv *Invocation) error {
	// vsh fork (D7): native --help answer.
	if inv != nil && len(inv.Args) == 1 && inv.Args[0] == "--help" {
		_, err := io.WriteString(inv.Stdout, "true: true [arg ...]\n    Do nothing, successfully (exit status 0).\n")
		return err
	}
	return nil
}

func (c *False) Run(_ context.Context, inv *Invocation) error {
	// vsh fork (D7): native --help answer.
	if inv != nil && len(inv.Args) == 1 && inv.Args[0] == "--help" {
		_, err := io.WriteString(inv.Stdout, "false: false [arg ...]\n    Do nothing, unsuccessfully (exit status 1).\n")
		return err
	}
	return &ExitError{Code: 1}
}

var _ Command = (*True)(nil)
var _ Command = (*False)(nil)
