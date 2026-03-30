# Decision: Platform Frontend — Wave 2 React SPA

**By:** Rusty (Lead / Architect)
**Date:** 2026-07
**Branch:** `feature/waza-platform`

## What

Implemented Wave 2 platform frontend: auth gate, auth context, Settings page (connections management), New Run page (multi-step wizard), updated navigation with user menu, and full API client extensions for platform endpoints.

## Key Decisions

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

## Files Created
- `web/src/contexts/AuthContext.tsx` — AuthProvider + useAuth hook
- `web/src/components/AuthGate.tsx` — Login page + loading screen + auth gate
- `web/src/components/Settings.tsx` — Full settings page with connections CRUD
- `web/src/components/NewRun.tsx` — 4-step run creation wizard

## Files Modified
- `web/src/api/client.ts` — 10 new API functions + 7 new types
- `web/src/hooks/useApi.ts` — 9 new React Query hooks (queries + mutations)
- `web/src/components/Layout.tsx` — User menu, New Run button, Settings link
- `web/src/App.tsx` — 2 new routes (`#/settings`, `#/runs/new`)
- `web/src/main.tsx` — AuthProvider + AuthGate wrapping
- `web/e2e/helpers/api-mock.ts` — Auth mock for test compatibility
