# Decision: Ralph Watch — Local Watchdog for Teams Notifications

**Date:** 2026-03-12
**Author:** Linus (Backend Developer)
**Status:** Implemented

## Context

Shayne requested a local process (Option 1 from heartbeat discussion) to monitor GitHub activity and post to Teams. Explicitly not GitHub Actions — "do not update the gh actions heartbeat, it does nothing for us."

## Decision

Created `ralph-watch.sh` as a local daemon that:
- Polls GitHub every N minutes (default 10) using `gh` CLI
- Tracks state in `.squad/identity/.ralph-watch-state.json` to avoid duplicate notifications
- Calls existing `teams-notify.sh` for each new event (pr_merged, issue_closed, work_complete)
- Seeds state on first run so existing activity doesn't flood Teams

## Design Choices

1. **State-file deduplication over timestamps:** Tracking known PR/issue numbers is more reliable than timestamp filtering when API results are paginated or delayed.
2. **`gh api` compare endpoint for commits:** Avoids needing local git state; the API gives commit count and authors directly.
3. **Delegates to `teams-notify.sh`:** Watchdog only detects events; formatting and posting is the notify script's job. Single responsibility.
4. **No background daemon management:** Runs in foreground with Ctrl+C shutdown. Shayne can wrap in `nohup` or `tmux` if needed.

## Impact

All squad members benefit from real-time Teams notifications about repo activity. No CI/CD changes required.
