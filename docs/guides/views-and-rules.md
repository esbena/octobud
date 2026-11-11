# Views and Rules

Views and rules help you organize and automate your notification workflow.

## Views

Views are saved queries that create filtered lists of notifications. They appear in your sidebar for quick access.

### Built-in Views

Octobud comes with several default views:

- **Inbox** - All notifications which haven't been archived, muted, snoozed, or routed to skip the inbox by a rule
- **Starred** - Starred notifications
- **Snoozed** - Currently snoozed notifications
- **Archived** - Archived notifications
- **Everything** - Everything, including muted notifications

### Create a View from the Search Bar

The easiest way to create a view:

1. Type a query in the search bar (e.g., `in:inbox type:pullrequest reason:review_requested`)
2. Click the **Save** button in the search bar and then click **Save as new view**
3. Give it a name and **Save view**

That's it! The view is now in your sidebar.

**Example:** Create a view for PRs that need review:
```
in:inbox type:pullrequest reason:review_requested
```

### Creating a View (Detailed Steps)

For more control when creating a view:

1. Click the **Add** button in the sidebar, or click the **Save** button in the query input to start creating a new view based on the current query
2. Enter a name for your view
3. Write a query to filter notifications (see [Query Syntax](query-syntax.md))
   - **Default query:** The query field is pre-populated with `in:inbox,filtered`
   - **Note:** Custom views default to `in:inbox,filtered`, which includes both inbox and filtered notifications. This makes custom views good targets for skip inbox rules, as notifications that skip the inbox will still appear in your view.
4. Choose an icon
5. Click **Save**

### Recommended Starting Views

Here are some recommended views to help you get started:

**Assigned / Authored**
```
in:inbox,filtered (reason:assign OR author:<YOUR_USERNAME>)
```
Replace `<YOUR_USERNAME>` with your GitHub username.

**Reviews**
```
in:inbox,filtered reason:review_requested
```

**Bots** (with auto-filter rule)
```
in:inbox,filtered author:[bot]
```
Create this view and then add a rule with the same query (`author:[bot]`) that skips the inbox. This will automatically filter bot notifications out of your inbox while keeping them accessible in this view.

### Example Views

**Needs Review**
```
type:pullrequest reason:review_requested
```

**My PRs**
```
type:pullrequest reason:author
```

**Urgent**
```
(reason:mention OR reason:review_requested) AND is:unread
```

**Work Repos**
```
org:my-company -is:archived
```

### Default Repositories

A view can remember a set of repositories to show by default. Open the repository selector above the list (or press `f`), pick the repositories, and choose **Set as default for this view**. Saving a view from the search bar's **Save** menu also stores the current repository selection.

When you open the view it starts scoped to those repositories. The selector still lists every repository with unread counts, and the bar shows how many unread notifications are in repositories outside the selection, so nothing is hidden for good. Pick other repositories at any time, or use **Reset to default** in the selector to go back. Sidebar badges always count the whole view, not just the default repositories.

### Managing Views

- **Reorder** - Drag and drop views in the sidebar
- **Edit** - Right-click a view to edit
- **Delete** - Right-click and select delete

The **Show in sidebar** checkbox is enabled by default when creating a view. Turn it
off to move the view into the collapsed **More views** section at the bottom of the
views list. These views have no unread badges, and Octobud skips their independent
badge queries, even when **More views** is expanded.

This setting only affects sidebar presentation. The view keeps its saved query,
URL, and references from other queries or linked rules. It is separate from the
default landing-view setting.

### Views Based on Other Views

Use `in:"View name"` to match another custom view's saved query. For example, keep a **Low priority** view for noise and define an actionable view as:

```
in:inbox,filtered -in:"Low priority"
```

The low-priority view keeps its own unread badge. Topic-focused views can overlap it or exclude it explicitly; items without a topic are not automatically low priority.

Membership follows saved query changes immediately. It does not depend on sidebar order, which views you have opened, or their default repository selections. Renaming a view updates its references in other saved view queries. Deletion is blocked until dependent views remove their references.

Use the full view name, matched case-insensitively. Missing references and cycles are errors.

