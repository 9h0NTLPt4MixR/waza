# Platform Module — `internal/platform/`

The platform module contains the server-side contracts for Waza Platform, a hosted
PaaS that evolves `waza serve` into a multi-tenant web application. These packages
define the interfaces that backend implementations build against.

## Architecture

```mermaid
graph TD
    subgraph Client Layer
        CLI[waza CLI]
        WebUI[React Dashboard]
    end

    subgraph API Layer
        Router[api/routes.go<br/>Chi Router]
        AuthMW[auth/Middleware<br/>Session validation]
    end

    subgraph Auth
        AuthProv[auth/AuthProvider<br/>GitHub OAuth]
        Session[auth/Session<br/>Token management]
    end

    subgraph Data Layer
        Store[db/Store<br/>Cosmos DB]
        Conn[db/Connection<br/>GitHub Repo · Azure Storage]
        Run[db/RunRequest<br/>Eval lifecycle]
    end

    subgraph Execution Layer
        ADC[adc/ADCConfig<br/>Sandbox orchestration]
        Engine[execution/AgentEngine<br/>Existing eval engine]
    end

    CLI -->|HTTP| Router
    WebUI -->|HTTP| Router
    Router --> AuthMW
    AuthMW --> AuthProv
    AuthMW --> Store
    Router -->|authenticated| Store
    Router -->|trigger run| ADC
    ADC --> Engine
    Store --> Conn
    Store --> Run
```

## Packages

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `auth`  | GitHub OAuth, sessions, HTTP middleware | `AuthProvider`, `User`, `Session`, `Middleware` |
| `db`    | Data persistence contracts (Cosmos DB) | `Store`, `Connection`, `RunRequest` |
| `api`   | HTTP route registration and handler stubs | `RegisterRoutes` |
| `adc`   | Azure Dev Compute sandbox configuration | `ADCConfig`, defaults |

## Design Principles

1. **Interface-first.** Every package exports interfaces, not implementations. Linus
   wires up the concrete Cosmos/GitHub/ADC backends against these contracts.
2. **Single-user isolation.** No team/org model in v1. Every resource is scoped to a
   `UserID`. Team sharing is a v2 concern.
3. **BYOS (Bring Your Own Storage).** Users connect their own Azure Storage account
   for eval artifacts. Waza stores only metadata.
4. **Quota enforcement.** ADC sandbox limits (max 10 per user) are encoded as
   constants, not config. Changing them requires a code change and review.

## Data Flow — Eval Run

```mermaid
sequenceDiagram
    actor User
    participant API as api/routes
    participant Auth as auth/AuthProvider
    participant DB as db/Store
    participant ADC as adc/Config
    participant Engine as AgentEngine

    User->>API: POST /api/runs/trigger
    API->>Auth: ValidateSession(token)
    Auth-->>API: User
    API->>DB: CreateRunRequest(req)
    DB-->>API: RunRequest (Queued)
    API->>ADC: Allocate sandboxes
    ADC->>Engine: Execute eval tasks
    Engine-->>ADC: Results
    ADC->>DB: UpdateRunRequest (Complete)
    DB-->>User: Results via dashboard
```
