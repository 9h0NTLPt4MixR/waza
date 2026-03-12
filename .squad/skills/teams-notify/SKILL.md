# Teams Notifications — Squad Communication Channel

## Overview

The Teams Notifications skill manages real-time communication of squad milestones to the Microsoft Teams channel _"Waza Squad"_. When significant work completes—PRs merge, decisions are recorded, or tasks finish—the squad receives rich HTML notifications in Teams without requiring webhooks, Power Automate, or app registrations.

This skill keeps the team synchronized on progress without interrupting focused work.

---

## How It Works

### Architecture

The system uses **Microsoft Graph API via Azure CLI** (`az rest`) to post messages directly to Teams:

1. **Scribe** (or other workflow) calls the notification script after work completes
2. Script reads config from `.squad/identity/teams-config.json` to determine:
   - Whether notifications are enabled
   - Which Teams group and channel to post to
   - Which event types should trigger notifications
3. Script formats the event as HTML and posts via Graph API
4. Message appears in the Teams channel within seconds

No webhooks, no Power Automate, no additional app registration required — just Azure CLI with Team owner permissions.

### Flow Example

```
[Work Complete: PR #42 Merged]
         ↓
    Scribe calls notification script
         ↓
    Script reads teams-config.json (enabled=true)
         ↓
    Formats message: "PR #42 merged to main"
         ↓
    POST /v1.0/teams/{groupId}/channels/{channelId}/messages
         ↓
    Message appears in Teams channel
```

---

## Configuration

### Location
`.squad/identity/teams-config.json`

### Schema

```json
{
  "enabled": false,
  "group_id": "your-group-id-here",
  "channel_id": "19:your-channel-id-here@thread.tacv2",
  "channel_name": "Waza Squad",
  "notify_on": {
    "work_complete": true,
    "pr_opened": true,
    "pr_merged": true,
    "issue_closed": true,
    "decisions": true,
    "ralph_status": false
  }
}
```

### Configuration Fields

| Field | Type | Purpose |
|-------|------|---------|
| `enabled` | boolean | Master toggle for all notifications. Set to `false` to silence all events |
| `group_id` | UUID | Teams group (tenant) ID — extracted from Teams channel settings |
| `channel_id` | string | Channel conversation ID — extract from channel link or Teams app |
| `channel_name` | string | Display name (for reference; not used in API calls) |
| `notify_on` | object | Per-event toggles — each event type can be enabled/disabled independently |

### Extracting Channel IDs from Teams

**Find Group ID and Channel ID:**
1. In Microsoft Teams, right-click the channel → **Copy channel link**
2. Paste the link. Format is: `https://teams.microsoft.com/l/channel/...@thread.tacv2/...`
3. Use this format to extract:
   - **Group ID:** Copy from your Teams org settings or Azure portal
   - **Channel ID:** The `...@thread.tacv2` part is the channel ID

**Alternatively, use Azure CLI:**
```bash
az teams channel list --team-id <group_id>
```

### Editing Configuration

Edit `.squad/identity/teams-config.json` directly:

**Enable/disable all notifications:**
```json
{
  "enabled": false
}
```

**Disable a specific event type:**
```json
{
  "notify_on": {
    "work_complete": false,
    "pr_merged": true
  }
}
```

**Change channel:**
```json
{
  "group_id": "new-group-id",
  "channel_id": "new-channel@thread.tacv2"
}
```

---

## Usage

### Manual Notification

Send a notification manually using the teams-notify script:

```bash
.squad/scripts/teams-notify.sh <event_type> "<message>"
```

**Examples:**

```bash
# Send work complete notification
.squad/scripts/teams-notify.sh work_complete "Completed spike on token optimization"

# Send PR merged notification
.squad/scripts/teams-notify.sh pr_merged "PR #42 merged: Add teams notification support"

# Send decision notification
.squad/scripts/teams-notify.sh decisions "Decided to use Microsoft Graph API for Teams integration"
```

### Automated Notifications

Scribe and other workflows call the script automatically after significant events:

```bash
# After a work batch completes
.squad/scripts/teams-notify.sh work_complete "Squad completed sprint milestone: E1 foundation"

# When a PR merges
.squad/scripts/teams-notify.sh pr_merged "PR #${PR_NUMBER} merged by ${AUTHOR}"

# When decisions are recorded
.squad/scripts/teams-notify.sh decisions "New decision recorded: teams-notify system"
```

The script exits silently if notifications are disabled — no errors, no noise.

---

## Event Types

The system recognizes these event categories:

| Event Type | When It Fires | Typical Message |
|------------|---------------|-----------------|
| `work_complete` | Squad finishes a work batch or milestone | "Completed feature development for E3" |
| `pr_opened` | Pull request opened by a squad member | "PR #42 opened: Add token budgeting" |
| `pr_merged` | PR merges to `main` or `preview` | "PR #42 merged to main" |
| `issue_closed` | GitHub issue marked closed | "Issue #89 closed: CI/CD integration guide" |
| `decisions` | Team decision recorded in `.squad/decisions/` | "Decided to use Claude Opus 4.6 for coding" |
| `ralph_status` | Sensei compliance check completes | "Sensei check: 3 skills updated, 2 need work" |

Enable or disable any event in `notify_on` configuration.

---

## Troubleshooting

### Messages Not Appearing

