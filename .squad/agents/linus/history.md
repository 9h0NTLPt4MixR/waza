# History — Linus

## Project Context
- **Project:** waza — CLI tool for evaluating Agent Skills
- **Stack:** Go (primary), React 19 + Tailwind CSS v4 (web UI)
- **User:** Shayne Boyer (spboyer)
- **Repo:** microsoft/waza
- **Universe:** The Usual Suspects

## Key Learnings

### Go Architecture
- **Model directive:** Code generation in GPT-5.3-Codex; review/verification in Claude Opus 4.6 (user requirement)
- **Code structure:** Functional options pattern for configuration
- **Interfaces:** AgentEngine, Validator, Grader (extensible design)
- **Testing:** Unit tests in internal packages, integration tests for CLI

### Waza-specific
- Fixture isolation: temp workspace created per task, original fixtures never modified
- TestCase, BenchmarkSpec, EvaluationOutcome models
- ValidatorRegistry pattern for pluggable graders
- CLI flags: -v (verbose), -o (output), --context-dir (fixtures)

### Integration
- Copilot SDK integration (via AgentEngine interface)
- Web UI gets results from CLI JSON output
- Makefile for build/test/lint automation

### Web API Architecture
- API types in `internal/webapi/types.go` are decoupled from internal models (no direct imports)
- `outcomeToDetail()` in `store.go` maps `models.EvaluationOutcome` → API response types
- JSON uses camelCase consistently across the API surface
- TranscriptEvent mapping uses direct field access (not marshal/unmarshal) due to MarshalJSON snake_case mismatch

## Completed Work

### #237 — Expose transcript & session digest in web API (PR #242)
- **Date:** 2026-02-19
- **Branch:** `squad/237-api-transcript`
- **Files changed:** `internal/webapi/types.go`, `internal/webapi/store.go`, `internal/webapi/handlers_test.go`, `web/src/api/client.ts`
- **What:** Added `TranscriptEventResponse`, `SessionDigestResponse` API types; wired them into `TaskResult`; mapped from `RunResult` in `outcomeToDetail()`; added TS interfaces; added test

### #239 — Trajectory Diffing (PR #244)
- **Date:** 2026-02-19
- **Branch:** `squad/239-trajectory-diffing`
- **Files changed:** `web/src/components/TrajectoryDiff.tsx` (new), `web/src/components/TaskTrajectoryCompare.tsx` (new), `web/src/components/CompareView.tsx` (modified)
- **What:** Added trajectory diffing to CompareView — aligns ToolExecutionStart events by tool name+index, renders matched/changed/insertion/deletion with color coding and expandable JSON diffs. No backend changes needed.


## 📌 Team update (2026-02-20): Model policy overhaul

All code roles now use `claude-opus-4.6`. Docs/Scribe/diversity use `gemini-3-pro-preview`. Heavy code gen uses `gpt-5.2-codex`. Decided by Scott Boyer. See decisions.md for full details.

### #299 — Grader Weighting (PR pending)
- **Date:** 2026-02-20
- **Branch:** `squad/299-grader-weighting`
- **Files changed:** `internal/models/spec.go`, `internal/models/outcome.go`, `internal/orchestration/runner.go`, `internal/reporting/interpreter.go`, `internal/webapi/types.go`, `internal/webapi/store.go`, `internal/models/spec_test.go`, `internal/models/outcome_test.go`
- **What:** Added optional `weight` field to `GraderConfig` (default 1.0 via `EffectiveWeight()`). Added `ComputeWeightedRunScore()` to `RunResult`. Weighted composite score surfaces in `TestStats.AvgWeightedScore`, `OutcomeDigest.WeightedScore`, and the interpretation report. Web API `GraderResult` also carries weight. All existing eval.yaml files work unchanged — weight is optional and defaults to 1.0.
- **Key learning:** `ValidatorInline` (task-level graders) already had a `Weight` field before this change — only `GraderConfig` (spec-level) was missing it. The runner is the correct place to stamp weights onto `GraderResults` since graders themselves don't know their config weight.

