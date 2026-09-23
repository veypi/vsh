package main

import (
	"github.com/veypi/vsh"
	"github.com/veypi/vsh/cli"
	"github.com/veypi/vsh/contrib/extras"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = ""
	builtBy = ""
)

func newCLIConfig() cli.Config {
	return cli.Config{
		Name: "vsh-extras",
		Build: &cli.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
			BuiltBy: builtBy,
		},
		BaseOptions: []vsh.Option{
			vsh.WithRegistry(extras.FullRegistry()),
		},
	}
}
