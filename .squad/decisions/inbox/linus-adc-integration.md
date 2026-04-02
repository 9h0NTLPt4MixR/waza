# Decision: ADC SDK Integration — GitHub Token Auth, No API Key

**Author:** Linus (Backend Developer)
**Date:** 2026-04-02
**Branch:** `feature/waza-platform`

## Context

Wired the real ADC Go SDK (`github.com/coreai-microsoft/adc-sdk-go`) into the platform end-to-end. The SDK lives in a mono-repo subdirectory, requiring a local `replace` directive in go.mod.

## Decisions

### 1. GitHub token auth replaces API key

**What:** Removed the `APIKey` field from `ADCConfig` (both `internal/platform/adc/config.go` and `internal/projectconfig/config.go`). ADC now authenticates using the user's GitHub OAuth token via the SDK's `GitHubToken` config field (`Authorization: GitHub {token}` header).

**Why:** The ADC SDK supports three auth methods: API key, bearer token, and GitHub token. Using the user's GitHub token means:
- No platform-level API key to manage/rotate
- Each user's sandbox operations are scoped to their identity
- The token is already available in the request context (from GitHub OAuth)

**Impact:** `ADC_API_KEY` env var is no longer needed. Existing `.waza.yaml` files with `adc.api_key` will have the field silently ignored.

### 2. ADC engine created per-request, not at startup

**What:** Instead of storing a single `*adc.Engine` in `Dependencies`, we store `*adc.ADCConfig`. The `dispatchRun` function creates a new `adc.Engine` per dispatch, which in turn creates ADC clients with the user's GitHub token.

**Why:** ADC authentication is per-user (GitHub token), not platform-level. A singleton client at startup would need the platform's token, which doesn't exist. Creating per-request engines is cheap (just struct initialization) and ensures proper user-scoped auth.

**Impact:** `api.Dependencies.ADCEngine` (type `ADCDispatcher` interface) replaced by `api.Dependencies.ADCConfig` (type `*adc.ADCConfig`). Tests and handlers updated.

### 3. Local replace directive for ADC SDK

**What:** `go.mod` uses `replace github.com/coreai-microsoft/adc-sdk-go => /Users/shboyer/github/adc/client-sdk/go-sdk` because the Go module path doesn't match the repo path (it's in a mono-repo subdirectory).

**Why:** `go get` can't resolve a module whose path (`github.com/coreai-microsoft/adc-sdk-go`) doesn't match the repo root (`github.com/coreai-microsoft/adc`). The local replace lets us develop against the real SDK.

**Impact:** Before merging to main, this replace must be changed to either: (a) the SDK publishes a proper Go module with a `go.mod` at the repo root, or (b) we use a versioned replace pointing to the mono-repo subdirectory.

### 4. ADC execution path in runner.go

**What:** `RunEval` now checks `cfg.ADCEngine != nil` and routes to `runViaADC()` or `runLocal()`. The ADC path creates a sandbox, clones repo inside it, runs waza, reads results, saves to Cosmos, and deletes the sandbox.

**Why:** Clean separation between execution modes. The handler wiring (`dispatchRun`) and status lifecycle (queued → running → complete/failed) are identical for both paths.

### 5. DefaultAPIURL updated to real endpoint

**What:** Changed `DefaultAPIURL` from `"https://adc.dev.azure.com"` to `"https://management.azuredevcompute.io"` (the actual SDK default).

**Why:** Matches the production ADC endpoint used by the SDK.

### 6. CPU expressed in millicores

**What:** `DefaultCPU` changed from `2` (vCPU count) to `2000` (millicores). The SDK expects string format like `"2000m"`.

**Why:** ADC SDK's `CreateFromDiskImageOptions.CPU` is a string in Kubernetes millicore format (e.g., `"2000m"`). Using integer millicores internally avoids float conversion.
