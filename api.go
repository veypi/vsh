package vsh

import (
	"context"
	"fmt"

	"github.com/veypi/vsh/commands"
	"github.com/veypi/vsh/host"
	"github.com/veypi/vsh/internal/builtins"
	"github.com/veypi/vsh/network"
	"github.com/veypi/vsh/policy"
	"github.com/veypi/vsh/shell/analysis"
)

// Method identifies an HTTP method that is allowed by the sandbox network
// policy.
type Method = network.Method

const (
	// MethodGet allows HTTP GET requests.
	MethodGet = network.MethodGet
	// MethodHead allows HTTP HEAD requests.
	MethodHead = network.MethodHead
	// MethodPost allows HTTP POST requests.
	MethodPost = network.MethodPost
	// MethodPut allows HTTP PUT requests.
	MethodPut = network.MethodPut
	// MethodDelete allows HTTP DELETE requests.
	MethodDelete = network.MethodDelete
	// MethodPatch allows HTTP PATCH requests.
	MethodPatch = network.MethodPatch
	// MethodOptions allows HTTP OPTIONS requests.
	MethodOptions = network.MethodOptions
)

// NetworkConfig controls the built-in HTTP client that powers curl inside the
// sandbox.
//
// All fields are optional except that some form of URL allowlist is required at
// runtime. Empty AllowedMethods defaults to GET and HEAD. Zero-valued limits use
// the network package defaults.
type NetworkConfig = network.Config

// Config describes the complete vsh runtime configuration.
//
// The zero value is useful: it creates the default in-memory sandbox rooted at
// /home/agent with the default command registry and the default static policy.
//
// Most callers should prefer [New] with a small number of option helpers such
// as [WithWorkspace], [WithHTTPAccess], [WithRegistry], or [WithBaseEnv]. This
// struct is provided for callers that want to construct a configuration value
// explicitly before handing it to [WithConfig].
type Config struct {
	// FileSystem controls how each session gets its sandbox filesystem and what
	// working directory new sessions start in.
	FileSystem FileSystemConfig

	// Registry contains the commands that can be executed inside the sandbox.
	// When nil, the default built-in registry is used.
	Registry commands.CommandRegistry

	// Policy governs path access, command limits, and other sandbox checks.
	// When nil, the default static policy is used.
	Policy policy.Policy

	// LimitOverrides overrides selected runtime limits without replacing the
	// active policy. Zero-valued fields leave the underlying policy/defaults
	// unchanged.
	LimitOverrides policy.Limits

	// BaseEnv provides the base environment visible to each execution before any
	// per-request environment overrides are applied.
	BaseEnv map[string]string

	// Host controls the host-derived platform behavior, process metadata, and
	// base environment defaults visible to the runtime. When nil, vsh uses its
	// internal virtual host adapter.
	Host host.Adapter

	// Network configures the built-in HTTP client used by the curl command. When
	// nil and NetworkClient is also nil, curl is not registered in the sandbox.
	Network *NetworkConfig

	// NetworkClient replaces the built-in HTTP client. This is the advanced
	// escape hatch for tests and custom transports.
	NetworkClient network.Client

	// Tracing controls structured execution events. Tracing is off by default.
	// When enabled, [ExecutionResult.Events] is populated for non-interactive
	// executions and OnEvent receives events for both non-interactive and
	// interactive runs.
	Tracing TraceConfig

	// Logger receives top-level execution lifecycle events. Logging is off by
	// default.
	Logger LogCallback

	// AnalysisObserver receives read-only shell semantic events from the
	// interpreter. The observer is installed at runtime construction time and
	// applies to both non-interactive and interactive shell execution.
	AnalysisObserver analysis.Observer

	// NativeExec executes an already resolved real file. Nil disables native execution.
	NativeExec func(context.Context, string, *commands.Invocation) error
}

// Option mutates a [Config] before [New] constructs the runtime.
//
// Options are applied in order, so later options can intentionally override
// earlier ones.
type Option func(*Config) error

// New constructs a runtime from the provided options.
//
// When called with no options, New returns the default sandbox runtime:
//
//   - an isolated in-memory filesystem rooted at /home/agent
//   - the built-in command registry
//   - the default in-tree shell core
//   - the default static policy and resource limits
//   - no network access
//
// Use [WithWorkspace] to mount a real host directory, [WithHTTPAccess] or
// [WithNetwork] to enable curl, and [WithRegistry], [WithPolicy], or
// [WithFileSystem] when you need lower-level control.
func New(opts ...Option) (*Runtime, error) {
	cfg, err := resolveConfig(opts)
	if err != nil {
		return nil, err
	}
	cfg.FileSystem = cfg.FileSystem.resolved()
	if cfg.Registry == nil {
		cfg.Registry = builtins.DefaultRegistry()
	}
	if cfg.NetworkClient == nil && cfg.Network != nil {
		client, err := network.New(cfg.Network)
		if err != nil {
			return nil, err
		}
		cfg.NetworkClient = client
	}
	if cfg.NetworkClient != nil {
		if err := builtins.EnsureNetworkCommands(cfg.Registry); err != nil {
			return nil, err
		}
	}
	defaultLimits := mergeLimits(policy.Limits{
		MaxCommandCount:      10000,
		MaxGlobOperations:    100000,
		MaxLoopIterations:    10000,
		MaxSubstitutionDepth: 50,
		MaxStdoutBytes:       1 << 20,
		MaxStderrBytes:       1 << 20,
		MaxFileBytes:         8 << 20,
	}, cfg.LimitOverrides)
	if cfg.Policy == nil {
		cfg.Policy = policy.NewStatic(&policy.Config{
			AllowedCommands: cfg.Registry.Names(),
			ReadRoots:       []string{"/"},
			WriteRoots:      []string{"/"},
			Limits:          defaultLimits,
			SymlinkMode:     policy.SymlinkDeny,
		})
	} else if cfg.LimitOverrides != (policy.Limits{}) {
		cfg.Policy = limitOverridePolicy{
			base:   cfg.Policy,
			limits: mergeLimits(cfg.Policy.Limits(), cfg.LimitOverrides),
		}
	}
	if cfg.Host == nil {
		cfg.Host = newVirtualHost()
	}
	hostEnv, err := runtimeBaseEnv(context.Background(), cfg.Host)
	if err != nil {
		return nil, err
	}
	cfg.BaseEnv = mergeEnv(hostEnv, cfg.BaseEnv)

	return &Runtime{
		cfg:            cfg,
		sessionFactory: cfg.FileSystem.Factory,
	}, nil
}

func resolveConfig(opts []Option) (Config, error) {
	var cfg Config
	for i, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return Config{}, fmt.Errorf("vsh: apply option %d: %w", i, err)
		}
	}
	return cfg, nil
}

// DefaultRegistry returns a registry populated with vsh's built-in commands.
//
// Callers can register additional custom commands onto the returned registry
// before passing it to [WithRegistry].
func DefaultRegistry() *commands.Registry {
	return builtins.DefaultRegistry()
}
