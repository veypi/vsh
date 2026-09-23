// Package extras registers the stable opt-in contrib commands.
package extras

import (
	"fmt"

	"github.com/veypi/vsh"
	"github.com/veypi/vsh/commands"
	contribawk "github.com/veypi/vsh/contrib/awk"
	contribhtmltomarkdown "github.com/veypi/vsh/contrib/htmltomarkdown"
	contribjq "github.com/veypi/vsh/contrib/jq"
	contribpython "github.com/veypi/vsh/contrib/python"
	contribsqlite3 "github.com/veypi/vsh/contrib/sqlite3"
	contribyq "github.com/veypi/vsh/contrib/yq"
)

// FullRegistry returns the default registry plus the stable contrib commands.
func FullRegistry() *commands.Registry {
	registry := vsh.DefaultRegistry()
	if err := Register(registry); err != nil {
		panic(fmt.Sprintf("extras: register full registry: %v", err))
	}
	return registry
}

// Register adds every stable contrib command module to the registry.
func Register(registry commands.CommandRegistry) error {
	if registry == nil {
		return nil
	}
	if err := contribawk.Register(registry); err != nil {
		return err
	}
	if err := contribhtmltomarkdown.Register(registry); err != nil {
		return err
	}
	if err := contribjq.Register(registry); err != nil {
		return err
	}
	if err := contribpython.Register(registry); err != nil {
		return err
	}
	if err := contribsqlite3.Register(registry); err != nil {
		return err
	}
	return contribyq.Register(registry)
}
