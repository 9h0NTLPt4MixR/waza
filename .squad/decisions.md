# Team Decisions

## 2026-02-19: User directive — preserve .squad/ state

**By:** Shayne Boyer (via Copilot)

**What:** .squad/ directory and its contents must be maintained and preserved across all work. Never gitignore it. It should be committed on feature branches. The Squad Protected Branch Guard CI is the enforcement mechanism that prevents it from reaching main/preview — that's the correct design. All agents and workflows must respect this.

**Why:** User request — captured for team memory. The worktree-local strategy depends on .squad/ being tracked on feature branches so state flows through git merge. Gitignoring it would break squad state propagation.

## 2026-02-18: Model selection directive (updated)

**By:** Shayne Boyer (via Copilot)

**What:** All coding work must use Claude Opus 4.6 (premium). All code reviews must use GPT-5.3-Codex. This supersedes and consolidates the earlier review-only directive from 2026-02-18.

**Why:** User request — captured for team memory. User explicitly stated "make sure we are coding in opus 4.6 high and reviewing in Codex 5.3" and requested this be persisted so it doesn't need repeating.

## 2026-02-18: Web UI model assignments

**By:** Shayne Boyer (via Copilot)

**What:** For Web UI (#14) implementation: coding in Claude Opus 4.6 (premium), checks/reviews in GPT-5.3-Codex, design work in Gemini Pro 3 Preview

**Why:** User request — optimizing model selection per task type for this epic

## 2026-02-18: Dashboard design — DevEx colors, no gradients

**By:** Shayne Boyer (via Copilot)

**What:** Dashboard theme should use colors/styling close to the DevEx Token Efficiency Benchmarks dashboard. No fancy gradients — keep it clean and functional.

**Why:** User preference — captured for design consistency

## 2026-02-19: Screenshot spec conventions

**By:** Basher (Tester / QA)
**Issue:** #251

**What:** Screenshot tests live in `web/e2e/screenshots.spec.ts` and output to `docs/images/`. Conventions:
- Viewport: 1280×720, chromium only (no firefox — screenshots must be pixel-consistent)
- Paths: Use `../docs/images/` (relative to Playwright config root `web/`), NOT relative to the test file
- Mock data: Reuse `mockAllAPIs` and existing fixtures — no screenshot-specific mock data
- Views requiring interaction: Set up state (select options, expand rows) before capturing
- Naming: kebab-case matching the view name: `dashboard-overview.png`, `run-detail.png`, `compare.png`, `trends.png`

**Why:** Reproducible screenshots from mock data mean docs images stay consistent regardless of when/where they're generated. Running `npx playwright test e2e/screenshots.spec.ts --project=chromium` regenerates all four images deterministically.

## 2026-02-19: Documentation Maintenance Routing (Issue #256)

**By:** Saul (Documentation Lead)

**Status:** Implemented

**What:** Established Saul (Documentation Lead) as the documentation quality gate. Added two new PR review rules:
- **Doc-review gate** (Rule 9): Saul reviews PRs touching CLI code (`cmd/waza/`, `internal/`, `web/src/`) for documentation impact
- **Doc-consistency gate** (Rule 10): Saul reviews PRs touching documentation files for style consistency and accuracy

Added Documentation Impact Matrix mapping code paths to required doc updates, showing which doc files must be checked when specific code changes.

**Why:** **Problem:** Documentation was reactive rather than proactive. Code changes happened without corresponding doc updates. Screenshots became stale. Examples diverged from actual behavior. No clear responsibility for doc freshness.

**Solution:** Make documentation review a first-class routing rule, like code review. Saul owns ongoing doc-freshness verification across all PRs. The Impact Matrix provides clear guidance on what needs checking for each code path.

**Scope:**
- **routing.md:** Added Rules 9–10 and Documentation Impact Matrix
- **charter.md:** Added doc-freshness reviews to "What I Own" and PR monitoring to "How I Work"
- **AGENTS.md:** Added Documentation Maintenance section with tables for "When to Update Docs" and screenshot regeneration steps
- **history.md:** Recorded doc-freshness reviews as a key learning

**Impact:** All code PRs (`cmd/waza/`, `internal/`, `web/src/`) now automatically routed to Saul for doc-impact review. All doc PRs (`docs/`, `README.md`, `DEMO-SCRIPT.md`) routed to Saul for consistency check. Clear accountability: Saul owns the matrix and updates it as new paths are discovered. Screenshot maintenance can be automated via Playwright tests.

## 2026-02-19: --tokenizer flag should be available on all token commands

**By:** Rusty (Lead / Architect)  
**PR:** #260  
**Date:** 2026-02-19

**What:** The `--tokenizer` flag is currently only on `waza tokens count`. The `check`, `compare`, and `suggest` commands hardcode `TokenizerDefault`. For consistency, all token commands should accept `--tokenizer` so users can choose between BPE and estimate across the board.

**Why:** If a user needs the fast estimate for CI (where speed matters more than precision), they should be able to use it from any token command — not just `count`. The current design forces BPE on `check` and `compare` with no escape hatch.

**Status:** Follow-up work, not blocking PR #260.

## 2026-02-20: Unified Release Trigger & Version Single Source-of-Truth

**By:** Rusty (Lead / Architect)
**Date:** 2026-02-20
**Status:** PROPOSED
**Impact:** Release process, artifact consistency, extension users

**What:** Unify the release process under a single `release.yml` workflow triggered by `v*.*.*` Git tags. Retire `go-release.yml` and `azd-ext-release.yml` once stable. Pre-flight validation ensures `version.txt == tag`. Version sync runs before builds, not after.

**Why:** Current two-workflow approach causes version desync (extension.yaml lags CLI), stale registry.json, dual tag schemes, and no validation. Tag-driven approach is Git-native, immutable, auditable.

**See Also:** Issue #223, `.squad/agents/rusty/history.md`

## 2026-02-20: Model assignment overhaul — quality-first policy

**By:** Scott Boyer (via Copilot)
**Date:** 2026-02-20
**Status:** APPROVED

**What:** Full model reassignment — cost is not a constraint, optimize for quality/speed per role:
1. Rusty (Lead) → `claude-opus-4.6` — always premium, no downgrade for triage
2. Linus (Backend Dev) → `claude-opus-4.6` — highest SWE-bench (81%), best debugging
3. Basher (Frontend Dev) → `claude-opus-4.6` — same quality advantage for components
4. Livingston (Tester) → `claude-opus-4.6` — best logical reasoning for edge cases
5. Saul (Documentation Lead) → `gemini-3-pro-preview` — 1M context, good for large docs
6. Scribe (Session Logger) → `gemini-3-pro-preview` — mechanical ops, Gemini handles fine
7. Diversity reviews → `gemini-3-pro-preview` — different provider = different perspective
8. Heavy code gen (500+ lines) → `gpt-5.2-codex` — 3.8× faster, 400K context

**Why:** User directive: "Cost is not an issue — optimize for best/fastest per role." Benchmarks consulted: SWE-bench Verified (Feb 2026). Claude Opus 4.6 leads at 81%, GPT-5.2 Codex wins speed, Gemini 3 Pro wins context window + provider diversity.

**Supersedes:** "Model selection directive (updated)" from 2026-02-18 and "Web UI model assignments" from 2026-02-18.

## 2026-02-21: User directive — MCP always on

**By:** Shayne Boyer (via Copilot)

**What:** MCP server always launches with `waza serve` — no --mcp flag needed. It's always on, supporting all features.

**Why:** User request — simplify the CLI surface, MCP is a core feature not an opt-in

## 2026-02-21: User directive — Waza skill should orchestrate workflows

**By:** Shayne Boyer (via Copilot)

**What:** The waza interactive skill (#288) should support scenarios and orchestrate user workflows — not just be a reference doc. It should guide users through creating evals, running them, interpreting results, comparing models, etc.

**Why:** User request — the skill needs to be a real workflow partner, not a tool catalog

## 2026-02-21: User directive — Use Mermaid for diagrams

**By:** Shayne Boyer (via Copilot)

**What:** Use Mermaid for all markdown diagrams in documentation and design docs — no ASCII art diagrams

**Why:** User request — captured for team memory

## 2026-02-20: Grader Weighting Design

**By:** Linus (Backend Dev)
**Date:** 2026-02-20
**Issue:** #299

### What

Added optional `weight` field to grader configs for weighted composite scoring. Key design choices:

1. **Weight lives on config, not on the grader interface.** Graders don't know their own weight — the runner stamps it onto `GraderResults` after grading. This keeps grader implementations simple and weight-unaware.

2. **Default weight is 1.0** (via `EffectiveWeight()`). Zero and negative values are treated as 1.0. This means all existing eval.yaml files produce identical results — no migration needed.

3. **Weighted score is additive, not a replacement.** `AggregateScore` (unweighted) is preserved. `WeightedScore` is a new parallel field. The interpretation report only shows weighted score when it differs from unweighted.

4. **Weight flows through the full pipeline:** `GraderConfig.Weight` → `GraderResults.Weight` → `RunResult.ComputeWeightedRunScore()` → `TestStats.AvgWeightedScore` → `OutcomeDigest.WeightedScore`. Web API also carries weight per grader result.

### Why

Weighted scoring lets users express that some graders matter more than others (e.g., correctness 3× more important than style). Without breaking existing pass/fail semantics.

### Impact

- `internal/models/` — new fields on `GraderConfig`, `GraderResults`, `TestStats`, `OutcomeDigest`
- `internal/orchestration/runner.go` — weight stamping in `runGraders`, weighted stats in `computeTestStats`/`buildOutcome`
- `internal/reporting/` — conditional weighted score display
- `internal/webapi/` — weight exposed in API responses
- JSON schema unchanged (eval.yaml schema is separate from waza-config.schema.json)

## 2026-02-20: SpecScorer as separate scorer from HeuristicScorer

**By:** Linus (Backend Developer)
**Date:** 2026-02-20
**PR:** #322
**Issue:** #314

**What:** The agentskills.io spec compliance checks are implemented as a separate `SpecScorer` type rather than extending `HeuristicScorer`. Both run independently — `HeuristicScorer` handles heuristic quality scoring (triggers, anti-triggers, routing clarity) while `SpecScorer` handles formal spec validation (field presence, naming rules, length limits).

**Why:** The two scorers have different concerns: `HeuristicScorer` is about quality/adherence level (Low→High), while `SpecScorer` is about pass/fail conformance to the agentskills.io specification. Keeping them separate means each can evolve independently. The spec may change without affecting heuristic scoring, and vice versa.

**Impact:** Both `waza dev` and `waza check` now run both scorers. Any new spec checks should be added to `SpecScorer` in `cmd/waza/dev/spec.go`. The `SpecResult.Passed()` method only considers errors (not warnings) — warnings like missing license/version don't block readiness.

## 2026-02-21: Releases page pattern

**By:** Saul (Documentation Lead)
**Date:** 2026-02-21
**Issue:** #383
**PR:** #384

**What:** Created a releases reference page at `site/src/content/docs/reference/releases.mdx` that shows the current release (v0.8.0) with changelog highlights, download table, install commands, and azd extension info. Older releases link out to GitHub Releases rather than duplicating content.

**Why:** The docs site should be a self-contained starting point for users downloading waza. Having binaries, install commands, and changelog highlights in one place reduces friction. Linking to GitHub Releases for history avoids maintaining two changelog surfaces.

**Pattern for future releases:** When cutting a new version, update the releases.mdx page — change the version number, update the changelog highlights, and update download URLs. The CHANGELOG.md remains the source of truth; the releases page is a curated summary of the latest.

## 2026-02-26: Performance Audit — 30 findings from dual-model assessment

**By:** Turk (Go Performance Specialist)
**Requested by:** Shayne Boyer
**Date:** 2026-02-26
**Scope:** runner, cmd_run, execution, jsonrpc, webapi, tokens, graders, links
**Status:** INFORMATIONAL + PRIORITIZED REMEDIATION

### What

Full independent performance audit of waza Go codebase across two models:
- **GPT-5.3-Codex pass:** 28 findings (3 P0, 9 P1, 16 P2)
- **Claude Opus 4.6 pass:** 23 findings (3 P0, 9 P1, 11 P2)
- **Coordinator synthesis:** 30 total unique findings (19 overlapping, 7 Codex-only, 4 Opus-only)

### Critical P0 Findings

1. **O(N²) stop-on-error scan** (runner.go:658–671): Sequential test iteration re-scans all previous outcomes. **Fix:** Track `hadFailure` flag instead.
2. **Grader instances recreated per run** (runner.go:1167–1233): `runGraders()` calls `graders.Create()` for every grader on every run. 10 tasks × 3 runs × 4 graders = 120 factory calls, each loading `.waza.yaml`. **Fix:** Create instances once during test init, reuse.
3. **Resource files read into memory per run** (runner.go:1081–1144): `loadResources()` reads fixture files as strings on every run. 1MB × 10 tests × 3 runs = 30MB redundant read. **Fix:** Cache across runs or use direct file copy.
4. **No signal propagation on long-running evals** (cmd_run.go:607): `runSingleModel` uses `context.Background()` — Ctrl+C never reaches engine/graders. **Fix:** Wire `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.

### Important P1 Findings

- Inline script grader creates temp file per invocation; write once in constructor
- Program grader loads `.waza.yaml` on every construction; accept timeout as parameter
- FileStore recomputes summaries on every API call; cache on load
- Goldmark parser recreated per markdown file; use single shared instance
- JSON transport allocates per message; use `json.Encoder` with buffer
- BPE tokenizer regex applied char-by-char with repeated substring slicing
- HTTP response bodies not drained, blocking connection reuse

### Nice-to-Have P2 Findings (11 total)

Cache mutex should be RWMutex, token limits reads files twice, session runs never evicted, spinner uses `time.After` in select loop, repeated JSON marshaling for cache keys, context ignored in `Shutdown()`, and others.

### Remediation Priority

1. **Concurrency/cancellation first:** Fix lifecycle leaks, add `ctx.Done()` checks before semaphore acquisition, wire signal context
2. **High-frequency I/O second:** Remove repeated config loads, reduce lock-held disk I/O, revisit workspace lifecycle
3. **CPU/allocation micro-optimizations third:** Tokenizer hot loops, pretty JSON on machine paths, HTTP link checker reuse

### Why

Top issues are multiplicative with benchmark scale (tasks × runs × graders). Correctness-oriented fixes prevent wasted work and leaks. Grader/store I/O reductions deliver largest immediate wall-clock gains. Micro-optimizations follow after control-path fixes.

### Session Log

See `.squad/log/2026-02-26-performance-audit.md` for detailed session notes.

## 2026-03-05: Token Diff Distribution Strategy (Issue #81)

**By:** Rusty (Lead / Architect)  
**Issue:** #81  
**Status:** APPROVED

### What

For the GitHub Action token budget PR comment feature (#81), **implement `waza tokens diff` CLI command + lightweight wrapper action**, not action-only or CLI-only.

### Implementation

1. **CLI command:** `waza tokens diff [ref1] [ref2] [--format json|table] [--strict]`
   - Compares token counts between git refs (default: origin/main to HEAD)
   - Outputs JSON or formatted table
   - Exit code 1 if limits exceeded and --strict is set
   - Works everywhere: GitHub, GitLab, Azure Repos, local CI

2. **Wrapper action:** `.github/actions/token-diff/action.yml` (thin, ~20 lines)
   - Calls `waza tokens diff`, posts PR comment
   - For GitHub users who want automation without custom workflow logic
   - Optional convenience layer, not required

### Why This Over Alternatives

**Rejected: Action-only (#1)** — Vendor lock-in to GitHub; semantically wrong (action is infrastructure, token-diff is product).

**Rejected: CLI-only (#2)** — Ignores GitHub users who want PR comment automation without custom workflow YAML.

**Rejected: azd extension (#3)** — Redundant wrapper; doesn't solve the underlying problem.

**Rejected: Template (#4)** — Doesn't scale; requires manual sync; no centralized maintenance.

**Chosen: Combination (#5)** — Splits concerns cleanly: domain logic (diff) in CLI, GitHub automation (posting) in action. Serves all audiences (binary, azd, GitHub). No vendor lock-in. Semantically correct. Low maintenance.

### User Choice

- **GitHub workflow (simple):** `uses: microsoft/waza/.github/actions/token-diff@main`
- **Custom workflow:** Call `waza tokens diff` directly, parse JSON
- **Non-GitHub CI:** Use `waza tokens diff` CLI directly
- **azd users:** `azd waza tokens diff` (auto-wrapped)

### Reasoning

Distribution should serve the user, not reverse-engineer infrastructure. The action is infrastructure; token tooling is product. Keeping token-diff in the CLI makes it independently useful (other tools can integrate, no GitHub lock-in). The action is optional, thin, and can be maintained with minimal effort. This avoids the semantic inversion of "GitHub Actions are user-facing product distribution mechanisms."

### Impact

- Code: Add `diff.go` (~100-150 lines, reuse `compare` logic)
- Tests: Unit tests for CLI, e2e test for action
- Docs: CLI reference, usage guide
- Maintenance: Single source of truth (CLI), thin wrapper (action)

## 2026-03-05: Model + Workflow Directive — Code in Codex, Verify in Opus, Use Worktrees

**By:** Shayne Boyer (via Copilot)
**Date:** 2026-03-05T00:36Z

**What:** 
- **Code generation:** Use GPT-5.3-Codex (speed + 400K context for large skills)
- **Code review/verification:** Use Claude Opus 4.6 (highest quality, best reasoning)
- **Workflow:** Use worktrees for parallel issue work across issues #80-89
- **PR gating:** All PRs must be green (CI passing) before merge

**Why:** User request — captured for team memory. Codex excels at code generation speed and large context. Opus excels at review and logical verification. Worktrees enable parallel work on multiple issues without context switching. Green CI enforces quality.

## 2026-03-05: CI Fixes — git config for temp repositories (Issue various)

**By:** Linus (Backend Dev)  
**Date:** 2026-03-05  
**PRs:** #65, #64, #55 (merge conflict and test failure fixes)

**What:** When git-history tests create temporary repositories (`cmd/waza/tokens/internal/git/git_test.go`), configure those temp repos with:
- `core.autocrlf=false`
- `core.safecrlf=false`

**Why:** On Windows, strict LF/CRLF conversion checks can fail `git add -A` inside temp repos even when test logic is correct. Setting repo-local git config in test setup removes environment-specific failures and keeps test behavior consistent across Ubuntu and Windows CI.

## 2026-03-05: CI/CD Integration Guide Structure (Issue #89)

**By:** Livingston (Documentation Specialist)  
**Date:** 2026-03-05  
**Status:** INFORMATIONAL  
**Related Issue:** #89  
**PR:** squad/89-ci-integration-guide

**What:** Expanded the CI/CD integration guide from GitHub Actions–only to a comprehensive multi-platform guide covering GitHub Actions, Azure DevOps, and GitLab CI.

**Key Decisions:**

1. **Structure: Installation → Platform Guides → Best Practices → Troubleshooting**
   - Installation first — azd extension (recommended), binary, source — reduces friction
   - Platform guides — Complete, runnable examples for each major platform
   - Best practices — 8 focused practices (filters, caching, timeouts, fail-fast, budgets, secrets, artifact retention)
   - Troubleshooting — Common issues: PATH, timeouts, token auditing, rate limits

2. **Real Patterns from the Repo** — All workflow examples based on:
   - `.github/workflows/go-ci.yml` — Multi-OS matrix pattern
   - `.github/workflows/waza-eval.yml` — Context-dir detection, input dispatch, artifact upload

3. **Token Budget as Core Pattern** — Included `waza tokens diff` command prominently (Issue #81 implementation) for PR-level token budget enforcement.

4. **MDX Tabs for Secrets Management** — Used Starlight's `<Tabs>` component for platform-specific secrets handling without duplication.

**Why:** 
1. **Visibility:** Users on different CI platforms deserve equal-quality guidance
2. **Real patterns:** All examples based on actual waza patterns from go-ci.yml and waza-eval.yml
3. **Installation upfront:** Developers often don't know how to install waza in their CI — covering this first prevents blocking issues
4. **Best practices driven by cost:** Path filtering and caching examples emphasize cost reduction (key concern in CI/CD)
5. **Platform-agnostic secrets:** Used MDX Tabs to show secrets management across three platforms without repeating boilerplate

**Impact:**
- **Docs site:** Guide is now comprehensive and covers 3 major CI/CD platforms
- **Build:** Site builds successfully, all 14 pages including ci-cd page
- **Sidebar:** No changes needed — entry already existed in astro.config.mjs
- **Users:** Single source of truth for CI/CD integration across platforms

## 2026-03-05: Batch PR Review & Issue Triage Routing

**By:** Rusty (Lead / Architect)  
**Date:** 2026-03-05T00:37Z  
**Status:** COMPLETED

**What:**

### PR Reviews (6 total)
All 6 PRs reviewed and set to auto-merge via squash:

| PR | Title | Verdict |
|----|-------|---------|
| #88 | Dependabot: svgo 4.0.0→4.0.1 | ✅ Approved + merged |
| #87 | fix: docs link to GitHub Pages | ✅ LGTM comment + auto-merge |
| #71 | chore: add MIT LICENSE | ✅ LGTM comment + auto-merge |
| #44 | fix: --discover project-root layout | ✅ Approved + auto-merge |
| #69 | fix: discover .github/skills/ | ✅ LGTM comment + auto-merge |
| #79 | feat: sensei scoring parity | ✅ LGTM comment + auto-merge |

PRs #87, #71, #69, #79 authored by spboyer — GitHub API prevents self-approval, so review comments were left confirming LGTM status.

### Issue Triage (8 issues)
All issues routed to qualified squad members:

| Issue | Title | Routed To | Rationale |
|-------|-------|-----------|-----------|
| #80 | Trigger heuristic grader (P0) | squad:linus | Go grader implementation |
| #81 | Token budget diff CLI (P1) | squad:linus | Go CLI command |
| #82 | Eval coverage grid (P1) | squad:linus | Go CLI command |
| #83 | Eval scaffolding waza eval new (P1) | squad:linus | Go CLI command |
| #84 | Multi-trial flakiness (P1) | squad:linus | Go orchestration engine |
| #85 | Snapshot auto-update (P2) | squad:linus | Go grader integration |
| #86 | Per-file token budget (P2) | squad:linus | Go config + token system |
| #89 | CI/CD integration guide (P1) | squad:livingston + squad:saul | Documentation with doc-review gate |

**Why:** Batch review + triage ensures all green PRs merge cleanly and new work is routed to the right specialist without manual coordination. Parallel work begins immediately once triage is complete.

## 2026-03-12: User directive — mixed notification formats

**By:** Shayne Boyer (via Copilot)

**What:** Teams notifications should use both HTML and Adaptive Cards depending on the update type. Pick the format that best fits each event — not everything needs to be a rich card.

**Why:** User request — captured for team memory.

## 2026-03-30: Platform Module Architecture

**By:** Rusty (Lead / Architect)  
**Date:** 2026-03-30  
**Branch:** `feature/waza-platform`  
**Status:** APPROVED

**What:** Created `internal/platform/` as the contract layer for Waza Platform with four packages:

| Package | Key Types | Implementation Target |
|---------|-----------|----------------------|
| `auth` | `AuthProvider`, `User`, `Session`, `Middleware` | GitHub OAuth App |
| `db` | `Store` (13 methods), `Connection`, `RunRequest` | Cosmos DB (serverless) |
| `api` | `RegisterRoutes`, `Dependencies` | Go stdlib `http.ServeMux` |
| `adc` | `ADCConfig`, quota constants | Azure Dev Compute |

**Key Architectural Choices:**

1. **Go stdlib router, not chi/gorilla.** The platform API uses Go 1.22+ `http.ServeMux` method patterns. We don't need middleware chaining or URL params beyond what stdlib provides. Fewer dependencies = fewer CVEs to track.

2. **User-scoped store interface.** Every `Store` method takes a `userID` parameter. No cross-user queries exist in v1. This makes single-user isolation impossible to accidentally violate — the interface enforces it.

3. **ADC quota as constants.** `MaxSandboxesPerUser = 10` is a constant, not YAML config. Changing sandbox limits is a policy decision that should go through code review, not a config file edit.

4. **map[string]any for Connection.Config.** Azure Storage needs `account_name`, `container`, `sas_token`. GitHub Repo needs `owner`, `repo`, `installation_id`. These schemas are too different for a single typed struct. The `Config` map lets each connection type define its own shape without a union type.

5. **Dependencies bundle for DI.** `api.Dependencies` struct bundles all services needed by handlers. No globals, no init() magic. Test handlers by constructing a `Dependencies` with mocks.

**What This Does NOT Include (Intentionally):**
- No implementations. These are contracts for Linus to build against.
- No team/org model. Single-user isolation per the v1 scope.
- No WebSocket routes. Real-time updates are a v2 concern (dashboard polling is fine for v1).
- No rate limiting types. Add when we have traffic data to calibrate against.

## 2026-03-30: Platform Frontend — Wave 2 React SPA

**By:** Rusty (Lead / Architect)  
**Date:** 2026-03-30  
**Branch:** `feature/waza-platform`

**What:** Implemented Wave 2 platform frontend: auth gate, auth context, Settings page (connections management), New Run page (multi-step wizard), updated navigation with user menu, and full API client extensions for platform endpoints.

**Key Decisions:**

### Auth pattern: Context + Gate, not React Query
Auth state lives in a standalone `AuthContext` using `useState` + `useEffect`, not React Query. Auth is foundational infrastructure — it must resolve before any React Query data fetches happen. The `AuthGate` component wraps the entire app and short-circuits to a login page on 401.

### Login flow: Server-side OAuth redirect
"Sign in with GitHub" is a plain `<a href="/api/auth/github">` — no client-side OAuth dance. The Go backend handles the full OAuth flow and sets a session cookie. This keeps the SPA stateless regarding tokens.

### Settings: Tab-based, Connections-first
Settings page uses a simple tab UI (Connections / Preferences). Connections tab is fully functional (CRUD + test). Preferences tab is a placeholder with local-only state — server-side persistence deferred to a future wave.

### New Run: 4-step wizard
New Run uses a step-by-step form (Source → Eval → Configure → Review & Run). After triggering, redirects to Live view with the run ID. This prevents misconfiguration and gives a clear review step.

### E2e test compatibility
Updated `mockAllAPIs` and `mockEmptyAPIs` in e2e helpers to mock `/api/auth/me` returning a test user. This ensures all 52 existing tests pass through the auth gate without changes to individual test files.

### Navigation structure
- "New Run" button (blue, prominent) in header right section
- Settings gear icon in header right section
- User avatar + dropdown menu (Settings, Sign out) in header right section
- Existing nav links unchanged on the left

**Files Created:**
- `web/src/contexts/AuthContext.tsx` — AuthProvider + useAuth hook
- `web/src/components/AuthGate.tsx` — Login page + loading screen + auth gate
- `web/src/components/Settings.tsx` — Full settings page with connections CRUD
- `web/src/components/NewRun.tsx` — 4-step run creation wizard

**Files Modified:**
- `web/src/api/client.ts` — 10 new API functions + 7 new types
- `web/src/hooks/useApi.ts` — 9 new React Query hooks (queries + mutations)
- `web/src/components/Layout.tsx` — User menu, New Run button, Settings link
- `web/src/App.tsx` — 2 new routes (`#/settings`, `#/runs/new`)
- `web/src/main.tsx` — AuthProvider + AuthGate wrapping
- `web/e2e/helpers/api-mock.ts` — Auth mock for test compatibility

## 2026-03-30: Platform Backend Implementation Patterns

**By:** Linus (Backend Developer)  
**Date:** 2026-03-30  
**Branch:** `feature/waza-platform`

**Context:** Implemented the concrete backend pieces (auth, db, adc engine) against Rusty's interface contracts.

**Decisions:**

### 1. ADCConfig defined inline in projectconfig (not imported from adc package)

**What:** The `ADCConfig` struct is defined directly in `internal/projectconfig/config.go` rather than imported from `internal/platform/adc/`.

**Why:** The `adc` package's `engine.go` depends on `github.com/coreai-microsoft/adc-sdk-go` which isn't in go.mod yet. Importing the adc package from projectconfig would prevent the entire projectconfig package (and everything that depends on it) from compiling. All other config types (PathsConfig, DefaultsConfig, etc.) are also defined inline in projectconfig — this is consistent.

**Impact:** If the canonical `ADCConfig` in `internal/platform/adc/config.go` changes field names or YAML tags, the mirrored type in projectconfig must be updated to match.

### 2. In-memory session revocation for GitHubProvider

**What:** `RevokeSession` uses a simple in-memory `map[string]struct{}` to track revoked tokens.

**Why:** Good enough for v1 single-instance deployment on Azure Container Apps. For multi-instance or production scale, this should be replaced with a shared store (Redis, Cosmos DB sessions container, etc.).

### 3. JWT implementation is hand-rolled (not using golang-jwt/v5)

**What:** The HMAC-SHA256 JWT creation/validation in `github.go` is a minimal hand-rolled implementation rather than using the `golang-jwt/jwt/v5` library that's already in go.sum.

**Why:** The JWT payload is minimal (sub, login, iat, exp) and the signing algorithm is fixed (HS256). A library adds unnecessary abstraction for 30 lines of code. If we need RS256, JWK rotation, or complex claims validation later, we should switch to `golang-jwt/v5`.

### 4. Cosmos DB partition key is GitHub ID as string

**What:** All Cosmos DB containers use the user's GitHub ID (as a string) as the partition key.

**Why:** Aligns with Rusty's interface where `User.GitHubID` is the primary identity. Using string format for the partition key is required by Cosmos DB.

## 2026-03-30: Platform API Handlers & Server Mode

**By:** Linus (Backend Developer)  
**Date:** 2026-03-30  
**Branch:** `feature/waza-platform`

**What:** Implemented the full HTTP API handler layer, platform Dependencies struct, `--platform` serve mode, and Azure deployment infrastructure:

1. **`internal/platform/api/handlers.go`** — 14 handler implementations covering auth (me, logout), connections (CRUD + test), runs (trigger, queue, get, cancel), repos (list, list evals). All handlers extract user from context, return structured JSON errors, and are user-scoped.

2. **`internal/platform/api/deps.go`** — `Dependencies` struct bundling `Store`, `Auth`, `AuthMiddleware`, and optional `ADCDispatcher` interface. Uses an interface for ADC to avoid pulling the ADC SDK (not yet in go.mod).

3. **`cmd/waza/cmd_serve.go`** — Added `--platform` flag. Platform mode initializes Cosmos DB, GitHub OAuth, and optional ADC engine from environment variables, binds to `0.0.0.0`, skips browser auto-open, and adds `/healthz` for container probes.

4. **Azure deployment** — `azure.yaml` (azd manifest), `Dockerfile.platform` (multi-stage Go + SPA build), `infra/main.bicep` (Container Apps + Cosmos DB serverless + Key Vault + Managed Identity + role assignments), `infra/main.parameters.json`.

**Key Decisions:**

- **ADCDispatcher interface instead of direct `*adc.Engine`**: The ADC SDK isn't in go.mod yet. Using an interface in deps.go means the API package compiles cleanly. When the SDK lands, swap `ADCDispatcher` to include `Execute` and `Shutdown`.

- **Environment variables for platform config**: Platform mode reads all secrets from env vars (`COSMOS_ENDPOINT`, `COSMOS_KEY`, `GITHUB_CLIENT_ID`, etc.) rather than .waza.yaml. This aligns with 12-factor and Key Vault reference injection in Container Apps.

- **Connection test is probe-based**: `handleTestConnection` does a lightweight HTTP probe (list 1 blob for Azure Storage, GET repo for GitHub) rather than importing the full Azure SDK. This keeps the binary lean.

- **Run dispatch is async goroutine**: `handleTriggerRun` creates the RunRequest in DB synchronously, then fires a goroutine for ADC dispatch. The client gets 202 immediately. ADC integration is stubbed until the SDK lands.

- **Cosmos DB serverless**: The Bicep uses serverless capacity mode — no throughput provisioning needed, pay-per-request. Appropriate for platform v1.

**Impact:**
- All 13 platform API tests pass (including newly un-skipped CRUD, trigger, cancel, and user isolation tests)
- Full binary builds clean
- Existing `waza serve` behavior unchanged (platform mode is opt-in via `--platform`)

## 2026-03-30: Linus — PR conflict resolution decision

**By:** Linus (Backend Developer)  
**Date:** 2026-03-30

**What:** For PR conflict fixes against `main`, use an isolated worktree and merge `origin/main` into the PR branch instead of rebasing when the branch is already published. When `main` already contains newer token-limit warning behavior and fixture coverage, keep the newer `main` implementation and preserve branch-specific intent with targeted docs/examples or assertions rather than reintroducing older warning text.

**Why:** Avoids force-push conflicts on published branches and preserves the newest, best-tested code from main.

## 2026-03-30: Linus — PR #96 token limits precedence

**By:** Linus (Backend Developer)  
**Date:** 2026-03-30

**What:** Treat any non-empty `.waza.yaml` `tokens.limits` config as authoritative over legacy `.token-limits.json`, including overrides-only configs where `defaults` is omitted. The regression test covers the exact case with overrides-only YAML plus a legacy file present so future refactors don't accidentally reintroduce the fallback.

**Why:** Ensures `.waza.yaml` is always authoritative and prevents subtle migration bugs.

## 2026-03-31: User directive — Cosmos DB is primary results store

**By:** Shayne Boyer (via Copilot)  
**Date:** 2026-03-31

**What:** Cosmos DB is the PRIMARY results store and is always used. Azure Storage (BYOS) is optional/secondary storage. When a user triggers a run:

1. If no Azure Storage connection exists, store results directly in Cosmos DB (in the run-requests container or dedicated results container)
2. If both Cosmos and a storage connection exist, show a dropdown on the New Run page allowing the user to choose destination
3. Cosmos should be the default storage option

Results should never be lost due to missing storage configuration.

**Why:** User request — captured for team memory. Ensures data persistence and reduces friction for users without external storage configured.
