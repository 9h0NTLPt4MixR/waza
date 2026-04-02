# Decision: Merge Dashboard and Queue into unified page

**By:** Rusty (Lead / Architect)
**Date:** 2026-02-20

**What:** Merged the Dashboard (`/` route) and Run Queue (`/runs/queue` route) into a single unified page at `/`. The standalone Queue page is removed from navigation and routing. The `/runs/queue` hash now redirects to `/`.

**Design:**
- Top section: KPI summary cards (unchanged)
- Main section: Unified runs table combining queue items and completed results
- Columns: Status (emoji badge), Spec, Model (with judge indicator), Pass Rate, Tasks, Duration, When, Actions
- Status badges: 🟡 Queued, 🔵 Running, ✅ Complete, ❌ Failed, ⚪ Cancelled
- Running rows have subtle pulse animation
- Click behavior: Complete → RunDetail, Active → RunStatus
- Actions: Results + Re-Run (completed/failed), Cancel (queued/running)
- Auto-refresh via queue polling when active runs exist
- Client-side merge of `/api/runs`, `/api/results`, and `/api/runs/queue` — queue takes precedence for dedup

**Navigation:** Runs | Compare | Trends | Live | New Run | Settings (Queue removed)

**Why:** Users were confused by two separate pages showing different slices of the same data. A unified view gives a single source of truth for all run states. RunStatus and RunDetail pages remain unchanged for drill-down.

**Files changed:**
- `web/src/components/Dashboard.tsx` — Rewrote to include unified table with queue data merge
- `web/src/components/Layout.tsx` — Removed Queue nav item
- `web/src/App.tsx` — Removed RunQueue route, added redirect
- `web/src/components/RunStatus.tsx` — Updated "View Queue" → "All Runs" pointing to `/`
- `web/src/index.css` — Added `animate-pulse-subtle` keyframe for running rows
- `web/e2e/helpers/api-mock.ts` — Added `/api/runs/queue` mock
- `web/e2e/dashboard.spec.ts` — Updated badge test for unified StatusBadge
- `web/e2e/judge-model.spec.ts` — Updated to use row-scoped selectors
- `web/e2e/weighted-scores.spec.ts` — Updated for unified table columns

**Impact:** `RunQueue.tsx` is now orphaned (no imports). It can be deleted or kept as reference. The `useRunQueue` hook and `fetchRunQueue` API client function are still used by the unified Dashboard.
