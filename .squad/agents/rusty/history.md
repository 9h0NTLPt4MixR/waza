# History — Rusty

## Project Context
- **Project:** waza — CLI tool for evaluating Agent Skills
- **Stack:** Go (primary), React 19 + Tailwind CSS v4 (web UI)
- **User:** Shayne Boyer (spboyer)
- **Repo:** microsoft/waza
- **Universe:** The Usual Suspects

## Key Learnings

### Architecture
- **Model selection directive (2026-02-18):** Coding in Claude Opus 4.6, reviews in GPT-5.3-Codex, design in Gemini Pro 3
- **Web UI styling:** Keep clean and functional — colors close to DevEx dashboard, no fancy gradients
- **Agent execution:** Go engine drives CLI, web UI for visualization

### Code Quality
- Test coverage is non-negotiable
- Interface-based design for flexibility (AgentEngine, Validator patterns in Go)
- Functional options for configuration (Go convention)

### Team Structure
- Linus owns Go backend implementation
- Basher owns all testing strategy
- Livingston/Saul own documentation
- Richard Park available for Copilot SDK questions

## Work Log

### 2026-02-19: #80 — BPE Tokenizer (PR #260)
- **Reviewed PR** by Charles Lowell (chlowell) — ported BPE tokenizer from Microsoft/Tokenizer
- **Architecture:** `Counter` interface preserved. `NewCounter(tokenizer)` factory replaces `NewEstimatingCounter()`. BPE is new default via `TokenizerDefault`.
- **New package:** `internal/tokens/bpe/` — BinaryMap, LRU cache, byte-pair encoder, tokenizer, builder
- **Embedded model:** `o200k_base.tiktoken` (~3.6MB) via `go:embed` — adds to binary size
- **Flag design:** `--tokenizer` flag only on `count` command; `check`/`compare`/`suggest` hardcode `TokenizerDefault`
- **Findings:** `regex` field on Tokenizer struct is dead code (set but never read); `NewTokenizerFromFile` is dead code (defined but never called); `Cache` field is exported unnecessarily; LRU cache is not thread-safe (fine for CLI but should be documented)
- **Verdict:** APPROVE with comments — architecture is sound, implementation correct, concerns are improvements not blockers

### 2025-07-25: #238 — True trajectory replay viewer (PR #243)
- **Branch:** `squad/238-trajectory-viewer`
- Full rewrite of `TrajectoryViewer.tsx` to consume real `TranscriptEvent` data
- Created `SessionDigestCard.tsx` (digest stats + tools used badges + errors)
- Created `ToolCallDetail.tsx` (expandable JSON viewers for args/result)
- Timeline: color-coded dots (blue=tool start, green/red=complete, emerald=turn, red=error)
- `toolCallId` correlation links Start ↔ Complete events
- Graceful fallback to grader-based heuristic when transcript is empty
- Depends on #237 (transcript + session digest in API)

## Learnings

### Batch PR Review & Issue Triage (Jul 2026)

**Reviewed and approved 6 PRs, triaged 8 issues.**

**PRs approved + auto-merged:**
- **#88** (Dependabot svgo bump) — approved directly, zero risk
- **#44** (--discover project-root layout fix) — approved directly, clean logic + Windows path handling in tests
- **#87** (docs link → GitHub Pages) — review comment + auto-merge (self-authored, can't self-approve)
- **#71** (MIT LICENSE) — review comment + auto-merge (self-authored)
- **#69** (.github/skills/ discovery) — review comment + auto-merge (self-authored)
- **#79** (sensei scoring parity, 777 additions) — third and final review, all must-fix issues from prior reviews resolved, review comment + auto-merge (self-authored)

**Issues triaged:**
- #80-#86 → `squad:linus` (all Go backend / CLI work)
- #89 → `squad:livingston` + `squad:saul` (docs with doc-review gate)

**Key observation:** Self-authored PRs can't be self-approved via GitHub API. For those, left review comments confirming LGTM and set auto-merge. The approval will come from branch protection or CODEOWNERS reviewers.

### Eval & Grader Registry Architecture (Issues #385–#390, Feb 25)

