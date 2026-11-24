<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="frontend/static/baby_octo.svg">
    <source media="(prefers-color-scheme: light)" srcset="frontend/static/baby_octo_dark.svg">
    <img src="frontend/static/baby_octo_dark.svg" alt="Octobud Logo" width="120" height="120">
  </picture>
</p>

<h1 align="center">Octobud</h1>

<p align="center">
  <strong>Superfast GitHub Notification Inbox.</strong>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#documentation">Documentation</a> •
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <a href="https://github.com/octobud-hq/octobud/releases/latest">
    <img src="https://img.shields.io/badge/Download-Latest%20Release-blue?style=for-the-badge" alt="Download Latest Release">
  </a>
</p>

<p align="center">
  <a href="https://github.com/octobud-hq/octobud/actions/workflows/ci.yml?query=branch%3Amain"><img src="https://github.com/octobud-hq/octobud/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI Status"></a>
  <a href="LICENSE.txt"><img src="https://img.shields.io/badge/license-AGPL--3.0-blue.svg" alt="License"></a>
  <a href="https://github.com/octobud-hq/octobud"><img src="https://img.shields.io/github/stars/octobud-hq/octobud?style=social" alt="GitHub Stars"></a>
</p>

---

Octobud is a Mac desktop application that helps you manage the flood of GitHub notifications with filtering, custom views, keyboard shortcuts, and automation rules.

Built with **Go** and **Svelte 5**. Runs as a single binary with SQLite - no external dependencies required. On macOS, Octobud runs as a menu bar app that opens your browser to a local web interface.

### Platform Support

- **macOS**: Full support with menu bar integration, auto-start, and secure Keychain storage for GitHub credentials
- **Linux & Windows**: Core functionality works, but menu bar integration and auto-start are not available. GitHub credentials are encrypted at rest, **but the encryption key is stored locally, so an attacker with machine access could decrypt them**. Keychain support is planned for a future release.

## Why?

If you manage many GitHub notifications, you know that organizing and triaging them efficiently can be a major challenge.

Octobud builds on GitHub's notification system and provides a smart, dedicated inbox experience: rich filtering and querying, custom views, keyboard shortcuts, automation rules, and more, all running locally on your machine.

## Features

![Smart Notification Inbox](docs/media/hero_split.gif)

#### ⚡ Super Fast Loading
Issue, PR, and Discussion data is stored locally for instant lookup and filtering

#### 📜 Rich Timeline
See the full story of an issue or PR: commits, reviews, comments, and status changes

Switch to **What's new** to read activity within the selected activity window,
including opening posts created or edited within that window, with familiar author cards,
human activity first, and compact thread context. Enable **Open What's new by default**
in Settings → Preferences to start there. **Since last read** includes all available
history when you have not read the notification before. When no cutoff is available
or the activity window includes the opening event, you can switch directly to **Full view**
to read the conversation from the beginning.
Tab counts load in the background and count individual activity entries, including replies.
What's new counts its displayed entries, including older context, without counting duplicates.
The selected tab is linked as `reader=full` or `reader=whats-new` in the URL, so reloads,
shared links, and browser Back/Forward restore it. Without a valid `reader` parameter,
the saved default applies.
Use each item's GitHub link to reply or take action.

#### 📥 Full Lifecycle Management
Star, Snooze, Archive, Tag, and Mute notifications

Individual notification actions update the list immediately after saving. Background refreshes
are debounced by 250 ms (at most one second of waiting) and coalesced while a refresh
is running, so rapid triage does not recount every view for each action.

Settings → Preferences → **Built-in sidebar views** lets you independently hide
Inbox, Starred, Snoozed, Archive, and Everything from the sidebar. All are shown
by default, and the choices are saved locally. Hidden views still open through
direct links, keyboard navigation, and the macOS menu bar without reappearing in
the sidebar. You can create custom views as alternatives.

### More

| Feature | Description |
|---------|-------------|
| **Rich Querying** | Filter with a powerful query language |
| **Custom Views** | Create saved filtered views for your workflow |
| **Automation Rules** | Filter, tag, archive and more with query-based rules |
| **View Annotations** | Run your JavaScript in the browser to add label-style information to view entries |
| **Keyboard-First** | Vim-style navigation and intuitive shortcuts |
| **Desktop Notifications** | Never miss a review request or issue reply |
| **Inline Details** | View issue and PR status, comments, and more inline |
| **Split Pane Mode** | View details side by side with your inbox |
| **Tags** | Organize with custom tags and colors |
| **Real-Time Sync** | Background sync keeps everything up to date |
| **Privacy-First** | Local-first, your data stays on your machine |

