package builtins

import "fmt"

func runNotImplemented(inv *Invocation, name string) error {
	// vsh fork (D7): even unimplemented commands answer --help natively.
	if inv != nil && len(inv.Args) == 1 && inv.Args[0] == "--help" {
		_, err := fmt.Fprintf(inv.Stdout, "Usage: %s [OPTION]... [ARG]...\n\n(note: %s is not implemented in vsh yet)\n", name, name)
		return err
	}
	return Exitf(inv, 1, "%s: not implemented", name)
}
