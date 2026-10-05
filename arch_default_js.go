//go:build js

package vsh

func defaultArchMachine() string {
	return archMachineFromGOARCH()
}
