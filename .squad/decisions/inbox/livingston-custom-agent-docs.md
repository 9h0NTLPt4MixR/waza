# Decision: Custom Agent Documentation Structure

**Date:** 2026-03-XX  
**Author:** Livingston (Documentation Specialist)  
**Issue:** #225 (Document custom agent eval support)  

## What Was Decided

1. **Create dedicated guide** at `site/src/content/docs/guides/custom-agents.mdx` rather than scattering content across existing guides.
2. **Cross-link strategically** from eval-yaml, graders, and CLI reference to the new guide rather than duplicating explanations.
3. **Highlight auto-injection behavior prominently** in the graders guide with a caution callout (not buried in the custom-agents guide).
4. **Document SKILL.md priority explicitly** so users understand precedence when both files exist.

## Why

- **Single source of truth:** Centralizing custom agent content in one guide makes it easier to maintain and keeps it discoverable.
- **Clear call-to-action:** The new guide entry in the sidebar makes it obvious that custom agents are supported.
- **Cross-link reduces repetition:** Linking from eval-yaml, graders, and CLI reference prevents doc duplication and creates natural discovery paths.
- **Callout placement:** Users learning about graders should immediately know that agents trigger auto-injection—don't make them hunt in another guide.

## What This Means for Future Work

- When new agent features ship (e.g., handoffs in P2), update **only** `custom-agents.mdx` and the "Limitations & roadmap" section.
- If grader behavior changes, update both `graders.mdx` and the auto-injection section in `custom-agents.mdx`.
- All agent examples should live in `examples/custom-agent/` (maintained by Linus) and referenced from the guide.

## Open Questions for the Team

- Should we add a CLI command like `waza new agent <name>` to scaffold custom agents? (Decision deferred—awaiting feature request)
- Should agent templates be part of `waza init`? (Decision deferred—blocked on template system, see #TBD)
