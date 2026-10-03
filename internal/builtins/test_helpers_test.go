package builtins_test

import (
	"context"
	"encoding/hex"
	"io"
	"os"
	"path"
	goruntime "runtime"
	"slices"
	"testing"

	gbruntime "github.com/veypi/vsh"
	gbfs "github.com/veypi/vsh/fs"
)

type Config = gbruntime.Config
type Runtime = gbruntime.Runtime
type Session = gbruntime.Session
type ExecutionRequest = gbruntime.ExecutionRequest
type ExecutionResult = gbruntime.ExecutionResult
type NetworkConfig = gbruntime.NetworkConfig
type Method = gbruntime.Method

const (
	MethodGet     = gbruntime.MethodGet
	MethodHead    = gbruntime.MethodHead
	MethodPost    = gbruntime.MethodPost
	MethodPut     = gbruntime.MethodPut
	MethodDelete  = gbruntime.MethodDelete
	MethodPatch   = gbruntime.MethodPatch
	MethodOptions = gbruntime.MethodOptions
)

const (
	defaultHomeDir = "/home/agent"
	defaultPath    = "/usr/bin:/bin"
	defaultUser    = "agent"
	defaultUID     = "1000"
	defaultGID     = "1000"
)

type seededFSFactory struct {
	files map[string]string
}

func CustomFileSystem(factory gbfs.Factory, workingDir string) gbruntime.FileSystemConfig {
	return gbruntime.CustomFileSystem(factory, workingDir)
}

func (f seededFSFactory) New(ctx context.Context) (gbfs.FileSystem, error) {
	mem := gbfs.NewMemory()
	if err := mem.MkdirAll(ctx, defaultHomeDir, 0755); err != nil {
		return nil, err
	}
	for name, contents := range f.files {
		file, err := mem.OpenFile(ctx, name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(file, contents); err != nil {
			_ = file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
	}
	return mem, nil
}

func newRuntime(tb testing.TB, cfg *Config) *Runtime {
	tb.Helper()

	rt, err := gbruntime.New(gbruntime.WithConfig(cfg))
	if err != nil {
		tb.Fatalf("vsh.New() error = %v", err)
	}
	return rt
}

func newSession(tb testing.TB, cfg *Config) *Session {
	tb.Helper()

	session, err := newRuntime(tb, cfg).NewSession(context.Background())
	if err != nil {
		tb.Fatalf("Runtime.NewSession() error = %v", err)
	}
	return session
}

func containsLine(lines []string, want string) bool {
	return slices.Contains(lines, want)
}

func mustExecSession(tb testing.TB, session *Session, script string) *ExecutionResult {
	tb.Helper()

	result, err := session.Exec(context.Background(), &ExecutionRequest{Script: script})
	if err != nil {
		tb.Fatalf("Session.Exec() error = %v", err)
	}
	return result
}

func writeSessionFile(tb testing.TB, session *Session, name string, data []byte) {
	tb.Helper()

	if err := session.FileSystem().MkdirAll(context.Background(), path.Dir(name), 0o755); err != nil {
		tb.Fatalf("MkdirAll(%q) error = %v", path.Dir(name), err)
	}

	file, err := session.FileSystem().OpenFile(context.Background(), name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		tb.Fatalf("OpenFile(%q) error = %v", name, err)
	}
	defer func() { _ = file.Close() }()

	if _, err := file.Write(data); err != nil {
		tb.Fatalf("Write(%q) error = %v", name, err)
	}
}

func readSessionFile(tb testing.TB, session *Session, name string) []byte {
	tb.Helper()

	file, err := session.FileSystem().Open(context.Background(), name)
	if err != nil {
		tb.Fatalf("Open(%q) error = %v", name, err)
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		tb.Fatalf("ReadAll(%q) error = %v", name, err)
	}
	return data
}

func mustHexDecode(tb testing.TB, value string) []byte {
	tb.Helper()

	raw, err := hex.DecodeString(value)
	if err != nil {
		tb.Fatalf("hex.DecodeString(%q): %v", value, err)
	}
	return raw
}

func defaultBaseEnv() map[string]string {
	arch := defaultArchMachine()
	return map[string]string{
		"HOME":                       defaultHomeDir,
		"PATH":                       defaultPath,
		"USER":                       defaultUser,
		"LOGNAME":                    defaultUser,
		"GROUP":                      defaultUser,
		"GROUPS":                     defaultGID,
		"UID":                        defaultUID,
		"EUID":                       defaultUID,
		"GID":                        defaultGID,
		"EGID":                       defaultGID,
		"SHELL":                      "/bin/sh",
		"VSH_ARCH":                   arch,
		"VSH_UNAME_SYSNAME":          defaultUnameKernelName(),
		"VSH_UNAME_NODENAME":         "vsh",
		"VSH_UNAME_RELEASE":          "unknown",
		"VSH_UNAME_VERSION":          "unknown",
		"VSH_UNAME_MACHINE":          arch,
		"VSH_UNAME_OPERATING_SYSTEM": defaultUnameOperatingSystem(),
		"VSH_UMASK":                  "0022",
	}
}

func archMachineFromGOARCH() string {
	switch goruntime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386":
		return "i686"
	case "arm64":
		return "aarch64"
	default:
		return goruntime.GOARCH
	}
}

func defaultUnameKernelName() string {
	switch goruntime.GOOS {
	case "android", "linux":
		return "Linux"
	case "darwin", "ios":
		return "Darwin"
	case "windows":
		return "Windows_NT"
	case "plan9":
		return "Plan 9"
	default:
		return defaultUnameOperatingSystem()
	}
}

func defaultUnameOperatingSystem() string {
	switch goruntime.GOOS {
	case "aix":
		return "AIX"
	case "android":
		return "Android"
	case "darwin":
		return "Darwin"
	case "dragonfly":
		return "DragonFly"
	case "freebsd":
		return "FreeBSD"
	case "fuchsia":
		return "Fuchsia"
	case "illumos":
		return "illumos"
	case "ios":
		return "Darwin"
	case "js":
		return "JavaScript"
	case "linux":
		return "GNU/Linux"
	case "netbsd":
		return "NetBSD"
	case "openbsd":
		return "OpenBSD"
	case "plan9":
		return "Plan 9"
	case "redox":
		return "Redox"
	case "solaris":
		return "SunOS"
	case "windows":
		return "MS/Windows"
	default:
		return goruntime.GOOS
	}
}
