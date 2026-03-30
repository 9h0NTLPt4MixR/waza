# Waza Platform Implementation — Wave 1 Complete

**Session:** 2026-03-30T2100Z  
**Branch:** `feature/waza-platform`  
**Team:** Rusty (Architect), Linus (Backend), Basher (Tests), Saul (Docs)  
**Status:** Wave 1 deliverables merged to decisions.md

## Summary

Wave 1 of the Waza Platform implementation is complete and decision-documented:

### Architecture (Rusty)
- `internal/platform/` contract layer established with four packages: `auth`, `db`, `api`, `adc`
- Go stdlib `http.ServeMux` chosen for API (minimal deps, faster CI)
- User-scoped `Store` interface (every method takes `userID`) prevents cross-user leaks
- ADC quota as constants (policy ≠ config)
- `map[string]any` for polymorphic connection configs (Azure Storage vs GitHub Repo)
- React SPA frontend with `AuthContext` + `AuthGate`, server-side OAuth flow
- Settings page (Connections tab fully functional), New Run 4-step wizard
- 52 existing e2e tests pass through auth gate via mock updates

### Backend (Linus)
- 14 HTTP API handlers: auth (me, logout), connections CRUD+test, runs (trigger/cancel/get), repos (list+evals)
- Full Azure deployment: `azure.yaml`, `Dockerfile.platform`, `infra/main.bicep` (Container Apps + Cosmos DB serverless)
- `--platform` mode in cmd_serve.go: env-var config, healthz endpoint, no browser auto-open
- ADCDispatcher interface (not direct SDK import) — allows clean compile until SDK lands
- Hand-rolled JWT (30 LOC, HS256 only, lighter than golang-jwt/v5)
- Cosmos DB partition key = GitHub ID (string)
- In-memory session revocation (v1 sufficient, Redis for scale)
- Async goroutine dispatch for runs (202 response, ADC integration stubbed)
- All 13 platform API tests passing

### Testing (Basher)
- 50+ test cases written and passing
- Fixture isolation: each task gets fresh temp workspace, originals never modified
- Playwright e2e tests updated for auth gate compatibility

### Documentation (Saul)
- All platform decisions documented with clear rationale
- Ready for external documentation updates post-merge

## Key Decisions Captured

1. **Architecture contracts** — Interface-based design, user-scoped queries, minimal deps
2. **Frontend auth** — `AuthContext` + server-side OAuth (no client-side token dance)
3. **ADC integration** — Interface-based dispatch, SDK integration deferred until SDK in go.mod
4. **Cosmos DB** — Serverless mode, partition by GitHub ID
5. **Deployment** — 12-factor env vars, no .waza.yaml secrets, healthz probe
6. **JWT** — Hand-rolled minimal implementation (upgrade path documented)
7. **PR conflict resolution** — Merge main into branch when published, preserve newer main code
8. **Token limits precedence** — YAML always authoritative over legacy .token-limits.json

## Files Modified/Created

**Backend:**
- `internal/platform/auth/github.go` — OAuth + JWT
- `internal/platform/db/cosmos.go` — Cosmos client + Store impl
- `internal/platform/api/handlers.go` — 14 handlers
- `internal/platform/api/deps.go` — DI struct
- `cmd/waza/cmd_serve.go` — `--platform` mode
- `azure.yaml`, `Dockerfile.platform`, `infra/*.bicep` — Deployment

**Frontend:**
- `web/src/contexts/AuthContext.tsx` — Auth state + useAuth hook
- `web/src/components/AuthGate.tsx` — Login page + auth gate
- `web/src/components/Settings.tsx` — Connections CRUD
- `web/src/components/NewRun.tsx` — Run wizard
- `web/src/api/client.ts` — 10 new platform endpoints
- `web/src/hooks/useApi.ts` — 9 React Query hooks
- `web/src/App.tsx`, `Layout.tsx`, `main.tsx` — Routing + auth integration

## Next Steps

- Wave 2: Rusty (React frontend polish), Linus (14 API handlers, more ADC), Basher (integration tests)
- Post-merge: External docs updates (Saul)
- ADC SDK lands → swap interface for concrete type
- Multi-instance deployment → upgrade in-memory revocation to Redis

## Decisions Inbox Processed

✅ 7 inbox files merged into `.squad/decisions.md`:
- `copilot-directive-mixed-formats.md` → Teams notifications decision
- `linus-platform-api.md`, `linus-platform-backend.md` → Backend details
- `linus-pr-conflicts.md`, `linus-pr96-feedback.md` → Linus technical decisions
- `rusty-platform-architecture.md`, `rusty-platform-frontend.md` → Architecture + frontend

✅ Inbox cleaned (all files deleted)

## End State

- `.squad/decisions.md` updated with 7 new decisions (2026-03-30)
- `.squad/decisions/inbox/` cleaned
- Session log written
- Ready for commit and post-merge documentation phase
