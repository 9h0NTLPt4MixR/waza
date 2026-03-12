# Orchestration: Livingston — Teams Notification Skill
**Date:** 2026-03-12T13:45:00Z  
**Agent:** Livingston (Documentation Specialist)  
**Mode:** background  
**Outcome:** ✅ success

## Work Summary

Created comprehensive skill documentation for the Teams notification system.

## Artifacts

### Documentation
- **`.squad/skills/teams-notify/SKILL.md`** — Full Teams notification skill
  - Explains purpose and usage of notification system
  - Documents all event types and when to trigger them
  - Shows example calls and expected Teams message format
  - References configuration at `.squad/identity/teams-config.json`
  - Provides troubleshooting section for common issues
  - Includes script exit codes and error handling notes

## Key Sections

1. **Overview** — Teams notifications via Graph API for squad milestones
2. **Event Types** — work_complete, pr_opened, pr_merged, issue_closed, decisions
3. **Usage** — How and when to call `.squad/scripts/teams-notify.sh`
4. **Configuration** — teams-config.json format and event filtering
5. **Examples** — Real example calls and message formatting
6. **Troubleshooting** — Common issues and solutions

## Integration Points

- Scribe calls this after logging other team work
- Coordinator invokes during setup or manual milestone recording
- All agents can trigger notifications for their work

## Next Steps

- Scribe will merge decisions and use this skill for notification
- Integration complete once Coordinator verifies
