//go:build js || wasip1

package builtins

func catHostAppendMode(file catHostHandle) bool {
	// The browser/wasm (js, wasip1) targets do not expose host fd flags, so treat
	// redirected handles as non-append.
	return false
}