## Rules

Rules automatically apply actions to notifications that match a query. They help automate your workflow.

### How Rules Work

When a new notification arrives, Octobud checks it against all enabled rules in order. If a notification matches a rule's query, the rule's actions are applied.

### Creating a Simple Rule

Start with a basic rule to see how they work:

1. Go to **Settings** → **Rules** → **New Rule**
2. Choose **Query-based**
3. Enter a query (e.g., `author:dependabot`)
4. Select an action (e.g., **Archive**)
5. Click **Create rule**

**Example:** Auto-archive all Dependabot notifications:
- Query: `author:dependabot`
- Action: Archive

### Creating a Rule (Detailed Steps)

1. Go to **Settings** → **Rules**
2. Click **New Rule**
3. Configure the rule:

**Step 1: Choose Rule Type**
- **Query-based** - Use a custom query to match notifications
- **View-linked** - Link to a view's query (automatically updates when the view changes)

**Step 2: Define the Query or View**
- For query-based: Enter a filter query (`in:` accepts only built-in locations, not named views)
- For view-linked: Select an existing custom view from the dropdown

**Step 3: Configure Rule Details**
- **Rule name** (required) - A descriptive name like "Dependabot PRs"
- **Description** (optional) - What this rule does

**Step 4: Select Actions**
Choose one or more actions to apply to matching notifications:
- Skip inbox
- Mark as read
- Star
- Archive
- Mute

**Step 5: Apply Tags (optional)**
Select tags to automatically apply to matching notifications

**Step 6: Additional Options**
- **Enable rule** - Toggle the rule on/off
- **Apply to existing notifications** - Retroactively apply to all existing notifications (only shown when creating)

4. Click **Create rule**

### Available Actions

