# Decision: Multi-Model Batch Trigger API Design

**By:** Linus (Backend Developer)
**Date:** 2026-04-03

## What

POST /api/runs/trigger now accepts a `models` (string array) field alongside the existing `model` (string) field. When `models` is populated, the backend creates one run per model sharing a `batch_id` grouping key. A new `GET /api/runs/batch/{batchId}` endpoint returns all runs in a batch.

## Design Choices

1. **Backward compatible:** Single `model` field still works exactly as before. The `models` array takes precedence only when non-empty.
2. **BatchID is a grouping key only:** No batch-level orchestration, state machine, or failure handling. Each run dispatches independently. This keeps it simple — the frontend groups by batchId for comparison views.
3. **Batch status filters in-memory:** The store doesn't have a batch-specific query. We list user runs and filter by batchId. Acceptable for v1 since batch sizes are small (2-5 models). If batches grow or frequency increases, add a store-level `ListRunRequestsByBatch` method.
4. **Run ID includes index suffix:** Batch runs use `run-{nanos}-{i}` to guarantee uniqueness within a fast loop.

## Why

Users need to compare eval results across models. Creating runs one-at-a-time through the UI is tedious. This lets the frontend send all models in one request and get back a batch reference for the comparison view.

## Impact

- Frontend team (Mika): New `triggerBatchResponse` shape and `/api/runs/batch/{batchId}` endpoint available for the comparison view.
- DB schema: `batch_id` added to RunRequest (omitempty, no migration needed for Cosmos).
