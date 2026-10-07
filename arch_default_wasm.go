//go:build js || wasip1

package vsh

func defaultArchMachine() string {
	return archMachineFromGOARCH()
}