**Check 1: Is Teams integration enabled?**
```bash
cat .squad/identity/teams-config.json | grep '"enabled"'
```
If `false`, no notifications will post. Set to `true` and try again.

**Check 2: Is the event type enabled?**
```bash
cat .squad/identity/teams-config.json | grep -A 10 '"notify_on"'
```
The specific event type must be set to `true`. For example, if `work_complete` is `false`, work completion notifications won't post.

**Check 3: Is Azure CLI logged in?**
```bash
az account show
```
If you get a "not logged in" error, authenticate:
```bash
az login
```

### Graph API 403 Error

**Symptom:** Script exits with "403 Forbidden"

**Cause:** Azure account lacks permission to post to the Teams channel.

**Fix:**
1. Ensure your Azure account has **Team Owner** or **Team Member** permissions for the group
2. May need to grant consent for the Microsoft Graph API permission `TeamworkActivitySend` — contact your Teams admin if needed
3. Re-authenticate: `az logout && az login`

### Channel ID Wrong

**Symptom:** "Invalid channel ID" or "Channel not found" error

**Fix:**
1. Re-extract the channel ID from Teams (right-click channel → Copy link)
2. Verify the `@thread.tacv2` format is preserved
3. Test with `az rest` directly:
   ```bash
   az rest --method post \
     --url "https://graph.microsoft.com/v1.0/teams/{group_id}/channels/{channel_id}/messages" \
     --body '{"body":{"content":"test"}}'
   ```
   If this works, the IDs are correct.

### Script Not Found

**Symptom:** `command not found: .squad/scripts/teams-notify.sh`

**Fix:**
- Ensure the script exists: `ls -la .squad/scripts/teams-notify.sh`
- Make it executable: `chmod +x .squad/scripts/teams-notify.sh`
- Call from the repository root: `cd $TEAM_ROOT && .squad/scripts/teams-notify.sh ...`

---

## Graceful Degradation

**The notification system is entirely optional.** If Teams integration is not configured or disabled:

- Squad workflows **continue normally** — no errors, no retries
- The script exits cleanly (`enabled: false` → skip → exit 0)
- Warnings are logged to stderr when something goes wrong (e.g., missing config, API failure), but the exit code is always 0
- No impact on development velocity or CI/CD pipelines
- Can be enabled later without code changes — just edit `.squad/identity/teams-config.json`

Teams notifications are a **team communication convenience**, not a blocker. The squad remains fully functional without them.

---

## Configuration Checklist

Before using Teams notifications:

- [ ] **Teams account:** You have access to the "Waza Squad" Teams channel
- [ ] **Azure CLI:** Installed and authenticated (`az login`)
- [ ] **Group ID:** Extracted from Teams organization settings
- [ ] **Channel ID:** Extracted from Teams channel link
- [ ] **Config file:** `.squad/identity/teams-config.json` created with correct IDs
- [ ] **Script:** `.squad/scripts/teams-notify.sh` exists and is executable
- [ ] **Permissions:** Azure account is Team Owner or Team Member for the group

Once complete, run a test:
```bash
.squad/scripts/teams-notify.sh work_complete "Test notification from squad"
```

Check the Teams channel — the message should appear within 2–3 seconds.

---

## Local Watchdog — `ralph-watch.sh`

For continuous monitoring of repo activity, use the local watchdog script. It runs on your machine, polls GitHub every N minutes via `gh` CLI, and posts new events to Teams automatically.

### Quick Start

```bash
# Default: poll every 10 minutes
.squad/scripts/ralph-watch.sh

# Poll every 5 minutes
.squad/scripts/ralph-watch.sh --interval 5

# Poll every 30 minutes (low-frequency monitoring)
.squad/scripts/ralph-watch.sh --interval 30
```

### What It Watches

| Activity | Event Type | Notification |
|----------|-----------|--------------|
| PR merged to main | `pr_merged` | PR number, title, author |
| Issue closed | `issue_closed` | Issue number and title |
| New commits on main | `work_complete` | Commit count and authors |

### How It Works

1. Polls GitHub using `gh` CLI on each cycle
2. Compares against state in `.squad/identity/.ralph-watch-state.json`
3. Calls `teams-notify.sh` for each genuinely new event
4. Updates state file to avoid duplicate notifications

**First run** initializes state with current activity — no flood of old notifications.

**Ctrl+C** triggers a graceful shutdown with a summary of what was reported.

### Requirements

- `gh` CLI (authenticated)
- `jq`
- `teams-notify.sh` (in the same scripts directory)

---

## Related Files

- `.squad/identity/teams-config.json` — Configuration (group ID, channel ID, event toggles)
- `.squad/scripts/teams-notify.sh` — Notification script (reads config, posts to Graph API)
- `.squad/scripts/ralph-watch.sh` — Local watchdog (polls GitHub, calls teams-notify.sh)
- `.squad/identity/.ralph-watch-state.json` — Watchdog state (auto-created on first run)
- `.squad/decisions.md` — Team decisions (includes Teams integration decision)
- `.squad/agents/scribe/charter.md` — Scribe role (calls notification script after work)

---

## See Also

- [Microsoft Graph Teams API Docs](https://learn.microsoft.com/en-us/graph/api/channel-post-messages)
- [Azure CLI `az rest` Command](https://learn.microsoft.com/en-us/cli/azure/reference-index#az-rest)
- Squad Charter: `.squad/agents/{role}/charter.md`
