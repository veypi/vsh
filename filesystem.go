package vsh

import (
	"context"
	"io/fs"
	"strings"

	gbfs "github.com/veypi/vsh/fs"
)

const (
	// DefaultWorkspaceMountPoint is the default sandbox mount point used by
	// [WithWorkspace] and [HostDirectoryFileSystem].
	DefaultWorkspaceMountPoint = gbfs.DefaultHostVirtualRoot
	// DefaultHostFileReadBytes is the default per-file read cap used when a host
	// directory is mounted into the sandbox.
	DefaultHostFileReadBytes = gbfs.DefaultHostMaxFileReadBytes
)

// FileSystemConfig describes how vsh provisions a session filesystem.
//
// Callers rarely need to populate this struct directly. Prefer the helper
// constructors [InMemoryFileSystem], [SeededInMemoryFileSystem],
// [HostDirectoryFileSystem], [MountableFileSystem],
// [ReadWriteDirectoryFileSystem], and [CustomFileSystem], and then apply the
// result with [WithFileSystem].
type FileSystemConfig struct {
	// Factory builds the filesystem instance for a new session.
	Factory gbfs.Factory

	// WorkingDir is the directory new sessions start in.
	WorkingDir string
}

// MountableFileSystemOptions configures a multi-mount sandbox filesystem.
type MountableFileSystemOptions struct {
	// Base provisions the base filesystem used for unmounted paths. When nil, a
	// fresh in-memory filesystem is used.
	Base gbfs.Factory

	// Mounts configures the mounted filesystems visible inside the sandbox.
	Mounts []gbfs.MountConfig

	// WorkingDir is the directory new sessions start in. When empty,
	// /home/agent is used.
	WorkingDir string
}

// HostDirectoryOptions controls how a real host directory is mounted into the
// sandbox.
//
// The mounted directory is always read-only from the host's perspective. vsh
// layers an in-memory writable upper filesystem on top so the shell can create,
// overwrite, and delete files without mutating the host tree.
type HostDirectoryOptions struct {
	// MountPoint is the sandbox path where the host directory should appear.
	// When empty, [DefaultWorkspaceMountPoint] is used.
	MountPoint string

	// MaxFileReadBytes limits the size of individual regular files that may be
	// read from the host directory. When zero or negative, the default host read
	// cap is used.
	MaxFileReadBytes int64
}

// ReadWriteDirectoryOptions controls how a real host directory is mounted as a
// mutable sandbox root.
//
// Unlike [HostDirectoryOptions], this mode writes directly back to the host
// directory instead of using an in-memory overlay. It is intended for opt-in
// compatibility harnesses and advanced embedding scenarios.
type ReadWriteDirectoryOptions struct {
	// MaxFileReadBytes limits the size of individual regular files that may be
	// read from the host directory. When zero or negative, the default host read
	// cap is used.
	MaxFileReadBytes int64
}

// InMemoryFileSystem returns the default mutable sandbox filesystem
// configuration.
//
// This is the same filesystem layout vsh uses when [New] is called without a
// filesystem option.
func InMemoryFileSystem() FileSystemConfig {
	return FileSystemConfig{
		Factory:    memoryFactory(nil),
		WorkingDir: defaultHomeDir,
	}
}

// SeededInMemoryFileSystem prepares the default home/tmp and provided files.
func SeededInMemoryFileSystem(files gbfs.InitialFiles) FileSystemConfig {
	return FileSystemConfig{
		Factory:    memoryFactory(files),
		WorkingDir: defaultHomeDir,
	}
}

// CustomFileSystem wires an arbitrary filesystem factory into the runtime.
//
// This is the low-level escape hatch for callers that want to seed a custom
// filesystem backend or provide their own persistence model.
func CustomFileSystem(factory gbfs.Factory, workingDir string) FileSystemConfig {
	return FileSystemConfig{
		Factory:    factory,
		WorkingDir: workingDir,
	}
}

// MountableFileSystem returns a multi-mount filesystem configuration.
func MountableFileSystem(opts MountableFileSystemOptions) FileSystemConfig {
	if opts.Base == nil {
		opts.Base = memoryFactory(nil)
	}
	workingDir := strings.TrimSpace(opts.WorkingDir)
	if workingDir == "" {
		workingDir = defaultHomeDir
	}
	return FileSystemConfig{
		Factory: gbfs.Mountable(gbfs.MountableOptions{
			Base:   opts.Base,
			Mounts: append([]gbfs.MountConfig(nil), opts.Mounts...),
		}),
		WorkingDir: workingDir,
	}
}