**Key Design Decisions:**
- **Repos-as-registry, not a central JSON file.** A Git repo with `waza.registry.yaml` IS a waza module. Same model as Go modules — zero infrastructure to publish. Shayne explicitly rejected single-file registries.
- **Federated index for discovery, not resolution.** Discovery is optional (like GOPROXY). Resolution is always direct Git clone. Offline-first, air-gap friendly.
- **OCI artifacts rejected for Phase 1.** Overkill for YAML+script packages. Docker concepts don't belong in eval authoring UX. Can revisit as alternative transport later.
- **External programs before WASM.** `program` grader already exists. Enhance it with `waza-grader-v1` protocol (stdin JSON → stdout JSON). WASM waits for WASI ecosystem maturity. Go plugins are dead — don't use them.
- **OpenAI battle.yaml doesn't map 1:1.** It's a completion-era pattern (compare two outputs). Agent-native equivalent is `waza compare` across runs. Don't cargo-cult completion patterns into agent eval.
- **`choice_scores` on prompt grader, not a new grader type.** One grader type handles classification, CoT, and scoring. Composability > type proliferation.
- **`extends:` uses shallow merge, local wins.** Predictable over clever. Deep merge of eval configs causes surprises.
- **Lock file with SHA-256 hashes.** Supply chain security is non-negotiable for a registry. `waza.lock` committed to source control, verified on every run.

**Document:** `tmp/waza-eval-registry-design.md`

### Release Infrastructure Audit (Issue #223, Feb 20)

**Version Management State:**
- `version.txt` (source-of-truth candidate): 0.4.0-alpha.1
- `extension.yaml`: 0.3.0 (STALE, 2 patch releases behind)
- `registry.json`: max version 0.3.0 (missing 0.4.0-alpha.1, blocking extension users)
- Go binary version: injected via ldflags `-X main.version=${VERSION}` during build

**Key Files & Their Purpose:**
- `.github/workflows/go-release.yml` — Standalone CLI release (builds 6 platform matrix, creates GitHub Release on `v*` tag) — **ACTIVE, currently used**
- `.github/workflows/azd-ext-release.yml` — Extension release (triggers on version.txt/extension.yaml changes, publishes to azd registry via `azd x publish`) — **ACTIVE, currently used**
- `.github/workflows/release.yml` — NEW unified workflow (both CLI + extension from single trigger, includes version sync job) — **PREPARED BUT NOT YET ACTIVATED**
- `.github/workflows/squad-release.yml` — Package.json-based release (NOT relevant to waza-go)
- `Makefile` — Local build with `VERSION?=0.1.0` (default, overridable)
- `build.sh` — Cross-platform binary builder for extension (VERSION env var driven)
- `cmd/waza/root.go` — Version variable: `var version = "dev"` (overwritten at build-time)

**Critical Issues Identified:**
1. **No unified release trigger** — CLI and extension release independently, easy to desync
2. **Version sync failure** — extension.yaml not bumped when CLI version.txt is updated
3. **registry.json desynchronization** — stale, depends on manual azd-ext-release workflow execution
4. **release.yml has logical flaw** — sync-versions job runs AFTER build jobs, too late to affect artifact versions
5. **Dual tag schemes** — CLI uses `v*`, extension uses `azd-ext-microsoft-azd-waza_*`, confusing
6. **No pre-flight validation** — easy for files to drift; no check before build

**Recommended Architecture:**
- Single canonical trigger: git tag `v*.*.*` 
- release.yml should be the sole release coordinator (retire go-release.yml + azd-ext-release.yml once stable)
- Pre-flight job in release.yml to validate version.txt == tag before proceeding
- Sync-versions job should run BEFORE builds, not after
- Document the flow in docs/RELEASE.md

**Immediate Action:** registry.json is blocking extension users on 0.4.0-alpha.1. Should manually trigger release.yml or azd-ext-release.yml to sync.


## 📌 Team update (2026-02-20): Model policy overhaul

All code roles now use `claude-opus-4.6`. Docs/Scribe/diversity use `gemini-3-pro-preview`. Heavy code gen uses `gpt-5.2-codex`. Decided by Scott Boyer. See decisions.md for full details.

### Competitive Analysis: Waza vs OpenAI Evals (Feb 21)