### #314 — agentskills.io Spec Compliance Checks (PR #322)
- **Date:** 2026-02-20
- **Branch:** `squad/314-spec-compliance`
- **Files changed:** `cmd/waza/dev/spec.go` (new), `cmd/waza/dev/spec_test.go` (new), `cmd/waza/dev/display.go`, `cmd/waza/dev/display_test.go`, `cmd/waza/dev/score_test.go`, `cmd/waza/dev/loop_test.go`, `cmd/waza/cmd_check.go`
- **What:** Added `SpecScorer` with 8 formal agentskills.io spec checks (frontmatter, allowed-fields, name format, dir-match, description length, compatibility length, license recommendation, version recommendation). Integrated into both `waza dev` (inline in DisplayScore) and `waza check` (separate section, summary table column, readiness gate). 15 new test cases.
- **Key learning:** `makeSkill` test helper needed `FrontmatterRaw` field populated (was nil before) to properly test spec checks. Existing display/loop tests use exact string matching — any new output from `DisplayScore` requires updating all dependent test expected strings.

### #308 — Statistical Confidence Intervals (PR #323)
- **Date:** 2026-02-20
- **Branch:** `squad/308-statistical-ci`
- **Files changed:** `internal/statistics/bootstrap.go` (new), `internal/statistics/bootstrap_test.go` (new), `internal/models/outcome.go`, `internal/orchestration/runner.go`
- **What:** New `internal/statistics/` package with `BootstrapCI` (10k resamples, percentile method), `IsSignificant` (CI doesn't cross zero), and `NormalizedGain` (Hake 1998). Wired bootstrap CI into `computeTestStats` (per-task, when ≥2 runs) and `buildOutcome` (digest-level `StatisticalSummary`). Also populated previously-empty `TestStats` fields: `ScoreVariance`, `CI95Lo`, `CI95Hi`, `Flaky`. 11 test cases covering edge cases, determinism, and CI properties. Fully backward compatible via `omitempty`/pointer types.
- **Key learning:** `TestStats` already had `CI95Lo`/`CI95Hi`/`ScoreVariance`/`Flaky` fields defined but never populated — they were placeholders from initial model design. The `internal/metrics` package already had a normal-approximation `ConfidenceInterval95` function; the new bootstrap approach is more robust for small samples and non-normal distributions. Using `BootstrapCIWithSeed` for deterministic tests is essential — non-seeded bootstrap CIs are non-deterministic and will cause flaky tests.

### #311 — Skill Profile Static Analysis (PR #325)
- **Date:** 2026-02-20
- **Branch:** `squad/311-skill-profile`
- **Files changed:** `cmd/waza/tokens/profile.go` (new), `cmd/waza/tokens/profile_test.go` (new), `cmd/waza/tokens/testdata/profile/SKILL.md` (new), `cmd/waza/tokens/root.go`, `README.md`, `site/src/content/docs/reference/cli.mdx`
- **What:** Added `waza tokens profile` subcommand for structural analysis of SKILL.md files. Reports token count, section count (## and deeper), code block count, numbered workflow steps, detail level classification (minimal/standard/detailed), and warnings (no steps, >2500 tokens, <3 sections). Supports JSON output and configurable tokenizer. 25 tests.
- **Key learning:** The tokens subcommand pattern is well-established — each subcommand (`check`, `compare`, `count`, `suggest`, now `profile`) gets its own file, uses shared `findMarkdownFiles` and `countTokens` helpers from `helpers.go`, and registers in `root.go`. The `findSkillFiles` filter (SKILL.md only) was needed since `findMarkdownFiles` returns all .md/.mdx files. The `mockCounter` test helper pattern (implementing `tokens.Counter` interface) is clean for testing analysis functions without BPE overhead.

### #286 — MCP Server (PR #364)
- **Date:** 2026-02-21
- **Branch:** `squad/286-mcp-server`
- **Files changed:** `internal/mcp/server.go` (new), `internal/mcp/tools.go` (new), `internal/mcp/stdio.go` (new), `internal/mcp/server_test.go` (new), `cmd/waza/cmd_serve.go`, `.copilot/mcp.json` (new)
- **What:** Added MCP (Model Context Protocol) server that runs on stdio alongside the HTTP dashboard in `waza serve`. Exposes 10 tools mapped from existing JSON-RPC handlers and webapi store. Thin adapter pattern — MCP server delegates to `internal/jsonrpc/` handlers via `MethodRegistry.Lookup`. New tools: `waza_results_summary`, `waza_results_runs` (from `webapi.FileStore`), `waza_skill_check` (lightweight readiness check). MCP is always on — no flag. 10 tests.
- **Key learning:** MCP protocol is essentially JSON-RPC 2.0 with specific methods (`initialize`, `tools/list`, `tools/call`) and a content-block response format for tool results. The thin adapter pattern works well — reusing `jsonrpc.Transport` for stdio and `jsonrpc.Handler` functions for tool dispatch avoids duplicating logic. The `tools/call` response wraps results as `{content:[{type:"text",text:"<json>"}]}` which means all tool results must be serialized to JSON text. Notifications (no `id` field) must not receive responses per both JSON-RPC 2.0 and MCP spec.

### #287 — `waza suggest` command (PR pending)
- **Date:** 2026-02-21
- **Branch:** `squad/287-suggest-command`
- **Files changed:** `cmd/waza/cmd_suggest.go` (new), `cmd/waza/cmd_suggest_test.go` (new), `internal/suggest/prompt.go` (new), `internal/suggest/suggest.go` (new), `internal/suggest/suggest_test.go` (new), `cmd/waza/root.go`, `README.md`, `site/src/content/docs/reference/cli.mdx`
- **What:** Added `waza suggest <skill-path>` for LLM-driven eval generation. Command supports `--model`, `--dry-run` (default), `--apply`, `--output-dir`, and `--format yaml|json`. New `internal/suggest` package builds prompt context from SKILL.md + grader types + eval schema summary + example eval, parses structured YAML responses, validates generated `eval_yaml`, and writes `eval.yaml`/task/fixture files when applying.
- **Key learning:** A robust parser needs to handle both structured wrapper YAML (`eval_yaml` + files) and fenced YAML blocks from models. Validating generated `eval_yaml` against `models.BenchmarkSpec.Validate()` catches malformed model output early before writing files.

## Learnings
- Windows local test runs can fail in `cmd/waza/tokens/internal/git` when temporary repos inherit strict CRLF behavior; setting `core.autocrlf=false` and `core.safecrlf=false` inside test repo setup makes these tests cross-platform stable.
- PR conflict resolution for `copilot/migrate-copilot-client-usage` in `internal/execution/copilot_test.go` should keep the `TestCopilotExecute_InitializePropagatesStartError` variant from main to preserve startup error propagation coverage.
- Coverage command (`cmd/waza/cmd_coverage.go`) uses `models.BenchmarkSpec` directly for eval parsing — no custom lite struct. Parse failures and missing grader kinds are hard errors. `parseSkillName` propagates errors.
- `slices.Sorted(maps.Keys(...))` is the idiomatic Go 1.23+ replacement for custom sorted-keys-from-map helpers. Available in Go 1.26.
- Full coverage threshold is >= 1 grader type (not 2) — a single complex grader like Program is sufficient.

### Platform Backend — Initial Implementation
- **Date:** 2026-03-30
- **Branch:** `feature/waza-platform`
- **Files created:** `internal/platform/auth/github.go`, `internal/platform/db/cosmos.go`, `internal/platform/adc/engine.go`, `Dockerfile.adc-runner`
- **Files modified:** `internal/projectconfig/config.go`
- **What:** Implemented concrete platform backend pieces against Rusty's interface contracts:
  1. **GitHubProvider** (auth/github.go) — full OAuth flow: login redirect, callback with code exchange, HMAC-SHA256 JWT sessions, token validation, session revocation, and auth middleware factory. Implements `AuthProvider` interface.
  2. **CosmosStore** (db/cosmos.go) — full Cosmos DB implementation of `Store` interface: CRUD for users, connections, run requests, and settings. AES-256-GCM encryption for connection configs. Partition key is user's GitHub ID as string.
  3. **ADC Engine** (adc/engine.go) — implements `AgentEngine` for sandboxed eval execution: sandbox lifecycle (create → upload → execute → collect → delete), mutex-protected sandbox tracking, graceful shutdown.
  4. **ProjectConfig ADC field** — added `ADCConfig` struct inline in projectconfig (mirroring adc.ADCConfig to avoid import dependency on ADC SDK) with yaml merge support.
  5. **Dockerfile.adc-runner** — multi-stage build for ADC sandbox disk images with waza binary + git/bash/curl/jq.
- **Key learning:** Can't import `internal/platform/adc` from `internal/projectconfig` because `engine.go` depends on the ADC SDK (`github.com/coreai-microsoft/adc-sdk-go`) which isn't in go.mod yet. Defined `ADCConfig` inline in projectconfig instead — same pattern as all other config types in that file.
- **Key learning:** Rusty's interface types (`auth.User`, `db.Connection`, `db.RunRequest`) are the source of truth. Implementation files must not redeclare types — use Rusty's definitions directly. The `auth.User` uses `GitHubID int64` (not string ID), and `db.Store` methods take `int64` user IDs.
- **Pending:** ADC SDK needs to be added to go.mod (`go get github.com/coreai-microsoft/adc-sdk-go`) before `internal/platform/adc/` compiles. Auth and DB packages pass `go vet` and compile now.

### Platform API Handlers & Server Mode — Wave 2
- **Date:** 2026-03-30
- **Branch:** `feature/waza-platform`
- **Files created:** `internal/platform/api/handlers.go`, `internal/platform/api/deps.go`, `azure.yaml`, `Dockerfile.platform`, `infra/main.bicep`, `infra/main.parameters.json`
- **Files modified:** `internal/platform/api/routes.go`, `internal/platform/api/routes_test.go`, `cmd/waza/cmd_serve.go`
- **What:** Implemented 14 HTTP handlers (auth, connections, runs, repos), Dependencies struct with ADCDispatcher interface, `--platform` serve mode with Cosmos/OAuth/ADC initialization, and full Azure deployment infrastructure (Container Apps + Cosmos DB serverless + Key Vault + Managed Identity). All 13 platform API tests pass including user isolation.
- **Key learning:** The ADC SDK dependency gap means any type that touches `internal/platform/adc/` can't compile. Solved by defining `ADCDispatcher` as an interface in the api package — decouples handlers from the concrete ADC engine. When the SDK lands, implement the interface.
- **Key learning:** Platform mode uses environment variables for all secrets (not .waza.yaml). This aligns with Container Apps Key Vault reference injection and 12-factor principles.
- **Key learning:** Connection testing uses lightweight HTTP probes (list 1 blob, GET repo) rather than importing heavy Azure/GitHub SDKs. Keeps the API handler layer dependency-free.
- **Pending:** ADC SDK still needed in go.mod. Storage proxy handlers (HandleProxyRuns, HandleProxyRunDetail) deferred until BYOS storage flow is finalized.

### Platform SPA Fix — Serve embedded React app in platform mode
- **Date:** 2026-03-30
- **Branch:** `feature/waza-platform`
- **Files changed:** `cmd/waza/cmd_serve.go`, `internal/webserver/routes.go`
- **What:** Exported `SPAHandler()` from the webserver package and mounted it as the catch-all `/` handler in `runPlatformServer`. Platform mode was only registering API routes, causing 404 on root. API routes (`/api/*`) registered first take precedence via Go ServeMux specificity rules.
- **Key learning:** Go 1.22+ ServeMux pattern specificity means method-qualified routes like `GET /api/auth/me` always beat the bare `/` catch-all, so registration order doesn't matter — just mount the SPA on `/` and API routes coexist safely.
- **Key learning:** Container Apps can briefly serve stale revisions during rollout — first curl after deploy may hit the old container. Wait a few seconds or use `Cache-Control: no-cache`.

### Cosmos DB Results Container — Persistent Eval Storage
- **Date:** 2026-03-31
- **Branch:** `feature/waza-platform`
- **Files created:** `internal/webapi/cosmos_adapter.go`
- **Files modified:** `internal/platform/db/db.go`, `internal/platform/db/cosmos.go`, `internal/platform/db/cosmos_test.go`, `internal/platform/api/handlers.go`, `internal/platform/api/routes.go`, `internal/platform/api/routes_test.go`, `cmd/waza/cmd_serve.go`, `infra/main.bicep`
- **What:** Added a `results` Cosmos container so eval outcomes are always persisted, even without BYOS Azure Storage:
  1. **Store interface** — added `SaveResult`, `GetResult`, `ListResults` methods and `ResultSummary` type to `db.go`.
  2. **CosmosStore** — implemented all three methods in `cosmos.go`. `SaveResult` extracts summary fields (spec, model, pass_rate) for indexed querying. Documents store the full EvaluationOutcome JSON under a `result` key. Uses string partition key (`user_id`) consistent with all other containers.
  3. **Platform API** — added `GET /api/results` and `GET /api/results/{id}` endpoints. Updated `handleTriggerRun` and `dispatchToADC` with comments showing where Cosmos save will be wired after ADC returns results.
  4. **Dashboard fallback** — `cmd_serve.go` now checks if BYOS storage is configured. If not, dashboard `/api/runs` uses a `CosmosRunStore` adapter that reads from the results container instead of local files.
  5. **Infrastructure** — added `containerResults` resource to `infra/main.bicep` with partition key `/user_id`.
- **Key learning:** When adding new methods to the `Store` interface, both the `cosmos_test.go` and `routes_test.go` mock stores must be updated to satisfy the compile-time interface check. Always grep for `var _ Store` / `var _ db.Store` to find all mock implementations.
- **Key learning:** The `CosmosRunStore` adapter in `internal/webapi/cosmos_adapter.go` bridges the platform `db.Store` to the dashboard's `webapi.RunStore` interface. This allows the same dashboard UI to work regardless of whether results come from local files, Azure Blob Storage, or Cosmos DB.

### Platform API Run Endpoint Fixes
- **Date:** 2026-03-31
- **Branch:** `feature/waza-platform`
- **Files changed:** `internal/platform/api/handlers.go`, `internal/platform/api/routes.go`, `internal/platform/api/handlers_test.go`, `internal/platform/api/routes_test.go`, `internal/platform/db/cosmos.go`
- **What:** Fixed 5 issues in the run queue API:
  1. `handleTriggerRun` now returns `{runId, status}` (camelCase) instead of the full `db.RunRequest` (snake_case). Frontend can redirect to `/runs/{runId}`.
  2. `handleListRuns` (GET /api/runs/queue) now returns `[]runQueueItem` with camelCase field names (`evalSpec`, `storageDestination`, `createdAt`, etc.) instead of raw `db.RunRequest`.
  3. Registered `GET /api/runs/queue/{id}` route using existing `handleGetRun` handler (was implemented but never wired). Returns single `runQueueItem` for status page polling.
  4. Added missing `storage_destination` and `error` fields to Cosmos `CreateRunRequest` and `UpdateRunRequest` document maps — these were defined in the struct but never persisted.
  5. Added `parseRunRequest` function (mirrors `parseConnection` pattern) to handle the string→int64 user_id mismatch when unmarshaling Cosmos documents back into Go structs.
- **Key learning:** Cosmos stores `user_id` as a string (to match partition key) but `db.RunRequest.UserID` is `int64`. Direct `json.Unmarshal` fails silently (zero value). The `parseRunRequest` fallback pattern (try unmarshal, on failure re-parse without the mismatched field and use fallback) is the same pattern used by `parseConnection`. Always check that Cosmos doc maps include ALL struct fields.
- **Key learning:** Integration tests in `handlers_test.go` used underscore connection types (`azure_storage`, `github_repo`) while the `db` constants use hyphens (`azure-storage`, `github-repo`). Test bugs are invisible when tests are green — always check that test assertions are actually exercising the right code paths.


### Wired Eval Execution into Run Trigger
- **Date:** 2026-03-31
- **Branch:** `feature/waza-platform`
- **Files changed:** `internal/platform/execution/runner.go` (new), `internal/platform/execution/runner_test.go` (new), `internal/platform/api/handlers.go`
- **What:** Connected the trigger handler to actual eval execution via local subprocess:
  1. Created `internal/platform/execution` package with `RunEval()` — clones repo, finds eval spec, runs `waza run` as subprocess, captures results JSON, saves to Cosmos via `Store.SaveResult()`.
  2. Replaced the stub `dispatchToADC()` goroutine with `dispatchRun()` that calls `execution.RunEval` with the user's GitHub token from auth context.
  3. Full status lifecycle: queued → running → complete/failed. `CompletedAt` timestamp set on terminal states.
  4. Safety: `defer recover()` in both `RunEval` and `dispatchRun`, context timeout (5 min default), token sanitization in error messages.
  5. Tests cover clone failure, context cancellation, token sanitization, truncation, and markFailed.
- **Key learning:** The waza binary is available in the container (it's the same binary serving the platform). Running `waza run` as a subprocess is the simplest path to end-to-end execution. ADC sandbox execution can replace the subprocess call later by swapping the `os/exec` call for ADC SDK sandbox creation — the handler wiring stays the same.
- **Key learning:** GitHub tokens must NEVER appear in error messages or logs. Use `strings.ReplaceAll` to scrub before logging. The `sanitizeToken` helper handles this.
- **Key learning:** `os.MkdirTemp` creates workspace dirs for cloning; `defer os.RemoveAll` ensures cleanup even on failure. Each run gets an isolated workspace.


### Added `--executor` CLI Flag and Platform Executor Override
- **Date:** 2026-04-01
- **Branch:** `feature/waza-platform`
- **Files changed:** `cmd/waza/cmd_run.go`, `internal/platform/execution/runner.go`, `internal/platform/api/handlers.go`, `internal/platform/db/db.go`
- **What:** Added `--executor` flag to `waza run` so the platform can override eval YAML's `config.executor` field:
  1. Added `executorOverride` package-level var and `--executor` cobra flag in `cmd_run.go`. Override applied after spec load, before engine factory switch — same pattern as `--model`, `--parallel`, `--judge-model`.
  2. In `runner.go`, `RunConfig` gained an `Executor` field. `RunEval` defaults to `copilot-sdk` if empty, and always passes `--executor <value>` in subprocess args. This ensures external repos with `executor: mock` actually call the LLM on the platform.
  3. In `handlers.go`, added `Executor` field to `triggerRunRequest` (defaults to `copilot-sdk`), `runQueueItem`, and `toRunQueueItem`. Wired through `db.RunRequest.Executor` → `dispatchRun` → `RunConfig.Executor`.
  4. In `db.go`, added `Executor` field to `RunRequest` struct with `json:"executor,omitempty"`.
- **Key learning:** The override chain for CLI flags follows a consistent pattern: package-level var → cobra flag registration → apply in `runCommandForSpec` after spec load but before engine creation. Always place new overrides near existing ones for readability.
- **Key learning:** Platform subprocess always defaults to `copilot-sdk` — never trust eval YAML from external repos to specify the right executor. The empty-string check in `runner.go` ensures backward compatibility if `RunConfig.Executor` is not set.

### Dockerfile.platform — Switch runtime to glibc (debian:bookworm-slim)
- **Date:** 2026-04-01
- **Branch:** `feature/waza-platform`
- **Files changed:** `Dockerfile.platform`
- **What:** The embedded Copilot SDK CLI binary (`copilot_1.0.2`) is dynamically linked against glibc. Alpine uses musl libc, so `fork/exec` failed with `no such file or directory` when waza extracted and ran the binary at `/root/.cache/copilot-sdk/copilot_1.0.2`. Switched runtime stage from `alpine:3.21` to `debian:bookworm-slim`. Builder stage stays Alpine (waza itself is `CGO_ENABLED=0`, fully static).
- **Key learning:** `no such file or directory` when exec'ing a binary in a container almost always means the dynamic linker is missing — check `readelf -l` or `file` output for the interpreter path. Alpine/musl can't run glibc-linked binaries without compatibility shims. Switching to a glibc-based slim image is the cleanest fix.

### Rerun API Endpoint
- **Date:** 2026-04-01
- **Branch:** `feature/waza-platform`
- **Files changed:** `internal/platform/api/handlers.go`, `internal/platform/api/routes.go`
- **What:** Added `POST /api/runs/rerun/{id}` endpoint that clones config from an existing run (repo, evalSpec, model, executor, workers, storageDestination) into a new queued run with a fresh ID and timestamp. Dispatches via the same `dispatchRun` goroutine as `handleTriggerRun`. Returns `201 Created` with `{runId, status}`.
- **Key learning:** Go's `net/http` ServeMux panics when two patterns like `POST /api/runs/{id}/rerun` and `POST /api/runs/cancel/{id}` both have wildcards in overlapping positions — they're ambiguous for paths like `/api/runs/cancel/rerun`. Solution: use `POST /api/runs/rerun/{id}` (action-first) to match the existing `cancel/{id}` convention. All action-scoped run routes should follow the `POST /api/runs/{action}/{id}` pattern.
