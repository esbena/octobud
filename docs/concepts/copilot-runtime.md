# Local Copilot SDK runtime

`backend/internal/copilotruntime` is a thin wrapper for using the Copilot SDK
to run prompts locally. It does not create prompts or interpret model results.

Call `copilotruntime.Run(ctx, RuntimeRequest)` with the instructions, prompt,
optional model, and application tools. Tool handlers own their policy and
results; `Run` returns an error, not generated text or a workflow result.

The SDK uses its default runtime selection and authentication. The runtime must
be bundled into the application or specified using the SDK's `COPILOT_CLI_PATH`
environment variable. GitHub CLI authentication requires `gh` on `PATH`.

## Enablement and authentication

Enable Copilot in **Settings → Copilot**, then enable the individual features
you want. The master and activity-highlights switches are off by default.
Preferences are persisted locally; saving settings never makes a model request.

Copilot uses authentication available to the Octobud backend's local Copilot
runtime, which may differ from your Octobud GitHub account. Sign in with
`copilot login`, or use an authenticated GitHub CLI (`gh`). To supply a different
token, set `COPILOT_GITHUB_TOKEN` in the environment that launches Octobud.
Octobud does not store this token.

The runtime forwards the standard `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, and
`GITHUB_TOKEN` overrides, as well as `GH_CONFIG_DIR` for GitHub CLI discovery.
Copilot CLI chooses credentials in that order, followed by its saved login and
then `gh auth token`. A PAT must be fine-grained with the **Copilot Requests**
account permission. Classic PATs used for Octobud's notifications cannot be
reused. See [Copilot CLI authentication](https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/authenticate-copilot-cli).
