// Package copilotruntime runs application tools in local Copilot SDK sessions.
package copilotruntime

import (
	"context"
	"errors"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

type RuntimeRequest struct {
	Model        string
	Instructions string
	Prompt       string
	Tools        func(context.Context, context.CancelCauseFunc) []copilot.Tool
}

func Run(ctx context.Context, input RuntimeRequest) (err error) {
	if input.Tools == nil {
		return errors.New("copilot runtime requires application tools")
	}
	request, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	tools := input.Tools(request, cancel)
	if cause := context.Cause(request); cause != nil {
		return cause
	}
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	client := copilot.NewClient(nil)
	defer func() { err = errors.Join(err, client.Stop()) }()
	session, err := client.CreateSession(request, &copilot.SessionConfig{
		Model: input.Model,
		SystemMessage: &copilot.SystemMessageConfig{
			Mode: "replace", Content: input.Instructions,
		},
		Tools: tools, AvailableTools: names,
		OnPermissionRequest: func(copilot.PermissionRequest, copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
			return &rpc.PermissionDecisionReject{}, nil
		},
	})
	if err != nil {
		return err
	}
	_, err = session.SendAndWait(request, copilot.MessageOptions{Prompt: input.Prompt})
	return err
}
