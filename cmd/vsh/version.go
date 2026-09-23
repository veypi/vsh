package main

import (
	"github.com/veypi/vsh/cli"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = ""
	builtBy = ""
)

func newCLIConfig() cli.Config {
	return cli.Config{
		Name: "vsh",
		Build: &cli.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
			BuiltBy: builtBy,
		},
	}
}
