# Command Entrypoints

`cmd/` contains the Go command entrypoints that ship from the root `vsh`
module.

- `cmd/vsh` is the main sandbox CLI for local execution and embedding demos.
- `cmd/vsh-gnu` is the compatibility harness used to summarize GNU
  coreutils test results against `vsh`.

Install the main CLI with:

```bash
go install github.com/veypi/vsh/cmd/vsh@latest
```
