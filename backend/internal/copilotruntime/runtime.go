// Package copilotruntime runs explicit application tools in isolated local Copilot
// SDK sessions. Callers own tool policy, accepted results, and completion criteria.
package copilotruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

const (
	maxInputBytes  = 2 * 1024 * 1024
	maxOutputBytes = 512 * 1024
	requestTimeout = 90 * time.Second
)

var (
	ErrUnavailable = errors.New("copilot runtime unavailable")
	ErrLimit       = errors.New("copilot runtime limit exceeded")
	ErrRestricted  = errors.New("copilot runtime restriction violated")
	ErrSession     = errors.New("copilot session failed")
	ErrCleanup     = errors.New("copilot runtime cleanup failed")
)

// RuntimeRequest keeps workflow-specific prompts and tools with the caller.
// Tools receives the request context and can cancel with a caller-owned cause.
type RuntimeRequest struct {
	Model        string
	Instructions string
	Prompt       string
	Tools        func(context.Context, context.CancelCauseFunc) []copilot.Tool
	// Unbounded disables the time, input, and text-output limits, not isolation.
	// Such callers must provide their own cancellation and resource policy.
	Unbounded bool
}

// Run starts an empty, restricted session and waits for inference to finish.
// A nil error does not imply workflow completion: callers must validate their
// accepted tool results. Cancellation causes are returned unchanged, with any
// cleanup error joined. Other failures support errors.Is with the package errors.
func Run(ctx context.Context, input RuntimeRequest) error {
	return run(ctx, input, "")
}

func run(ctx context.Context, input RuntimeRequest, cliPath string) (err error) {
	if input.Tools == nil {
		return errors.New("copilot runtime requires application tools")
	}
	if !input.Unbounded && len(input.Instructions)+len(input.Prompt) > maxInputBytes {
		return fmt.Errorf("%w: input exceeds 2 MiB", ErrLimit)
	}
	timeout := ctx
	if !input.Unbounded {
		var stopTimeout context.CancelFunc
		timeout, stopTimeout = context.WithTimeout(ctx, requestTimeout)
		defer stopTimeout()
	}
	request, cancel := context.WithCancelCause(timeout)
	defer cancel(nil)
	tools := input.Tools(request, cancel)
	if cause := context.Cause(request); cause != nil {
		return cause
	}
	root, err := os.MkdirTemp("", "octobud-copilot-")
	if err != nil {
		return fmt.Errorf("%w: create private scratch data: %w", ErrUnavailable, err)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(root); cleanupErr != nil {
			err = errors.Join(
				err,
				fmt.Errorf("%w: remove private scratch data: %w", ErrCleanup, cleanupErr),
			)
		}
	}()
	if setupErr := prepareScratch(root); setupErr != nil {
		return setupErr
	}
	client := newClient(root, cliPath)
	// Canceling SendAndWait alone does not stop inference.
	stopped := make(chan struct{})
	stopWatch := context.AfterFunc(request, func() {
		client.ForceStop()
		close(stopped)
	})
	defer func() {
		if !stopWatch() {
			<-stopped
		}
		if stopErr := client.Stop(); stopErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: %w", ErrCleanup, stopErr))
		}
	}()
	config := restrictedSessionConfig(
		filepath.Join(root, "work"),
		cancel,
		tools,
		input.Instructions,
	)
	if input.Model != "" {
		config.Model = input.Model
	}
	session, err := client.CreateSession(request, config)
	if err != nil {
		if cause := context.Cause(request); cause != nil {
			return cause
		}
		return fmt.Errorf(
			"%w: cannot create session: %w",
			ErrUnavailable, err,
		)
	}
	unsubscribe := watchSession(session, cancel, tools, input.Unbounded)
	defer unsubscribe()
	_, sendErr := session.SendAndWait(request, copilot.MessageOptions{Prompt: input.Prompt})
	if cause := context.Cause(request); cause != nil {
		return cause
	}
	if sendErr != nil {
		return fmt.Errorf("%w: cannot complete request: %w", ErrSession, sendErr)
	}
	return nil
}

func prepareScratch(root string) error {
	for _, name := range []string{"work", "copilot", "cache"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			return fmt.Errorf("%w: create private directory: %w", ErrUnavailable, err)
		}
	}
	settings := `{"disableAllHooks":true,"ide":{"autoConnect":false},"remoteExport":false,` +
		`"sessionManagementTools":{"enabled":false},"contextManagementTools":false,"toolSearch":false,` +
		`"connectors":false,"autoUpdate":false,"notifications":false}`
	if err := os.WriteFile(filepath.Join(root, "copilot", "settings.json"), []byte(settings), 0600); err != nil {
		return fmt.Errorf("%w: write private settings: %w", ErrUnavailable, err)
	}
	return nil
}

