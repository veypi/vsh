package vsh

import (
	"context"
	"time"

	"github.com/veypi/vsh/trace"
)

// TraceMode controls whether vsh records structured execution events.
type TraceMode uint8

const (
	// TraceOff disables structured execution events.
	TraceOff TraceMode = iota
	// TraceRedacted enables structured events with argv redaction for
	// secret-bearing values. This is the recommended mode for agent workloads.
	TraceRedacted
	// TraceRaw enables full structured events without argv redaction.
	//
	// This mode is unsafe for shared logs or centralized telemetry unless the
	// embedder controls retention and sink access tightly.
	TraceRaw
)

// TraceConfig configures structured execution tracing.
//
// When Mode is [TraceOff], [ExecutionResult.Events] is empty and OnEvent is not
// called. Interactive executions only deliver events to OnEvent; they do not
// return events in an [InteractiveResult].
type TraceConfig struct {
	Mode    TraceMode
	OnEvent func(context.Context, trace.Event)
}

// LogKind identifies a high-level execution lifecycle log event.
type LogKind string

const (
	// LogExecStart fires before the shell engine begins execution.
	LogExecStart LogKind = "exec.start"
	// LogStdout carries the final captured stdout for one execution.
	LogStdout LogKind = "stdout"
	// LogStderr carries the final captured stderr for one execution.
	LogStderr LogKind = "stderr"
	// LogExecFinish fires when execution completes normally, including shell
	// exit statuses and timeout/cancel control outcomes.
	LogExecFinish LogKind = "exec.finish"
	// LogExecError fires when vsh returns an unexpected runtime error rather
	// than a normal shell exit status.
	LogExecError LogKind = "exec.error"
)

// LogEvent describes one top-level execution lifecycle log callback.
type LogEvent struct {
	Kind        LogKind
	SessionID   string
	ExecutionID string
	Name        string
	WorkDir     string
	ExitCode    int
	Duration    time.Duration
	Output      string
	Truncated   bool
	ShellExited bool
	Error       string
}

// LogCallback receives top-level execution lifecycle logs.
//
// Callbacks run synchronously on the execution path. Panics are recovered and
// ignored so observability hooks do not fail shell execution.
type LogCallback func(context.Context, LogEvent)
