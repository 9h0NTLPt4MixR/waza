# Decision: Auto-inject tool_constraint from .agent.md frontmatter

**Date:** 2026-02-23
**By:** Linus (Backend Developer)
**Issue:** #225

## Context

When an eval targets a custom agent (`.agent.md`) that declares `tools: [...]` in its frontmatter, users should get tool constraint validation for free — without manually adding a `tool_constraint` grader to their `eval.yaml`.

## Decision

**Injection point:** `runNormalBenchmark()` in `internal/orchestration/runner.go`, right after `validateRequiredSkills()` and before `loadTestCases()`.

**Agent path resolution:** Rather than adding a new field to `EvalSpec` or `EvalConfig`, we resolve the agent path at runtime by scanning `Config.SkillPaths` (already resolved via `utils.ResolvePaths`) for the first `.agent.md` file. This uses the new `resolveAgentPath()` helper in `agent_graders.go`.

**Why this approach over a transient EvalSpec field:**
- Zero schema changes — `EvalSpec` stays clean for YAML serialization
- The skill paths are already the source of truth for where agents live
- The resolution is cheap (one `os.ReadDir` per skill path directory)
- Keeps injection logic isolated in `agent_graders.go` instead of spreading across config/models

**Opt-out:** If the user already has a `tool_constraint` grader in their `eval.yaml`, injection is skipped entirely. This is a simple presence check on `GraderConfig.Kind`.

## Files

- `internal/skill/agent.go` — added `LoadAgentDefinition()` helper
- `internal/orchestration/agent_graders.go` — new file with `augmentGradersFromAgent()` and `resolveAgentPath()`
- `internal/orchestration/runner.go` — injection call in `runNormalBenchmark()`
- `internal/orchestration/agent_graders_test.go` — 9 test cases
