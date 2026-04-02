# Decision: Real-time log tailing is ADC-only

**Author:** Linus (Backend Developer)
**Date:** 2026-02-20

## Context
We added a `LogTail` field to `RunRequest` that captures the last 30 lines of waza output during ADC eval runs. This is populated by tailing `/workspace/waza.log` inside the sandbox on each 15-second poll cycle.

## Decision
- **ADC mode only.** Local subprocess mode captures stdout/stderr in memory buffers but doesn't write to a file — there's nothing to tail. The `LogTail` field stays empty for local runs.
- **Best-effort updates.** If the log tail read or the Cosmos update fails, we log a warning and move on. Log tailing must never block or fail the eval.
- **Last 30 lines.** Enough context for the frontend to show progress without storing megabytes of log per poll cycle.

## Implications
- Frontend should treat `logTail` as optional — it may be empty for local runs or early in an ADC run before waza starts writing output.
- The field is `omitempty` in JSON, so it won't appear at all when empty.
