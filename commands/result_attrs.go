package commands

import (
	"fmt"
	"sync"
)

// ResultAttrs carries command-produced metadata alongside stdout and stderr.
// One collector is shared by all command invocations in a script execution.
type ResultAttrs struct {
	mu     sync.Mutex
	values map[string]string
}

func NewResultAttrs() *ResultAttrs {
	return &ResultAttrs{values: map[string]string{}}
}

func (a *ResultAttrs) Set(key, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.values == nil {
		a.values = map[string]string{}
	}
	a.values[key] = value
}

// Snapshot returns an independent copy that callers may safely retain or modify.
func (a *ResultAttrs) Snapshot() map[string]string {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.values) == 0 {
		return nil
	}
	out := make(map[string]string, len(a.values))
	for key, value := range a.values {
		out[key] = value
	}
	return out
}

// SetResultAttr attaches metadata to the current script execution result.
func (inv *Invocation) SetResultAttr(key, value string) error {
	if key == "" {
		return fmt.Errorf("result attr key is required")
	}
	if inv == nil || inv.resultAttrs == nil {
		return fmt.Errorf("result attrs are unavailable")
	}
	inv.resultAttrs.Set(key, value)
	return nil
}
