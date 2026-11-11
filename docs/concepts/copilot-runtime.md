# Local Copilot SDK runtime

`backend/internal/copilotruntime` is a thin wrapper for using the Copilot SDK
to run prompts locally. It does not create prompts or interpret model results.

Call `copilotruntime.Run(ctx, RuntimeRequest)` with the instructions, prompt,
optional model, and application tools. Tool handlers own their policy and
results; `Run` returns an error, not generated text or a workflow result.

The SDK uses its default runtime selection and authentication. The runtime must
be bundled into the application or specified using the SDK's `COPILOT_CLI_PATH`
environment variable. GitHub CLI authentication requires `gh` on `PATH`.
