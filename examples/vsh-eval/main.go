package main

import (
	"context"
	"fmt"
	"os"

	vsheval "github.com/veypi/vsh/examples/vsh-eval/internal"
)

func main() {
	if err := vsheval.RunCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
