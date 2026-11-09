package vsh

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/veypi/vsh/commands"
	"github.com/veypi/vsh/internal/builtins"
)

func TestCommandResultAttrs(t *testing.T) {
	t.Parallel()
	registry := builtins.DefaultRegistry()
	if err := registry.Register(commands.DefineCommand("attrprobe", func(_ context.Context, inv *commands.Invocation) error {
		return inv.SetResultAttr(inv.Args[0], inv.Args[1])
	})); err != nil {
		t.Fatal(err)
	}
	session := newSession(t, &Config{Registry: registry})
	result := mustExecSession(t, session, "attrprobe first one; attrprobe second two")
	if result.Attrs["first"] != "one" || result.Attrs["second"] != "two" {
		t.Fatalf("attrs = %#v", result.Attrs)
	}
	result.Attrs["first"] = "changed"
	next := mustExecSession(t, session, "attrprobe first fresh")
	if next.Attrs["first"] != "fresh" || next.Attrs["second"] != "" {
		t.Fatalf("attrs leaked across executions: %#v", next.Attrs)
	}
}

func TestResultAttrsConcurrentSetAndSnapshot(t *testing.T) {
	attrs := commands.NewResultAttrs()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attrs.Set(fmt.Sprint(i), "value")
		}()
	}
	wg.Wait()
	snapshot := attrs.Snapshot()
	if len(snapshot) != 32 {
		t.Fatalf("snapshot has %d attrs, want 32", len(snapshot))
	}
	snapshot["0"] = "changed"
	if attrs.Snapshot()["0"] != "value" {
		t.Fatal("snapshot aliases collector storage")
	}
}

func TestSetResultAttrRequiresRuntimeCollector(t *testing.T) {
	inv := &commands.Invocation{}
	if err := inv.SetResultAttr("image_data", "value"); err == nil {
		t.Fatal("manual invocation without a collector accepted result attrs")
	}
}