func newClient(root, path string) *copilot.Client {
	state := filepath.Join(root, "copilot")
	cache := filepath.Join(root, "cache")
	runtimePath := "/usr/bin:/bin"
	if ghPath, err := exec.LookPath("gh"); err == nil {
		runtimePath = filepath.Dir(ghPath) + string(os.PathListSeparator) + runtimePath
	}
	if path != "" {
		runtimePath = filepath.Dir(path) + string(os.PathListSeparator) + runtimePath
	}
	env := []string{
		"HOME=" + os.Getenv("HOME"), "USERPROFILE=" + os.Getenv("HOME"),
		"COPILOT_HOME=" + state, "COPILOT_CACHE_HOME=" + cache,
		"TMPDIR=" + root, "TEMP=" + root, "TMP=" + root,
		"COPILOT_PLUGIN_DIR_ONLY=true", "COPILOT_DISABLE_LOGIN_SHELL_ENV=true",
		"COPILOT_ALLOW_ALL=false", "COPILOT_ASSISTED_APPROVAL=false",
		"COPILOT_TELEMETRY_DISABLED=true", "USE_TGREP=false", "CI=true",
		"PATH=" + runtimePath,
	}
	if cliPath := os.Getenv("COPILOT_CLI_PATH"); cliPath != "" {
		env = append(env, "COPILOT_CLI_PATH="+cliPath)
	}
	return copilot.NewClient(&copilot.ClientOptions{
		Connection: copilot.StdioConnection{
			Path: path,
			Args: []string{
				"--disable-builtin-mcps",
				"--no-bash-env",
				"--no-custom-instructions",
				"--sync-session=off",
				"--disable-client-telemetry",
			},
		},
		Mode:             copilot.ModeEmpty,
		BaseDirectory:    state,
		WorkingDirectory: filepath.Join(root, "work"),
		Env:              env,
		LogLevel:         "none",
	})
}

func watchSession(
	session *copilot.Session,
	cancel context.CancelCauseFunc,
	tools []copilot.Tool,
	unbounded bool,
) func() {
	var mu sync.Mutex
	outputBytes := 0
	allowed := make(map[string]bool, len(tools))
	for _, tool := range tools {
		allowed[tool.Name] = true
	}
	return session.On(func(event copilot.SessionEvent) {
		switch data := event.Data.(type) {
		case *copilot.ExternalToolRequestedData:
			if !allowed[data.ToolName] {
				cancel(fmt.Errorf("%w: unapproved tool requested", ErrRestricted))
			}
		case *copilot.ToolExecutionStartData:
			if !allowed[data.ToolName] {
				cancel(fmt.Errorf("%w: unapproved tool attempted", ErrRestricted))
			}
		case *copilot.AssistantMessageData:
			for _, tool := range data.ToolRequests {
				if !allowed[tool.Name] {
					cancel(fmt.Errorf("%w: unapproved tool requested", ErrRestricted))
				}
			}
			mu.Lock()
			outputBytes += len(data.Content)
			exceeded := outputBytes > maxOutputBytes
			mu.Unlock()
			if exceeded && !unbounded {
				cancel(fmt.Errorf("%w: text output exceeds 512 KiB", ErrLimit))
			}
		case *copilot.SessionErrorData:
			cancel(fmt.Errorf("%w: check runtime and credential", ErrSession))
		}
	})
}

func restrictedSessionConfig(
	work string,
	cancel context.CancelCauseFunc,
	tools []copilot.Tool,
	system string,
) *copilot.SessionConfig {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return &copilot.SessionConfig{
		WorkingDirectory: work,
		SystemMessage: &copilot.SystemMessageConfig{
			Mode:    "replace",
			Content: system,
		},
		Tools:                              tools,
		AvailableTools:                     names,
		ExcludedTools:                      []string{"builtin:*", "mcp:*"},
		Streaming:                          copilot.Bool(false),
		EnableConfigDiscovery:              copilot.Bool(false),
		EnableOnDemandInstructionDiscovery: copilot.Bool(false),
		EnableFileHooks:                    copilot.Bool(false),
		EnableHostGitOperations:            copilot.Bool(false),
		EnableSessionStore:                 copilot.Bool(false),
		EnableSkills:                       copilot.Bool(false),
		EnableSessionTelemetry:             copilot.Bool(false),
		SkipCustomInstructions:             copilot.Bool(true),
		InfiniteSessions: &copilot.InfiniteSessionConfig{
			Enabled: copilot.Bool(false),
		},
		OnPermissionRequest: func(copilot.PermissionRequest, copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
			cancel(fmt.Errorf("%w: permission requested", ErrRestricted))
			return &rpc.PermissionDecisionReject{}, nil
		},
		Hooks: &copilot.SessionHooks{
			OnPreToolUse: func(
				input copilot.PreToolUseHookInput,
				_ copilot.HookInvocation,
			) (*copilot.PreToolUseHookOutput, error) {
				if slices.Contains(names, input.ToolName) {
					return &copilot.PreToolUseHookOutput{PermissionDecision: "allow"}, nil
				}
				cancel(fmt.Errorf("%w: unapproved tool attempted", ErrRestricted))
				return &copilot.PreToolUseHookOutput{PermissionDecision: "deny"}, nil
			},
		},
	}
}
