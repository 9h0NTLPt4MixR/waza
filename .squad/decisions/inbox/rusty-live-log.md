# Decision: LogPanel auto-scroll UX pattern

**By:** Rusty (Lead/Architect)
**Date:** 2026-07

## What

The `LogPanel` component auto-scrolls to bottom only when the user is already within 24px of the bottom. If the user scrolls up to read earlier output, auto-scroll pauses and a "Bottom" button appears. This is the standard terminal UX pattern (same as VS Code terminal, iTerm, etc.).

## Why

Forcibly scrolling to bottom on every poll would make it impossible to read earlier output during a running eval. The 24px threshold avoids false negatives from sub-pixel rendering differences.

## Impact

Any future streaming/log components should follow this same pattern. The `LogPanel` component is reusable and could be extracted to a shared component if needed elsewhere.
