package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/veypi/vsh"
	vshtrace "github.com/veypi/vsh/trace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	stdoutlog "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	stdouttrace "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	serviceName      = "vsh-example-otel"
	instrumentation  = "github.com/veypi/vsh/examples/otel"
	demoFile         = "/tmp/otel-session-demo.txt"
	demoFileContents = "hello from vsh session"
	firstExecName    = "seed-file"
	secondExecName   = "read-file"
)

func main() {
	if err := run(context.Background(), os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, telemetryOut, statusOut io.Writer) (err error) {
	if telemetryOut == nil {
		telemetryOut = io.Discard
	}
	if statusOut == nil {
		statusOut = io.Discard
	}

	bridge, err := newTelemetryBridge(ctx, telemetryOut)
	if err != nil {
		return fmt.Errorf("create telemetry bridge: %w", err)
	}
	defer func() { //nolint:contextcheck // shutdown needs independent context after parent may be canceled
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = errors.Join(err, bridge.Shutdown(shutdownCtx))
	}()

	//nolint:contextcheck // constructor does not accept context
	runtime, err := vsh.New(
		vsh.WithTracing(vsh.TraceConfig{
			Mode: vsh.TraceRedacted,
			OnEvent: func(ctx context.Context, event vshtrace.Event) {
				bridge.onTraceEvent(&event)
			},
		}),
		vsh.WithLogger(func(ctx context.Context, event vsh.LogEvent) {
			bridge.onLogEvent(ctx, &event)
		}),
	)
	if err != nil {
		return fmt.Errorf("create runtime: %w", err)
	}

	session, err := runtime.NewSession(ctx)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	if err := runExecution(ctx, session, statusOut, firstExecName, fmt.Sprintf(
		"printf '%s\\n' > %s\ncat %s\n",
		demoFileContents,
		demoFile,
		demoFile,
	)); err != nil {
		return err
	}

	if err := runExecution(ctx, session, statusOut, secondExecName, fmt.Sprintf("cat %s\n", demoFile)); err != nil {
		return err
	}

	return nil
}

func runExecution(ctx context.Context, session *vsh.Session, statusOut io.Writer, name, script string) error {
	result, err := session.Exec(ctx, &vsh.ExecutionRequest{
		Name:   name,
		Script: script,
	})
	if err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s exited with status %d", name, result.ExitCode)
	}

	if statusOut != nil {
		if _, err := fmt.Fprintf(statusOut, "%s exit=%d stdout=%q\n", name, result.ExitCode, strings.TrimSpace(result.Stdout)); err != nil {
			return fmt.Errorf("write status output: %w", err)
		}
	}

	return nil
}

type telemetryBridge struct {
	loggerProvider *sdklog.LoggerProvider
	traceProvider  *sdktrace.TracerProvider
	logger         otellog.Logger
	tracer         oteltrace.Tracer

	mu         sync.Mutex
	executions map[string]*executionState
}

type executionState struct {
	ctx       context.Context //nolint:containedctx // intentional: execution-scoped context for span
	span      oteltrace.Span
	startedAt time.Time
}

func newTelemetryBridge(ctx context.Context, out io.Writer) (*telemetryBridge, error) {
	writer := &lockedWriter{out: out}

	resource, err := sdkresource.New(ctx,
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithAttributes(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	traceExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithWriter(writer),
	)
	if err != nil {
		return nil, fmt.Errorf("create stdout trace exporter: %w", err)
	}

	logExporter, err := stdoutlog.New(
		stdoutlog.WithPrettyPrint(),
		stdoutlog.WithWriter(writer),
	)
	if err != nil {
		return nil, fmt.Errorf("create stdout log exporter: %w", err)
	}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(traceExporter),
	)
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(resource),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExporter)),
	)

	return &telemetryBridge{
		loggerProvider: loggerProvider,
		traceProvider:  traceProvider,
		logger:         loggerProvider.Logger(instrumentation),
		tracer:         traceProvider.Tracer(instrumentation),
		executions:     make(map[string]*executionState),
	}, nil
}

func (b *telemetryBridge) Shutdown(ctx context.Context) error {
	if b == nil {
		return nil
	}
	return errors.Join(
		b.loggerProvider.Shutdown(ctx),
		b.traceProvider.Shutdown(ctx),
	)
}

