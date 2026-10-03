package builtins

import (
	"context"
	"fmt"
	pubcmd "github.com/veypi/vsh/commands"
	"io"
)

type Which struct{}

func NewWhich() *Which {
	return &Which{}
}

func (c *Which) Name() string {
	return "which"
}

func (c *Which) Run(ctx context.Context, inv *Invocation) error {
	return RunCommand(ctx, c, inv)
}

func (c *Which) Spec() CommandSpec {
	return CommandSpec{
		Name:  "which",
		Usage: "which [-as] NAME...",
		HelpRenderer: func(w io.Writer, _ CommandSpec) error {
			_, err := io.WriteString(w, "usage: which [-as] NAME...\n")
			return err
		},
		Options: []OptionSpec{
			{Name: "all", Short: 'a', Help: "print all matching PATH entries"},
			{Name: "silent", Short: 's', Help: "print nothing, only return status"},
		},
		Args: []ArgSpec{
			{Name: "name", ValueName: "NAME", Repeatable: true},
		},
		Parse: ParseConfig{
			GroupShortOptions: true,
			AutoHelp:          true,
		},
	}
}

func (c *Which) RunParsed(ctx context.Context, inv *Invocation, matches *ParsedCommand) error {
	all := matches.Has("all")
	silent := matches.Has("silent")
	names := matches.Args("name")
	if len(names) == 0 {
		return &ExitError{Code: 1}
	}

	exitCode := 0
	for _, name := range names {
		if inv.LookupCommand == nil {
			return fmt.Errorf("which: command lookup unavailable")
		}
		locations, err := inv.LookupCommand(ctx, pubcmd.CommandLookupRequest{Name: name, Env: inv.Env, WorkDir: inv.Cwd, All: all})
		var paths []string
		for _, loc := range locations {
			if loc.Path != "" {
				paths = append(paths, loc.Path)
			} else {
				paths = append(paths, loc.Name)
			}
		}
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			exitCode = 1
			continue
		}
		if silent {
			continue
		}
		if !all {
			paths = paths[:1]
		}
		for _, path := range paths {
			if _, err := fmt.Fprintln(inv.Stdout, path); err != nil {
				return &ExitError{Code: 1, Err: err}
			}
		}
	}
	if exitCode != 0 {
		return &ExitError{Code: exitCode}
	}
	return nil
}

var _ Command = (*Which)(nil)
var _ SpecProvider = (*Which)(nil)
var _ ParsedRunner = (*Which)(nil)