**Key Findings:**
- Waza wins 10 of 15 comparison dimensions on shipped capability. The fundamental gap is architectural: OpenAI Evals evaluates *completions*, waza evaluates *agents*. Different paradigm entirely.
- OpenAI Evals' strengths are ecosystem (800+ community evals, brand recognition, LangChain integration) and custom eval extensibility (Python class inheritance). Waza's strengths are agent-native architecture, trajectory visualization, statistical rigor, and development toolchain.
- OpenAI's Platform dashboard (platform.openai.com) is vendor-locked to OpenAI models — not a competitive threat to waza's model-agnostic approach.
- Highest-impact moves: (1) positioning doc framing "agent eval vs LLM eval" narrative, (2) migration guide from OpenAI Evals format, (3) grader plugin interface to close extensibility gap.
- OpenAI Evals cannot evaluate agents without a ground-up redesign. As the industry moves from completions to agents, this becomes a legacy framework for a shrinking use case.

**Document:** `tmp/waza-vs-openai-evals.md`

### Competitive Analysis: Waza vs skill-validator (Feb 25)

**Key Findings:**
- skill-validator is the closest peer competitor — same SDK (Copilot SDK), same domain (agent skill eval), same eval format (YAML scenarios). Not a legacy framework like OpenAI Evals; this is a real threat.
- Waza wins 9 of 15 dimensions (platform depth, visualization, multi-model, grader variety, data-driven testing, caching, MCP, distribution, hooks). skill-validator wins 4 (A/B testing, pairwise judging, tool constraints, auto-discovery). 2 at parity.
- skill-validator's 4 wins are concentrated in one coherent insight: **A/B testing (with/without skill)**. This answers "does the skill *help*?" — a question waza cannot answer with absolute grading alone. This is the single most important idea to adopt.
- Pairwise comparative judging with position-swap bias mitigation is architecturally more rigorous than waza's independent `prompt` grader for relative quality assessment.
- Tool constraint assertions (`expect_tools`/`reject_tools`) are a real gap — waza captures tool calls in trajectories but has no grader that inspects them.
- The strategic move: adopt skill-validator's A/B methodology (`--baseline` mode, pairwise judging, tool constraints) while maintaining platform advantages. Moving right on the "validates improvement" axis is easier than skill-validator moving up on the "full lifecycle" axis.
- Worst-case scenario: skill-validator becomes the "correct" way to validate skills internally at Microsoft while waza is seen as "just an eval runner." Best-case: waza ships `--baseline` before skill-validator ships a dashboard.

**Document:** `tmp/waza-vs-skill-validator.md`

### Batch PR Review — Linus Feature PRs + Legacy Fixes (Mar 2026)

**Reviewed 7 PRs with Opus 4.6 verification.**

**PRs reviewed + auto-merged (CI green):**
- **#91** (multi-trial flakiness detection, issue #84) — `--trials` flag, `FlakinessPercent` metric, minority-outcomes formula, new stats fields. Clean extension of runner stats pipeline. LGTM + auto-merge.
- **#92** (eval coverage grid generator, issue #82) — New `waza coverage` command, three output formats, smart skill/eval discovery with deduplication. Minor note: `evalSpecLite.Tasks` is `[]string` vs structured task objects — conservative for reporting. LGTM + auto-merge.
- **#93** (waza tokens diff, issue #81) — Precisely matches Token Diff Distribution Strategy from decisions.md. Smart base ref fallback, reuses git helper + token counter, clean JSON report. LGTM + auto-merge.

**PRs blocked:**
- **#90** (trigger heuristic grader, issue #80) — Code is architecturally sound (clean grader pattern, constructor validation, 4 tests, trigger.md docs, schema updates) but has **merge conflict** (likely `internal/models/outcome.go` overlap with PR #91). Only CLA check ran. Needs rebase.
- **#65** (config schema defaults fix) — CI still failing: Go Build+Test ❌ on ubuntu + windows. Last commit from March 4.
- **#64** (token limits priority inversion) — CI still failing: Lint ❌. Last commit from March 4.
- **#55** (prompt grader migration) — Incomplete: last commit message is "Changes before error encountered", only CLA ran.

**Key observation:** Self-authored PRs can't be self-approved or self-request-changes via GitHub API. For approved PRs, left review comments + set auto-merge. For blocked PRs (#65, #64, #55), used request-changes. For blocked self-authored PR (#90), left comment.

**Pattern confirmed:** The same-account limitation is consistent with previous batch review. The workaround (comment + auto-merge for approvals) is reliable.