func (b *telemetryBridge) onLogEvent(ctx context.Context, event *vsh.LogEvent) {
	if event == nil {
		return
	}
	timestamp := time.Now().UTC()

	if event.Kind == vsh.LogExecStart {
		b.startExecution(event, timestamp)
	}

	state := b.lookupExecution(event.ExecutionID)
	logCtx := ctx //nolint:contextcheck // selecting execution context or fallback
	if state != nil {
		logCtx = state.ctx
	}

	var record otellog.Record
	record.SetEventName(string(event.Kind))
	record.SetTimestamp(timestamp)
	record.SetObservedTimestamp(timestamp)
	record.SetSeverity(severityForLogEvent(event))
	record.SetSeverityText(severityForLogEvent(event).String())
	record.SetBody(otellog.StringValue(logBody(event)))
	record.AddAttributes(logKeyValues(logAttributes(event))...)

	b.logger.Emit(logCtx, record)

	switch event.Kind {
	case vsh.LogExecFinish:
		b.finishExecution(event, timestamp, false)
	case vsh.LogExecError:
		b.finishExecution(event, timestamp, true)
	}
}

func (b *telemetryBridge) onTraceEvent(event *vshtrace.Event) {
	if event == nil {
		return
	}
	state := b.lookupExecution(event.ExecutionID)
	if state == nil {
		state = b.ensureExecution(event.SessionID, event.ExecutionID, event.ExecutionID, "", time.Now().UTC())
	}
	if state == nil {
		return
	}

	at := event.At
	if at.IsZero() {
		at = time.Now().UTC()
	}

	state.span.AddEvent(
		string(event.Kind),
		oteltrace.WithTimestamp(at),
		oteltrace.WithAttributes(traceAttributes(event)...),
	)
}

func (b *telemetryBridge) startExecution(event *vsh.LogEvent, now time.Time) {
	b.ensureExecution(event.SessionID, event.ExecutionID, event.Name, event.WorkDir, now)
}

func (b *telemetryBridge) finishExecution(event *vsh.LogEvent, now time.Time, markError bool) {
	state := b.lookupExecution(event.ExecutionID)
	if state == nil {
		state = b.ensureExecution(event.SessionID, event.ExecutionID, event.Name, event.WorkDir, now)
	}
	if state == nil {
		return
	}

	endAt := now
	if event.Duration > 0 && !state.startedAt.IsZero() {
		endAt = state.startedAt.Add(event.Duration)
	}

	state.span.SetAttributes(
		attribute.Int("exit_code", event.ExitCode),
		attribute.Int64("vsh.exec.duration_ms", event.Duration.Milliseconds()),
		attribute.Bool("shell_exited", event.ShellExited),
	)
	switch {
	case markError:
		if event.Error != "" {
			state.span.RecordError(errors.New(event.Error), oteltrace.WithTimestamp(endAt))
			state.span.SetStatus(codes.Error, event.Error)
		} else {
			state.span.SetStatus(codes.Error, "execution failed")
		}
	case event.ExitCode == 0:
		state.span.SetStatus(codes.Ok, "")
	default:
		state.span.SetStatus(codes.Error, fmt.Sprintf("exit code %d", event.ExitCode))
	}
	state.span.End(oteltrace.WithTimestamp(endAt))
	b.deleteExecution(event.ExecutionID)
}

