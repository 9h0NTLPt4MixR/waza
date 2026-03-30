# Decision: Platform API Handlers & Server Mode

**By:** Linus (Backend Developer)
**Date:** 2026-03-30
**Branch:** `feature/waza-platform`

## What

Implemented the full HTTP API handler layer, platform Dependencies struct, `--platform` serve mode, and Azure deployment infrastructure:

1. **`internal/platform/api/handlers.go`** — 14 handler implementations covering auth (me, logout), connections (CRUD + test), runs (trigger, queue, get, cancel), repos (list, list evals). All handlers extract user from context, return structured JSON errors, and are user-scoped.

2. **`internal/platform/api/deps.go`** — `Dependencies` struct bundling `Store`, `Auth`, `AuthMiddleware`, and optional `ADCDispatcher` interface. Uses an interface for ADC to avoid pulling the ADC SDK (not yet in go.mod).

3. **`cmd/waza/cmd_serve.go`** — Added `--platform` flag. Platform mode initializes Cosmos DB, GitHub OAuth, and optional ADC engine from environment variables, binds to `0.0.0.0`, skips browser auto-open, and adds `/healthz` for container probes.

4. **Azure deployment** — `azure.yaml` (azd manifest), `Dockerfile.platform` (multi-stage Go + SPA build), `infra/main.bicep` (Container Apps + Cosmos DB serverless + Key Vault + Managed Identity + role assignments), `infra/main.parameters.json`.

## Key Decisions

- **ADCDispatcher interface instead of direct `*adc.Engine`**: The ADC SDK isn't in go.mod yet. Using an interface in deps.go means the API package compiles cleanly. When the SDK lands, swap `ADCDispatcher` to include `Execute` and `Shutdown`.

- **Environment variables for platform config**: Platform mode reads all secrets from env vars (`COSMOS_ENDPOINT`, `COSMOS_KEY`, `GITHUB_CLIENT_ID`, etc.) rather than .waza.yaml. This aligns with 12-factor and Key Vault reference injection in Container Apps.

- **Connection test is probe-based**: `handleTestConnection` does a lightweight HTTP probe (list 1 blob for Azure Storage, GET repo for GitHub) rather than importing the full Azure SDK. This keeps the binary lean.

- **Run dispatch is async goroutine**: `handleTriggerRun` creates the RunRequest in DB synchronously, then fires a goroutine for ADC dispatch. The client gets 202 immediately. ADC integration is stubbed until the SDK lands.

- **Cosmos DB serverless**: The Bicep uses serverless capacity mode — no throughput provisioning needed, pay-per-request. Appropriate for platform v1.

## Impact

- All 13 platform API tests pass (including newly un-skipped CRUD, trigger, cancel, and user isolation tests)
- Full binary builds clean
- Existing `waza serve` behavior unchanged (platform mode is opt-in via `--platform`)
