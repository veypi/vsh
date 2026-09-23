// Package codingtools exposes reusable read, edit, and write tool contracts for
// LLM integrations on top of vsh-owned filesystem abstractions.
//
// It ports the upstream pi-mono coding-agent tool semantics onto
// github.com/veypi/vsh/fs.FileSystem so embedders can run the same tool
// surface against in-memory, overlay, session, and host-backed filesystems
// without changing the tool contract.
package codingtools
