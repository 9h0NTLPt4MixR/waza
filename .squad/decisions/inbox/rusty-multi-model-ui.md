# Decision: Multi-Model Trigger Payload Shape

**By:** Rusty (Lead / Architect)
**Date:** 2026-07-26

**What:** When the New Run page sends a trigger request, it now sends both `model` (string, primary/first model) and `models` (string array, all selected models). The backend response shape is `TriggerRunResponse { runId?, batchId?, runIds? }` — single runs return `runId`, batch runs return `batchId` + `runIds`.

**Why:** Backward compatibility. Existing backend handlers that read `config.model` continue to work unchanged. New batch-aware handlers read `config.models`. The response union avoids breaking the single-run flow while supporting batch navigation (multi-model → dashboard, single → run status page).

**Impact:** Backend needs to handle the `models` field on `POST /api/runs/trigger`. If `models` has >1 entry, create parallel runs and return `{ batchId, runIds }`. If `models` has 1 entry (or is absent), fall back to single-run behavior returning `{ runId }`.