### View annotations

Open **Settings → Annotations** to create reusable annotations. Give each a name,
enter a JavaScript function expression, and check the views where it should run.
Selecting no views runs the annotation in all views. An independent **Enable annotation**
toggle lets you pause it without losing its selected views. Expand an annotation
in the overview to inspect its code and views, toggle enablement, or open its editor.
Existing annotations remain enabled when upgrading. Definitions, enablement and view assignments
are saved in Octobud's database; results are computed only in the browser and are
not saved as tags or used to change view membership.

```js
({ notification, subject }) => {
  if (!subject?.body?.includes("BREAKING CHANGE")) return [];
  return [{
    text: "⚠️ Breaking change",
    color: "#d93f0b",
    hoverText: "The stored description mentions a breaking change."
  }];
}
```

The function receives one object: `notification` contains the entry's frontend
fields, `notification.raw` is its complete API snapshot, and `subject` is the stored
raw GitHub subject JSON, or `null` if it is unavailable. The original notification
payload is accessible as `notification.payload`. `notification.subjectFetchedAt`
indicates when the subject was fetched, if known. View list requests include stored
subject JSON so bodies can be inspected without opening each entry. This does not
fetch fresh GitHub data or load comments/timeline events for annotations.

Return an array of labels, or `[]` for no results. Each label requires non-empty
`text` and a six-digit hex `color` (`#RRGGBB`); `hoverText` is optional.
All text is plain text, including Unicode emojis: HTML, Markdown and emoji
shortcodes are not interpreted. Octobud renders the labels and selects contrasting
text colors. Async functions are supported, with pending indicators while they run
and visible errors for exceptions or invalid output.

Annotation code is trusted user code and runs directly in the browser with
`await`, not in a worker or sandbox. Only use code you trust. Synchronous expensive
work or infinite loops can block the UI; a function that never settles will remain
pending. Inputs are copied so accidental mutation does not change the entry.

The annotation editor includes a **Test annotation** panel. Paste an Octobud view
URL such as `http://localhost:8808/views/inbox?id=27088099001` and click **Load input**
to inspect the JSON argument built from that notification's stored snapshot.
Only the notification ID is taken from the URL; data is loaded from the current
Octobud instance without marking it read or fetching fresh GitHub data.
Click **Run annotation** to execute the current unsaved code and inspect its
validated label output as JSON. Logs appear in the browser developer console.
Changing the code clears the previous output; changing the URL clears the loaded input.

## Quick Start

### macOS