func (b *telemetryBridge) ensureExecution(sessionID, executionID, name, workDir string, now time.Time) *executionState {
	if executionID == "" {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if state := b.executions[executionID]; state != nil {
		if name != "" {
			state.span.SetName(spanName(name))
			state.span.SetAttributes(attribute.String("vsh.exec.name", name))
		}
		if workDir != "" {
			state.span.SetAttributes(attribute.String("workdir", workDir))
		}
		return state
	}

	ctx, span := b.tracer.Start(
		context.Background(),
		spanName(name),
		oteltrace.WithNewRoot(),
		oteltrace.WithTimestamp(now),
		oteltrace.WithAttributes(
			attribute.String("session.id", sessionID),
			attribute.String("execution.id", executionID),
			attribute.String("vsh.exec.name", name),
			attribute.String("workdir", workDir),
		),
	)

	state := &executionState{
		ctx:       ctx,
		span:      span,
		startedAt: now,
	}
	b.executions[executionID] = state
	return state
}

func (b *telemetryBridge) lookupExecution(executionID string) *executionState {
	if executionID == "" {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.executions[executionID]
}

func (b *telemetryBridge) deleteExecution(executionID string) {
	if executionID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.executions, executionID)
}

func spanName(name string) string {
	if name == "" {
		return "vsh.exec.stdin"
	}
	return "vsh.exec." + name
}

func logBody(event *vsh.LogEvent) string {
	switch event.Kind {
	case vsh.LogStdout, vsh.LogStderr:
		return event.Output
	case vsh.LogExecError:
		if event.Error != "" {
			return event.Error
		}
		return "execution failed"
	case vsh.LogExecFinish:
		return fmt.Sprintf("execution finished with exit code %d", event.ExitCode)
	default:
		return "execution started"
	}
}

func severityForLogEvent(event *vsh.LogEvent) otellog.Severity {
	switch event.Kind {
	case vsh.LogStderr:
		return otellog.SeverityWarn
	case vsh.LogExecError:
		return otellog.SeverityError
	default:
		return otellog.SeverityInfo
	}
}

func logAttributes(event *vsh.LogEvent) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("session.id", event.SessionID),
		attribute.String("execution.id", event.ExecutionID),
		attribute.String("vsh.log.kind", string(event.Kind)),
		attribute.String("vsh.exec.name", event.Name),
		attribute.String("workdir", event.WorkDir),
		attribute.Int("exit_code", event.ExitCode),
		attribute.Bool("truncated", event.Truncated),
		attribute.Bool("shell_exited", event.ShellExited),
	}
	if event.Error != "" {
		attrs = append(attrs, attribute.String("error", event.Error))
	}
	return attrs
}

func traceAttributes(event *vshtrace.Event) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("vsh.trace.schema", event.Schema),
		attribute.String("session.id", event.SessionID),
		attribute.String("execution.id", event.ExecutionID),
		attribute.String("vsh.trace.kind", string(event.Kind)),
		attribute.Bool("vsh.trace.redacted", event.Redacted),
	}
	if event.Message != "" {
		attrs = append(attrs, attribute.String("message", event.Message))
	}
	if event.Error != "" {
		attrs = append(attrs, attribute.String("error", event.Error))
	}
	if event.Command != nil {
		attrs = append(attrs, commandAttributes(event.Command)...)
	}
	if event.File != nil {
		attrs = append(attrs, fileAttributes(event.File)...)
	}
	if event.Policy != nil {
		attrs = append(attrs, policyAttributes(event.Policy)...)
	}
	return attrs
}

func commandAttributes(event *vshtrace.CommandEvent) []attribute.KeyValue {
	if event == nil {
		return nil
	}
	attrs := []attribute.KeyValue{
		attribute.String("command.name", event.Name),
		attribute.String("command.dir", event.Dir),
		attribute.Int("command.exit_code", event.ExitCode),
		attribute.Bool("command.builtin", event.Builtin),
		attribute.String("command.position", event.Position),
		attribute.Int64("command.duration_ms", event.Duration.Milliseconds()),
		attribute.String("command.resolved_name", event.ResolvedName),
		attribute.String("command.resolved_path", event.ResolvedPath),
		attribute.String("command.resolution_source", event.ResolutionSource),
	}
	if len(event.Argv) > 0 {
		attrs = append(attrs, attribute.StringSlice("command.argv", event.Argv))
	}
	return attrs
}

func fileAttributes(event *vshtrace.FileEvent) []attribute.KeyValue {
	if event == nil {
		return nil
	}
	return []attribute.KeyValue{
		attribute.String("file.action", event.Action),
		attribute.String("file.path", event.Path),
		attribute.String("file.from_path", event.FromPath),
		attribute.String("file.to_path", event.ToPath),
	}
}

func policyAttributes(event *vshtrace.PolicyEvent) []attribute.KeyValue {
	if event == nil {
		return nil
	}
	return []attribute.KeyValue{
		attribute.String("policy.subject", event.Subject),
		attribute.String("policy.reason", event.Reason),
		attribute.String("policy.action", event.Action),
		attribute.String("policy.path", event.Path),
		attribute.String("policy.command", event.Command),
		attribute.Int("policy.exit_code", event.ExitCode),
		attribute.String("policy.resolution_source", event.ResolutionSource),
	}
}

func logKeyValues(attrs []attribute.KeyValue) []otellog.KeyValue {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]otellog.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		out = append(out, otellog.KeyValueFromAttribute(attr))
	}
	return out
}

type lockedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}