### Platform Module Architecture (feature/waza-platform)

**Date:** 2026-07 | **Branch:** `feature/waza-platform`

**What:** Designed and created the `internal/platform/` module — the contract layer for Waza Platform (hosted PaaS). Five files across four packages:

- `internal/platform/README.md` — Module overview with Mermaid architecture diagram and data flow sequence diagram.
- `internal/platform/auth/auth.go` — `AuthProvider` interface (GitHub OAuth), `User`/`Session` structs, `Middleware` type, `UserFromContext` helper.
- `internal/platform/db/db.go` — `Store` interface (13 methods), `Connection`/`RunRequest` structs, `ConnectionType`/`RunStatus` enums with helper methods.
- `internal/platform/api/routes.go` — Full route registration via `http.ServeMux` (Go 1.22+ patterns), 14 endpoints across auth/connections/runs/repos, `Dependencies` bundle for DI.
- `internal/platform/adc/config.go` — `ADCConfig` struct with YAML/JSON tags, `WithDefaults()` and `CanAllocate()` methods, quota constants.

**Design decisions:**
- Used Go 1.22+ `http.ServeMux` method patterns (`"GET /api/..."`) instead of chi/gorilla — fewer deps for a contract layer.
- `Store` interface methods are user-scoped by design — no cross-user queries in v1.
- ADC quota (max 10 sandboxes) is a constant, not config — changing limits requires code review.
- `Connection.Config` is `map[string]any` — provider-specific config varies too much for a fixed struct.
- `RunRequest.ADCSandboxIDs` is `[]string` — tracks allocated sandboxes for cleanup on cancellation.
- `auth.UserContextKey` uses a typed `ContextKey` to avoid collisions.

**For Linus:** These are the interfaces to implement against. Start with `db.Store` (Cosmos SDK), then `auth.AuthProvider` (GitHub OAuth), then wire up `api.RegisterRoutes`.

### Platform Frontend — Wave 2 (feature/waza-platform)

**Date:** 2026-07 | **Branch:** `feature/waza-platform`

**What:** Implemented the full React SPA layer for Waza Platform — auth gate, settings, new run wizard, and updated navigation. Six tasks delivered:

1. **AuthGate** (`web/src/components/AuthGate.tsx`) — Calls `/api/auth/me` on mount. 401 → GitHub login page. Success → render app. Loading spinner while checking.
2. **AuthContext** (`web/src/contexts/AuthContext.tsx`) — `AuthProvider` + `useAuth()` hook. Standalone state (not React Query) because auth is infrastructure-level.
3. **Settings** (`web/src/components/Settings.tsx`) — Tab-based. Connections tab: full CRUD for Azure Storage and GitHub Repo connections, with test/delete buttons and status badges. Preferences tab: placeholder with model selector and worker count.
4. **NewRun** (`web/src/components/NewRun.tsx`) — 4-step wizard: Select repo → Select eval → Configure (model, workers, parallel) → Review & Run. Redirects to Live view on trigger.
5. **Navigation** — "New Run" button (blue, header-right), Settings gear icon, user avatar dropdown with Settings + Sign out.
6. **API Client** — 10 new functions in `client.ts`, 7 new types, 9 new React Query hooks (queries + mutations with cache invalidation).

**Key observation:** E2e test mocks needed auth endpoint added — `mockAllAPIs` now returns a mock user for `/api/auth/me`. All 52 existing Playwright tests pass without changes to individual test files.

**Build verified:** TypeScript clean, Vite production build clean, 52/52 Playwright e2e tests pass.

### Storage Destination Selector (feature/waza-platform)

**Date:** 2026-07 | **Branch:** `feature/waza-platform`

**What:** Added results storage destination to the New Run wizard and backend. Cosmos DB is the primary/default; BYOS Azure Storage is optional.

**Changes across 4 files:**
- `web/src/components/NewRun.tsx` — Step 3 now includes a Results Storage section. If the user has no `azure-storage` connections, shows a static "💾 Results stored in Waza Cloud" label. If connections exist, shows a dropdown: "Waza Cloud (default)" + `{accountName}/{containerName}` for each connection. Step 4 review panel shows the selection. Uses existing `useConnections()` hook.
- `web/src/api/client.ts` — `TriggerRunConfig.storageDestination` added (optional, values: `"cosmos"` or connection ID).
- `internal/platform/api/handlers.go` — `triggerRunRequest.StorageDestination` field, defaults to `"cosmos"` when empty. Logged on trigger and passed through to `db.RunRequest`. `dispatchToADC` logs the destination for traceability.
- `internal/platform/db/db.go` — `RunRequest.StorageDestination` field persisted to Cosmos.