type hostDirectoryFactory struct {
	root             string
	mountPoint       string
	maxFileReadBytes int64
}

func (f hostDirectoryFactory) New(ctx context.Context) (gbfs.FileSystem, error) {
	return gbfs.Overlay(gbfs.Host(gbfs.HostOptions{
		Root:             f.root,
		VirtualRoot:      f.mountPoint,
		MaxFileReadBytes: f.maxFileReadBytes,
	})).New(ctx)
}

func (f hostDirectoryFactory) WithMaxFileReadBytes(maxBytes int64) gbfs.Factory {
	f.maxFileReadBytes = maxBytes
	return f
}

// HostDirectoryFileSystem mounts a real host directory into the sandbox under a
// writable in-memory overlay.
//
// The mounted host tree is read-only. All writes and deletes
// live in the in-memory upper layer, so shell activity never mutates the host
// directory directly.
func HostDirectoryFileSystem(root string, opts HostDirectoryOptions) FileSystemConfig {
	mountPoint := strings.TrimSpace(opts.MountPoint)
	if mountPoint == "" {
		mountPoint = DefaultWorkspaceMountPoint
	}
	return FileSystemConfig{
		Factory: hostDirectoryFactory{
			root:             root,
			mountPoint:       mountPoint,
			maxFileReadBytes: opts.MaxFileReadBytes,
		},
		WorkingDir: mountPoint,
	}
}

type readWriteDirectoryFactory struct {
	root             string
	maxFileReadBytes int64
}

func (f readWriteDirectoryFactory) New(ctx context.Context) (gbfs.FileSystem, error) {
	return gbfs.ReadWrite(gbfs.ReadWriteOptions{
		Root:             f.root,
		MaxFileReadBytes: f.maxFileReadBytes,
	}).New(ctx)
}

func (f readWriteDirectoryFactory) WithMaxFileReadBytes(maxBytes int64) gbfs.Factory {
	f.maxFileReadBytes = maxBytes
	return f
}

// ReadWriteDirectoryFileSystem mounts a real host directory as the mutable
// sandbox root.
//
// This is the closest vsh equivalent to just-bash's ReadWriteFs: sandbox
// paths are rooted at "/", sessions start at "/", and writes persist directly
// to the host directory.
func ReadWriteDirectoryFileSystem(root string, opts ReadWriteDirectoryOptions) FileSystemConfig {
	return FileSystemConfig{
		Factory: readWriteDirectoryFactory{
			root:             root,
			maxFileReadBytes: opts.MaxFileReadBytes,
		},
		WorkingDir: "/",
	}
}

func (cfg FileSystemConfig) resolved() FileSystemConfig {
	if cfg.Factory == nil {
		cfg.Factory = memoryFactory(nil)
	}
	cfg.WorkingDir = strings.TrimSpace(cfg.WorkingDir)
	if cfg.WorkingDir == "" {
		cfg.WorkingDir = defaultHomeDir
	}
	cfg.WorkingDir = gbfs.Clean(cfg.WorkingDir)
	return cfg
}

type maxFileReadBytesOverrider interface {
	WithMaxFileReadBytes(maxBytes int64) gbfs.Factory
}

func overrideMaxFileReadBytes(cfg FileSystemConfig, maxBytes int64) (FileSystemConfig, bool) {
	if maxBytes <= 0 || cfg.Factory == nil {
		return cfg, false
	}
	overrider, ok := cfg.Factory.(maxFileReadBytesOverrider)
	if !ok {
		return cfg, false
	}
	cfg.Factory = overrider.WithMaxFileReadBytes(maxBytes)
	return cfg, true
}

// The memory filesystem factory owns its initial directories. Sessions never
// mutate the filesystem to reflect command registrations or HOME/PATH changes.
func memoryFactory(files gbfs.InitialFiles) gbfs.Factory {
	return gbfs.FactoryFunc(func(ctx context.Context) (gbfs.FileSystem, error) {
		mem, err := gbfs.SeededMemory(files).New(ctx)
		if err != nil {
			return nil, err
		}
		if err := mem.MkdirAll(ctx, defaultHomeDir, 0755); err != nil {
			return nil, err
		}
		if err := mem.MkdirAll(ctx, defaultTempDir, fs.ModeSticky|0777); err != nil {
			return nil, err
		}
		return mem, nil
	})
}