**[Download Latest Release](https://github.com/octobud-hq/octobud/releases/latest)** - Download the `.pkg` installer for your Mac (Apple Silicon or Intel). Octobud will appear in your menu bar and open automatically in your browser.

### Linux & Windows (Build from Source)

Official releases with embedded assets are currently only available for macOS. For Linux and Windows, you must build from source (see below). The app will be accessible at `http://localhost:8808` in your browser. Note: Menu bar integration and auto-start are not available on these platforms.

**Note:** Keychain support is not yet implemented on Linux and Windows. Your GitHub OAuth token or PAT will be encrypted at rest, but because the encryption key will be stored in plain text, anyone with access to your machine can still get your token. 

### Build from Source

**Prerequisites:** [Go 1.26+](https://go.dev/dl/) and [Node.js 24+](https://nodejs.org/)

```bash
git clone https://github.com/octobud-hq/octobud.git && cd octobud
make build
./bin/octobud
```

🎉 That's it! On macOS, Octobud will appear in your menu bar and open in your browser. On Linux and Windows, open `http://localhost:8808` manually. Connect your GitHub account using either **OAuth** or a **Personal Access Token (PAT)**. Both methods work equally well - choose OAuth for convenience, or PAT if your organization disables OAuth apps or you have multiple GitHub orgs. See the [OAuth Setup Guide](docs/guides/oauth-setup.md) or [Personal Access Token Setup Guide](docs/guides/personal-access-token-setup.md) for detailed instructions.

`make build` produces a development binary: it does not prompt to restart into an installed app bundle, and the app restart endpoint is disabled. Restart the binary manually after rebuilding.

<details>
<summary><strong>More options</strong></summary>

### Development Mode (Frontend Hot Reload)

Run the backend and frontend separately for hot reload during development:

```bash
# Terminal 1: Backend
make backend-dev

# Terminal 2: Frontend
make frontend-dev
```

`make backend-dev` also uses development build behavior, so restart the backend manually when needed.

Open `http://localhost:5173`

### Command Line Options

```bash
./bin/octobud --help
  -port int        Port to listen on (default 8808)
  -data-dir string Data directory (default: platform-specific)
  -no-open         Don't open browser on startup
  -version         Show version and exit
```

</details>

## After Installation

When you first launch Octobud, you'll be guided through setup:

1. **Connect GitHub** - You'll be prompted to connect your GitHub account. Choose either **OAuth** or a **Personal Access Token (PAT)**. Both methods work equally well:
   - **OAuth** - Convenient and secure, uses GitHub's device flow
   - **PAT** - Recommended if your organization disables OAuth apps or you have multiple GitHub orgs
   
   Both require the following scopes:
   - `repo`
   - `notifications`
   - `read:discussions` (this permission is only required for PAT)
   
   See the [OAuth Setup Guide](docs/guides/oauth-setup.md) or [Personal Access Token Setup Guide](docs/guides/personal-access-token-setup.md) for detailed instructions. You can update your authentication method later in Settings → Account.

2. **Configure Initial Sync** - Next, you'll be prompted to configure how far back to sync notifications. Start small (30 days) - you can always [sync more later](docs/start-here.md#syncing-more-later).

3. **Learn the basics** - Press `h` for keyboard shortcuts, or see [Start Here](docs/start-here.md).

### Recommended Starting Views and Rules

Here are some recommended views and rules to help you get started:

**Views:**

- **Assigned / Authored** - `in:inbox,filtered (reason:assign OR author:<YOUR_USERNAME>)`
- **Reviews** - `in:inbox,filtered reason:review_requested`

**Views with auto-filter rule:**

- **[Bots]** - `in:inbox,filtered author:[bot]` + rule to skip inbox

See the [Views & Rules guide](docs/guides/views-and-rules.md) for detailed instructions on creating views and rules.

### Reset on macOS (clean reinstall)

If you want to start over, quit Octobud and remove its local state:

```bash
osascript -e 'quit app "Octobud"' 2>/dev/null || true

rm -rf ~/Library/Application\ Support/Octobud
rm -f ~/Library/Preferences/io.octobud.plist
rm -rf ~/Library/Caches/io.octobud

osascript -e 'tell application "System Events" to delete login item "Octobud"' 2>/dev/null || true

while security delete-generic-password -s io.octobud >/dev/null 2>&1; do :; done
```

Then launch Octobud again. It starts with a fresh setup flow.

If you ran Octobud with `--data-dir`, also remove that custom directory.

## Documentation

- [Start Here](docs/start-here.md) - Initial setup and core workflows
- [OAuth Setup](docs/guides/oauth-setup.md) - Complete guide for OAuth authentication
- [Personal Access Token Setup](docs/guides/personal-access-token-setup.md) - Complete guide for PAT setup
- [Query Syntax](docs/guides/query-syntax.md) - Filter and search notifications
- [Keyboard Shortcuts](docs/guides/keyboard-shortcuts.md) - Navigate efficiently
- [Views & Rules](docs/guides/views-and-rules.md) - Organize your inbox
- [Concepts](docs/concepts/) - How syncing, queries, and actions work
- [Architecture](docs/ARCHITECTURE.md) - Technical architecture overview
- [Migration Guide](docs/MIGRATION.md) - Upgrading and database migrations

## Roadmap

See [ROADMAP.md](ROADMAP.md) for planned features.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](docs/CONTRIBUTING.md) for guidelines.

- 🐛 [Report bugs](https://github.com/octobud-hq/octobud/issues/new?template=bug_report.md)
- 💡 [Request features](https://github.com/octobud-hq/octobud/issues/new?template=feature_request.md)
- 📖 [Improve documentation](https://github.com/octobud-hq/octobud/issues/new?template=docs.md)

## Security

Found a security vulnerability? Please report it responsibly. See [SECURITY.md](SECURITY.md) for details.

## License

Octobud is licensed under the [GNU Affero General Public License v3.0](LICENSE.txt) (AGPL-3.0).

This means you're free to use, modify, and distribute this software, but if you run a modified version as a network service, you must make the source code available to users of that service.

## Community

- 💬 [Discussions](https://github.com/octobud-hq/octobud/discussions) - Ask questions and share ideas
- 🐛 [Issue Tracker](https://github.com/octobud-hq/octobud/issues) - Report bugs and request features
- 📚 [Documentation](docs/) - Comprehensive guides and reference

## Disclaimer

Octobud is an independent, open-source project. It is **not affiliated with, endorsed by, or officially supported by GitHub, Inc.** GitHub and the GitHub logo are trademarks of GitHub, Inc.

---

<p align="center">
  Made with ❤️ by someone who wanted a better way to stay on top of GitHub notifications.
</p>
