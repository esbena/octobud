package copilotruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
	"github.com/stretchr/testify/require"
)

func testRequest(called *atomic.Bool) RuntimeRequest {
	return RuntimeRequest{
		Instructions: "Use only the supplied application tool.",
		Prompt:       "Synthetic evidence.",
		Tools: func(context.Context, context.CancelCauseFunc) []copilot.Tool {
			return []copilot.Tool{{
				Name: "submit_result", Description: "Submit an application result.",
				Parameters:     map[string]any{"type": "object"},
				SkipPermission: true, IsTerminal: true,
				Handler: func(copilot.ToolInvocation) (copilot.ToolResult, error) {
					called.Store(true)
					return copilot.ToolResult{TextResultForLLM: "Accepted"}, nil
				},
			}}
		},
	}
}

type runtimeRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type runtimeCapture struct {
	Env      []string
	Dir      string
	PID      int
	Requests []runtimeRequest
}

func fakeCLI(t *testing.T, mode string) (path, capture string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell launcher requires POSIX")
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	root := t.TempDir()
	path, capture = filepath.Join(root, "copilot"), filepath.Join(root, "capture.json")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nOCTOBUD_TEST_RUNTIME=" + quote(mode) +
		" OCTOBUD_TEST_CAPTURE=" + quote(capture) + " exec " + quote(executable) +
		" -test.run=^TestRuntimeHelper$ -- \"$@\"\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0700))
	return path, capture
}

func readCapture(t *testing.T, path string) runtimeCapture {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var capture runtimeCapture
	require.NoError(t, json.Unmarshal(data, &capture))
	return capture
}

func TestRuntimeRunsApplicationTool(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "must-not-inherit")
	path, captured := fakeCLI(t, "success")
	for _, model := range []string{"", "explicit-model"} {
		var called atomic.Bool
		input := testRequest(&called)
		input.Model = model
		require.NoError(t, run(t.Context(), input, path))
		require.True(t, called.Load())
		capture := readCapture(t, captured)
		env := map[string]string{}
		for _, entry := range capture.Env {
			key, value, _ := strings.Cut(entry, "=")
			env[key] = value
		}
		require.NotContains(t, env, "GITHUB_TOKEN")
		root := filepath.Dir(capture.Dir)
		require.Equal(t, filepath.Join(root, "work"), capture.Dir)
		_, err := os.Stat(root)
		require.ErrorIs(t, err, os.ErrNotExist)
		var creates, sends int
		for _, request := range capture.Requests {
			switch request.Method {
			case "session.create":
				creates++
				var config map[string]any
				require.NoError(t, json.Unmarshal(request.Params, &config))
				require.Equal(t, []any{"submit_result"}, config["availableTools"])
				require.Equal(
					t,
					map[string]any{"mode": "replace", "content": input.Instructions},
					config["systemMessage"],
				)
				if model == "" {
					require.NotContains(t, config, "model", "preserve the SDK default")
				} else {
					require.Equal(t, model, config["model"])
				}
			case "session.send":
				sends++
				var sent struct{ Prompt string }
				require.NoError(t, json.Unmarshal(request.Params, &sent))
				require.Equal(t, input.Prompt, sent.Prompt)
			}
		}
		require.Equal(t, 1, creates)
		require.Equal(t, 1, sends)
	}
}

func TestRuntimeAvailability(t *testing.T) {
	require.ErrorContains(
		t,
		Run(t.Context(), RuntimeRequest{}),
		"requires application tools",
	)
}

func TestRunUsesSDKRuntimePath(t *testing.T) {
	path, _ := fakeCLI(t, "success")
	t.Setenv("COPILOT_CLI_PATH", path)
	var called atomic.Bool
	require.NoError(t, Run(t.Context(), testRequest(&called)))
	require.True(t, called.Load())
}

func TestRuntimeCanFindGitHubCLI(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0700))
	t.Setenv("PATH", dir)
	path, captured := fakeCLI(t, "success")
	require.NoError(t, run(t.Context(), testRequest(&atomic.Bool{}), path))
	capture := readCapture(t, captured)
	for _, entry := range capture.Env {
		if value, ok := strings.CutPrefix(entry, "PATH="); ok {
			require.Contains(t, filepath.SplitList(value), dir)
			return
		}
	}
	t.Fatal("runtime PATH missing")
}

func TestRuntimeReportsStartupFailure(t *testing.T) {
	t.Setenv("COPILOT_CLI_PATH", filepath.Join(t.TempDir(), "missing-runtime"))
	err := Run(t.Context(), testRequest(&atomic.Bool{}))
	require.ErrorIs(t, err, ErrUnavailable)
	require.ErrorContains(t, err, "missing-runtime")
}

func TestRuntimeFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		mode string
		want error
	}{
		{"create-error", ErrUnavailable},
		{"cleanup-error", ErrCleanup}, {"send-error", ErrSession},
		{"tool", ErrRestricted}, {"tool-request", ErrRestricted}, {"external-tool", ErrRestricted},
		{"session-error", ErrSession}, {"overflow", ErrLimit}, {"cumulative-overflow", ErrLimit},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			path, captured := fakeCLI(t, tc.mode)
			var called atomic.Bool
			err := run(t.Context(), testRequest(&called), path)
			require.ErrorIs(t, err, tc.want)
			capture := readCapture(t, captured)
			_, err = os.Stat(capture.Dir)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestRuntimeLimits(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		extra     int
		unbounded bool
		want      error
	}{
		{"exact-boundaries", "output-boundary", 0, false, nil},
		{"input-overflow", "success", 1, false, ErrLimit},
		{"unbounded", "overflow", 1, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, _ := fakeCLI(t, tc.mode)
			var called atomic.Bool
			input := testRequest(&called)
			input.Prompt = strings.Repeat("x", maxInputBytes-len(input.Instructions)+tc.extra)
			input.Unbounded = tc.unbounded
			factory := input.Tools
			input.Tools = func(ctx context.Context, cancel context.CancelCauseFunc) []copilot.Tool {
				deadline, ok := ctx.Deadline()
				if tc.unbounded {
					require.False(t, ok)
				} else {
					require.True(t, ok)
					require.InDelta(t, requestTimeout.Seconds(), time.Until(deadline).Seconds(), 1)
				}
				return factory(ctx, cancel)
			}
			err := run(t.Context(), input, path)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}
}

func TestRuntimeCancellationStopsOwnedProcess(t *testing.T) {
	for _, mode := range []string{"hang-start", "hang-send"} {
		t.Run(mode, func(t *testing.T) {
			path, captured := fakeCLI(t, mode)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			var called atomic.Bool
			input := testRequest(&called)
			input.Unbounded = true
			start := time.Now()
			err := run(ctx, input, path)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Less(t, time.Since(start), 4*time.Second)
			capture := readCapture(t, captured)
			_, err = os.Stat(capture.Dir)
			require.ErrorIs(t, err, os.ErrNotExist)
			process, err := os.FindProcess(capture.PID)
			require.NoError(t, err)
			require.ErrorIs(t, process.Signal(syscall.Signal(0)), os.ErrProcessDone)
		})
	}
}

func TestRuntimePreservesCallerCancellationCause(t *testing.T) {
	path, _ := fakeCLI(t, "success")
	input := RuntimeRequest{
		Tools: func(_ context.Context, cancel context.CancelCauseFunc) []copilot.Tool {
			return []copilot.Tool{{
				Name: "submit_result", Parameters: map[string]any{"type": "object"},
				SkipPermission: true, IsTerminal: true,
				Handler: func(copilot.ToolInvocation) (copilot.ToolResult, error) {
					cancel(errCallerDone)
					return copilot.ToolResult{TextResultForLLM: "Finished"}, nil
				},
			}}
		},
	}
	require.ErrorIs(
		t,
		run(t.Context(), input, path),
		errCallerDone,
	)
}

var errCallerDone = errors.New("caller-defined completion")

func TestRuntimeCompletionBelongsToCaller(t *testing.T) {
	path, _ := fakeCLI(t, "text-only")
	var called atomic.Bool
	require.NoError(
		t,
		run(t.Context(), testRequest(&called), path),
	)
	require.False(t, called.Load(), "successful inference need not produce an accepted result")
}

func TestSessionDeniesPermissionsAndUnapprovedTools(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	config := restrictedSessionConfig(
		t.TempDir(),
		cancel,
		[]copilot.Tool{{Name: "submit_result"}},
		"Explicit instructions",
	)
	decision, err := config.OnPermissionRequest(nil, copilot.PermissionInvocation{})
	require.NoError(t, err)
	require.IsType(t, &rpc.PermissionDecisionReject{}, decision)
	require.ErrorIs(t, context.Cause(ctx), ErrRestricted)

	ctx, cancel = context.WithCancelCause(t.Context())
	defer cancel(nil)
	config = restrictedSessionConfig(
		t.TempDir(),
		cancel,
		[]copilot.Tool{{Name: "submit_result"}},
		"",
	)
	hook, err := config.Hooks.OnPreToolUse(
		copilot.PreToolUseHookInput{ToolName: "submit_result"},
		copilot.HookInvocation{},
	)
	require.NoError(t, err)
	require.Equal(t, "allow", hook.PermissionDecision)
	require.NoError(t, context.Cause(ctx))
	hook, err = config.Hooks.OnPreToolUse(
		copilot.PreToolUseHookInput{ToolName: "shell"},
		copilot.HookInvocation{},
	)
	require.NoError(t, err)
	require.Equal(t, "deny", hook.PermissionDecision)
	require.ErrorIs(t, context.Cause(ctx), ErrRestricted)
}

// The helper implements only SDK transport, never inference or network access.
func TestRuntimeHelper(t *testing.T) {
	mode := os.Getenv("OCTOBUD_TEST_RUNTIME")
	if mode == "" {
		return
	}
	if err := serveRuntime(mode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

//nolint:gocyclo // Keep the synthetic protocol cases together for auditability.
func serveRuntime(mode string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	capture := runtimeCapture{
		Env: os.Environ(),
		Dir: dir,
		PID: os.Getpid(),
	}
	sessionID := ""
	send := func(message any) error {
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(data), data)
		return err
	}
	event := func(kind string, data any) error {
		return send(
			map[string]any{"jsonrpc": "2.0", "method": "session.event", "params": map[string]any{
				"sessionId": sessionID,
				"event": map[string]any{
					"id":        kind,
					"parentId":  nil,
					"timestamp": time.Now().UTC(),
					"type":      kind,
					"data":      data,
				},
			}},
		)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		header, err := reader.ReadString('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:")))
		if err != nil {
			return err
		}
		if _, err = reader.ReadString('\n'); err != nil {
			return err
		}
		body := make([]byte, size)
		if _, err = io.ReadFull(reader, body); err != nil {
			return err
		}
		var request runtimeRequest
		if decodeErr := json.Unmarshal(body, &request); decodeErr != nil {
			return decodeErr
		}
		if request.Method == "connect" ||
			request.Method == "session.create" ||
			request.Method == "session.send" {
			capture.Requests = append(capture.Requests, request)
			saved, err := json.Marshal(capture)
			if err != nil {
				return err
			}
			if err := os.WriteFile(os.Getenv("OCTOBUD_TEST_CAPTURE"), saved, 0600); err != nil {
				return err
			}
		}
		if len(request.ID) == 0 || request.Method == "" {
			continue
		}
		response := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{}}
		switch request.Method {
		case "session.detach":
			response["result"] = map[string]any{"success": mode != "cleanup-error"}
		case "connect":
			if mode == "hang-start" {
				continue
			}
			response["result"] = map[string]any{"protocolVersion": 3}
		case "session.create":
			var config struct {
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(request.Params, &config); err != nil {
				return err
			}
			sessionID = config.SessionID
			response["result"] = map[string]any{"sessionId": sessionID}
		case "session.sendAndWait":
			delete(response, "result")
			response["error"] = map[string]any{"code": -32601, "message": "Unsupported"}
		case "session.send":
			if mode == "hang-send" {
				continue
			}
			response["result"] = map[string]any{"messageId": "request"}
		}
		failures := map[string]string{
			"create-error": "session.create",
			"send-error":   "session.send",
		}
		if request.Method == failures[mode] {
			delete(response, "result")
			response["error"] = map[string]any{"code": -32000, "message": "synthetic failure"}
		}
		if err := send(response); err != nil {
			return err
		}
		if request.Method == "session.tools.handlePendingToolCall" {
			if err := event("assistant.message", map[string]any{"content": "Ignored free text", "messageId": "assistant", "originatingMessageId": "request"}); err != nil {
				return err
			}
			if err := event("session.idle", map[string]any{}); err != nil {
				return err
			}
			continue
		}
		if request.Method != "session.send" || mode == "send-error" {
			continue
		}
		if mode == "success" || mode == "cleanup-error" || mode == "external-tool" {
			name := "submit_result"
			if mode == "external-tool" {
				name = "shell"
			}
			if err := event("external_tool.requested", map[string]any{
				"requestId": "tool-request", "sessionId": sessionID, "toolCallId": "tool-call",
				"toolName": name, "arguments": map[string]any{},
			}); err != nil {
				return err
			}
			continue
		}
		content := "Ignored free text"
		switch mode {
		case "tool":
			if err := event("tool.execution_start", map[string]any{"toolCallId": "tool", "toolName": "shell"}); err != nil {
				return err
			}
		case "session-error":
			if err := event("session.error", map[string]any{"errorType": "authentication", "message": "synthetic"}); err != nil {
				return err
			}
		case "overflow":
			content = strings.Repeat("x", maxOutputBytes+1)
		case "output-boundary":
			content = strings.Repeat("x", maxOutputBytes)
		case "cumulative-overflow":
			content = strings.Repeat("x", maxOutputBytes/2+1)
			if err := event("assistant.message", map[string]any{"content": content, "messageId": "first", "originatingMessageId": "request"}); err != nil {
				return err
			}
		}

		message := map[string]any{
			"content":              content,
			"messageId":            "assistant",
			"originatingMessageId": "request",
		}
		if mode == "tool-request" {
			message["toolRequests"] = []any{
				map[string]any{
					"toolCallId": "tool",
					"name":       "shell",
					"arguments":  map[string]any{},
				},
			}
		}
		if err := event("assistant.message", message); err != nil {
			return err
		}
		if err := event("session.idle", map[string]any{}); err != nil {
			return err
		}
	}
}
