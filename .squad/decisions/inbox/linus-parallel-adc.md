# Decision: Parallel ADC Workers — Task Sharding via Eval YAML Modification

**Author:** Linus (Backend Developer)
**Date:** 2026-04-02
**Status:** Implemented

## Context

The platform needed to support `workers: N` in eval configs for multi-sandbox parallelism. Three approaches were considered:

1. **`--shard` CLI flag** — Would require adding a new flag to waza CLI
2. **`--task` filter per sandbox** — Requires knowing task names upfront
3. **Modified eval YAML per sandbox** — Write a shard-specific eval with subset of task paths

## Decision

**Option 3: Modified eval YAML per sandbox.** Each sandbox gets an eval YAML where the `tasks` field contains only its assigned task file paths (explicit paths, not globs). This avoids changes to the waza CLI and works with any eval spec.

## Task Discovery Flow

1. Clone repo in all sandboxes concurrently
2. Read eval spec from sandbox 0
3. Parse `tasks` globs, resolve via `ls` in sandbox
4. Distribute task files round-robin across workers
5. Write `shard-N-eval.yaml` to each sandbox

## Result Merging

Lightweight JSON merge: concatenate `tasks` arrays, recalculate `summary` fields, sum `usage` stats. Uses `map[string]any` for schema flexibility (handles both legacy Python and Go result formats).

## Implications

- **ADC batch API** is now used when workers > 1 — team should monitor batch creation success rates
- **No waza CLI changes needed** — the shard eval approach works with the existing CLI
- **Workers clamped to `MaxSandboxesPerUser` (10)** — matches platform design agreement
