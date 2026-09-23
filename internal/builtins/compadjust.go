package builtins

import (
	"context"
	"io"

	"github.com/veypi/vsh/internal/completionutil"
)

type Compadjust struct{}

func NewCompadjust() *Compadjust {
	return &Compadjust{}
}

func (c *Compadjust) Name() string {
	return "compadjust"
}

func (c *Compadjust) Run(ctx context.Context, inv *Invocation) error {
	// vsh fork (D7): native --help answer.
	if len(inv.Args) == 1 && inv.Args[0] == "--help" {
		_, err := io.WriteString(inv.Stdout, "compadjust: compadjust [name ...]\n    Adjust completion specifications for the given names.\n")
		return err
	}
	if err := completionutil.ApplyCompadjust(completionBackend(ctx, inv), inv.Args); err != nil {
		return exitf(inv, 2, err.Error())
	}
	return nil
}

var _ Command = (*Compadjust)(nil)
