//go:build windows

package vsh

func defaultArchMachine() string {
	return archMachineFromGOARCH()
}
