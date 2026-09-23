package vsh

import "github.com/veypi/vsh/shellvariant"

type ShellVariant = shellvariant.ShellVariant

const (
	ShellVariantAuto = shellvariant.Auto
	ShellVariantBash = shellvariant.Bash
	ShellVariantSH   = shellvariant.SH
	ShellVariantMksh = shellvariant.Mksh
	ShellVariantZsh  = shellvariant.Zsh
	ShellVariantBats = shellvariant.Bats
)
