# Decision: .agent.md files reuse SkillInfo struct

**Date:** 2026-02-23
**Author:** Linus (Backend Developer)
**Issue:** #225

## Context

Custom agent files (`.agent.md`) share the same frontmatter/body structure as `SKILL.md` with additional agent-specific fields (tools, model, handoffs, mcp-servers, agents). We needed to decide whether to create a separate `AgentInfo` type or reuse the existing `SkillInfo`.

## Decision

Reuse `SkillInfo` for agent files. `AgentFrontmatter` embeds `Frontmatter` and adds agent-specific fields, but at the workspace/orchestration level, agents are represented as `SkillInfo` with `SkillPath` pointing to the `.agent.md` file.

## Rationale

- Avoids sweeping changes to `EvalSpec`, `EvalRunner`, and the coverage system which all operate on `SkillInfo`
- SKILL.md always takes priority when both exist — agents are a fallback, not a parallel system
- Agent-specific fields are only needed at parse time (execution layer), not throughout orchestration

## Implications

- If agent-specific routing or scoring is needed later, a type assertion or wrapper may be required
- The `tokens profile` command currently only finds `SKILL.md` files — extending it to `.agent.md` is a separate task
