//go:build js || wasip1

package fs

func OwnershipFromSys(_ any) (FileOwnership, bool) {
	return FileOwnership{}, false
}