**Key decisions:**
- No dropdown when zero storage connections — keeps the UI minimal for most users.
- Default is always `"cosmos"` — BYOS is opt-in, not opt-out.
- Storage destination is stored on the RunRequest, not resolved at dispatch time — this makes the intent auditable and replayable.

**Build verified:** `go vet` clean, TypeScript + Vite production build clean. Deployed to Azure Container Apps (200 OK).

### Run Status & Queue UI (feature/waza-platform)

**Date:** 2026-07 | **Branch:** `feature/waza-platform`

**What:** Fixed the "black hole" UX after clicking "Run Eval" — users now get immediate feedback via a status page and can monitor all runs in a queue view.

**Changes across 8 files (488 insertions):**

1. **RunStatus.tsx** — New page at `#/runs/status/{runId}`. Polls `/api/runs/queue` every 3s via `useRunStatus` hook. Status card with run details + color-coded badges (🟡 Queued, 🔵 Running, ✅ Complete, ❌ Failed, ⚪ Cancelled). Timeline/log area. "View Results" link when complete.
2. **RunQueue.tsx** — New page at `#/runs/queue`. Auto-refreshing table (10s) with cancel/results actions. Click row → RunStatus.
3. **NewRun.tsx** — Redirect changed from `#/live?run={runId}` to `#/runs/status/{runId}` (the key UX fix).
4. **App.tsx** — Two new routes. `/runs/status/:id` must match before `/runs/:id`.
5. **Layout.tsx** — "Queue" nav link with `ListOrdered` icon.
6. **useApi.ts** — `useRunStatus(runId)` hook (3s polling), `useRunQueue` updated to 10s.
7. **client.ts** — `RunQueueItem` extended with `repo`, `storageDestination`, `error`, union status type.

**Build verified:** TypeScript clean, Vite production build clean. `go vet` clean. Deployed to Azure Container Apps (200 OK).

### Cosmos Results Integration (feature/waza-platform)

**Date:** 2026-07 | **Branch:** `feature/waza-platform`

**What:** Wired the dashboard to display Cosmos DB results alongside local file results. Five deliverables across 7 files (186 insertions):

1. **API Client** (`web/src/api/client.ts`) — Added `ResultSummary` type with `runId` and `source` fields. Added `fetchResults()` (list) and `fetchResultDetail()` (single) functions hitting `/api/results` and `/api/results/{id}`.
2. **React Query Hooks** (`web/src/hooks/useApi.ts`) — `useResults()` for the results list, `useResultDetail(id)` for individual result detail.
3. **Dashboard** (`web/src/components/Dashboard.tsx`) — Merges `/api/runs` (local) and `/api/results` (Cosmos) into a single sorted table. Deduplication by ID — Cosmos wins on conflict (it's the authoritative completed result). Shows loading skeleton only when both sources are loading.
4. **RunDetail** (`web/src/components/RunDetail.tsx`) — Dual-source fallback: tries `/api/runs/{id}` first, falls back to `/api/results/{id}`. Users can click any result row and get the full detail view regardless of source.
5. **RunStatus** (`web/src/components/RunStatus.tsx`) — When a run completes, fetches result detail and shows a green summary card with pass rate, task count, tokens, and duration. "View Results" button was already present.
6. **E2e Mocks** (`web/e2e/helpers/api-mock.ts`) — Added `/api/results` and `/api/results/{id}` mocks to both `mockAllAPIs` and `mockEmptyAPIs`. All 52 Playwright tests pass.

**Key decisions:**
- Cosmos results take precedence over local runs in merge — Cosmos is the authoritative store per team decision.
- Dashboard shows unified table (not separate tabs) — simpler UX, users don't need to know the storage backend.
- RunDetail falls back gracefully — no routing changes needed, existing `#/runs/{id}` route handles both sources.
- RunStatus summary only appears when result data loads — no broken UI if results API is slow.