| Action | Description |
|--------|-------------|
| **Skip Inbox** | Mark as filtered (won't appear in inbox, but can appear in custom views) |
| **Mark as Read** | Automatically mark as read |
| **Archive** | Move to archive |
| **Star** | Add star |
| **Mute** | Mute the notification (also archives it) |

### Query-based vs View-linked Rules

**Query-based rules** have their own independent query. Use these when you want a rule that doesn't correspond to any view.

**View-linked rules** inherit their query from an existing view. When you update the view's query, the rule automatically uses the new query. This is useful when you want a view and a rule to always stay in sync.

### Example Rules

**Auto-archive Dependabot**
```
Type: Query-based
Query: author:dependabot
Actions: Archive
```

**Skip CI Noise**
```
Type: Query-based
Query: type:CheckSuite
Actions: Skip Inbox, Mark as Read
```

**Tag Security Alerts**
```
Type: Query-based
Query: type:RepositoryVulnerabilityAlert
Actions: (none)
Tags: security
```

**Auto-star Review Requests**
```
Type: Query-based
Query: reason:review_requested
Actions: Star
```

### Rule Order

Rules are processed in order from top to bottom. You can reorder rules by dragging them.

**Tip:** Put more specific rules before general ones.

## Activity highlights

**Summarize 10 more** queues up to ten missing summaries from the top of the
current view, across pages, using its submitted query and repository filters.
Cached and pending items are skipped; repeated clicks extend the queue without
duplicates. Browsing, scrolling, changing the lookback or polling never starts
generation. Per-view automatic generation is not enabled.
Use **Summarize activity** (the Copilot icon) in a notification's hover actions
or detail action bar to summarize just that issue, PR or discussion. It uses
the selected lookback and the same queue and cache as batches; current results
are reused, pending actions are disabled, and failures can be retried explicitly.

**Max lookback** offers 1, 3, 7, 14 or 30 days, initially using the configured
default (3 days unless changed). This is a maximum
activity window, not activity since an archive: archive timestamps are not yet
recorded. Results show inferred **For you** requests first and quieter **Other
activity** below. Empty groups are omitted; no **For you** section is not an
all-clear. Highlights describe new developments, not the original thread.

In **Settings → Copilot**, enable Copilot and then **Activity highlights**.
Both switches are off by default and persisted in the local database; no
Octobud feature environment variable is needed. The feature has its own default
lookback (3 days) and model (`gpt-6-luna`), used for new summaries. Enabling or
saving settings never starts generation. Disabling either switch cancels queued
and running summaries while preserving the other preferences.
Until enabled, views show only a setup link rather than inactive generation
controls. Once enabled, the compact highlights toolbar links back to these settings.

Generation uses the [local Copilot SDK runtime](../concepts/copilot-runtime.md).
Copilot authentication is separate from Octobud's notifications token; the
settings page explains local login and standard token overrides.

The backend runs up to ten jobs concurrently. **Stop** cancels the user's
waiting and running jobs, including their owned SDK runtimes. Errors are
displayed, not automatically retried; another explicit batch action can retry
failed items. The server-memory cache is shared across views and isolated by
user, notification version and lookback. Completed entries expire after one
hour, at most 500 entries are retained, and at most 100 jobs may be pending.
Restarting clears the cache. A full queue returns an error or partial-admission
notice. Neither generation nor caching changes notification archive/read state.

The prompt classifies recipients, ranks and limits developments, then summarizes
at most three bullets with usernames and new-event citations. Other reviewers'
feedback to the author is background when the reader is a fellow reviewer.
Copilot review bodies are withheld; human replies remain eligible. A generic
Copilot-notes mention is allowed only when it is the sole substantive change.

Issues/PRs use GitHub timelines and PR inline comments; discussions include
available replies. Each source is bounded to ten pages of 100 entries, with
coverage warnings. New replies under old parents are checked independently.
Current edited bodies are not edit diffs, and commit authored dates are not push
dates. Exact body/code deltas and deleted history cannot be reconstructed without
historical snapshots.
In the detail view, expand **Window and coverage** for the cutoff and coverage
notes; recipient labels are inferred rather than assurances that no action is needed.

API:

- `GET /api/user/copilot-settings` and `PUT /api/user/copilot-settings` read and
  save the master switch and feature preferences without inference.
- `POST /api/notifications/activity-briefs` accepts `query`, `repositoryIds`,
  and `days`; the server selects at most ten missing supported notifications.
  Alternatively, supply `notificationId` and `days` without view filters to
  queue only that notification.
- `GET /api/notifications/activity-briefs` reads authenticated cache/queue status
  without GitHub or model calls.
- `POST /api/notifications/activity-briefs/stop` stops that user's pending work.
- `POST /api/notifications/{githubID}/activity-brief` accepts an explicit RFC3339
  `since` and optional `summarize` boolean. Without summarization it returns
  deterministic event kinds and partitioned evidence without inference.

Each inference uses only the terminal `submit_brief` tool, with no notification
actions, filesystem or MCP tools. Jobs have a two-minute total deadline;
inference is bounded to 90 seconds, 2 MiB input, 512 KiB assistant text and eight
submission attempts. Invalid/unsupported requests return 400, unavailable
configuration returns 503, and GitHub/runtime failures surface explicitly.

## Tags

Tags help you categorize and organize notifications.

### Creating Tags

1. Click the **Add** button in the sidebar's Tags section, or right-click an existing tag to edit it
2. Click **Add Tag**
3. Enter a name and choose a color
4. Click **Save**

### Using Tags

- **Manual tagging** - Select notifications and press `t` to add/remove tags
- **Rule-based tagging** - Create rules that automatically add tags
- **Filter by tag** - Use `tags:name` in queries

### Example Tags

- `urgent` - Needs immediate attention
- `waiting` - Waiting for someone else
- `review` - Needs your review
- `followup` - Follow up later

## Best Practices

1. **Start with Views** - Create views for your main workflows before adding rules
2. **Use View-linked Rules** - When you want a view and rule to stay in sync
3. **Use Rules Sparingly** - Too many rules can make behavior unpredictable
4. **Test Queries First** - Test your query in the search bar before using in a rule
5. **Review Filtered** - Periodically enter the `in:filtered` query in the search bar to ensure rules aren't unexpectedly hiding important notifications
6. **Combine Tags and Rules** - Use rules to auto-tag, then create views based on tags
