# contrib/bashtool

`contrib/bashtool` provides a reusable LLM-facing bash tool contract on top of `vsh`.

It is a public helper module, not a sandbox command bundle:

- not part of `vsh.DefaultRegistry()`
- not registered by `contrib/extras`
- intended for embedders that want a `bash` function/tool definition with vsh-backed execution

## What It Includes

- provider-neutral tool metadata via `ToolDefinition()`
- input and output schemas
- upstream-style `SystemPrompt()` and `Help()` surfaces
- request parsing for `commands` plus the `script` alias
- evaluator-style `FormatToolResult()` rendering
- one-shot `Execute()` backed by a fresh `vsh` runtime
- optional config-level prompt appends via `Config.SystemPromptAppend`

Defaults stay vsh-native:

- tool name: `bash`
- home directory: `/home/agent`
- hostname: `vsh`
- command profile: plain `vsh`

Use `CommandProfileExtras` when you want the stable extras registry (`awk`, `html-to-markdown`, `jq`, `python`, `python3`, `sqlite3`, `yq`) reflected in the prompt/help/execute surface.

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	"github.com/veypi/vsh/contrib/bashtool"
)

func main() {
	tool := bashtool.New(bashtool.Config{
		Profile:            bashtool.CommandProfileExtras,
		SystemPromptAppend: "Always prefer jq for JSON reshaping when available.",
	})

	resp := tool.Execute(context.Background(), bashtool.Request{
		Commands: `printf '{"name":"alice"}' | jq -r '.name'`,
	})
	fmt.Print(resp.Stdout)
}
```

## Relationship To `examples/vsh-eval`

`examples/vsh-eval` uses this module for bash tool metadata, prompt generation, request parsing, and tool-result formatting, while keeping its own persistent session harness for multi-turn filesystem persistence.

## Attribution

This package is a vsh-owned Go port of the upstream `bashkit` Bash tool contract from [`everruns/bashkit`](https://github.com/everruns/bashkit), adapted under Apache-2.0 for vsh-specific defaults and execution plumbing.
