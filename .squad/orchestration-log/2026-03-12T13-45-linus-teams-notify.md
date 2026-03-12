# Orchestration: Linus — Teams Notification System
**Date:** 2026-03-12T13:45:00Z  
**Agent:** Linus (Backend Developer)  
**Mode:** background  
**Outcome:** ✅ success

## Work Summary

Created the Teams notification system for squad milestone updates via Microsoft Graph API.

## Artifacts

### Code
- **`.squad/scripts/teams-notify.sh`** — Main notification script
  - Sends messages to Teams channel via Microsoft Graph API (`az rest`)
  - Supports multiple event types: `work_complete`, `pr_opened`, `pr_merged`, `issue_closed`, `decisions`
  - Event filtering driven by `.squad/identity/teams-config.json`
  - Always exits 0 — notification failures never block automation
  - Robust: Falls back from jq to grep/sed for JSON parsing
  - HTML content escaping for security

- **`.squad/scripts/teams-test.sh`** — Test/verification companion
  - Standalone test of notification delivery to Teams
  - Verified notification appeared in Waza Squad channel

## Design Decisions

1. **Always exit 0** — Notification script handles all errors gracefully. If az isn't installed, not logged in, config missing, or API fails, it logs warning and exits cleanly.

2. **jq with grep/sed fallback** — JSON parsing prefers jq (proper escaping) but falls back to grep/sed for minimal environments.

3. **HTML content escaping** — Message content escaped (`&`, `<`, `>`, `"`) before embedding in Teams HTML.

4. **Config-driven event filtering** — Each event type independently controlled via `notify_on` object in teams-config.json.

5. **TEAM_ROOT auto-detection** — Script walks up from location to find repo root. Can be overridden via env var.

## Verification

- Test notification sent via `teams-test.sh`
- Message successfully delivered to "Waza Squad" Teams channel
- No blocking errors observed

## Next Steps

- Scribe will merge decisions and call notifications after logging
- Livingston to document skill
- Coordinator to verify integration completeness
