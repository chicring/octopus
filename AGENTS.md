# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Project Overview

Octopus is an LLM API aggregation and load balancing service. It accepts requests in OpenAI Chat/Responses/Embeddings, Anthropic Messages, and Gemini API formats, then routes them to upstream LLM providers with protocol conversion, load balancing (round-robin/random/failover/weighted), circuit breaking, and session affinity.

- **Backend**: Go 1.24, Gin HTTP framework, GORM ORM (SQLite/MySQL/PostgreSQL)
- **Frontend**: Next.js 16 (static export), React 19, shadcn/ui, Tailwind CSS v4, zustand
- **License**: AGPL-3.0

## Build & Development Commands

### Backend

```bash
# Run in development
go run main.go start

# Debug mode (verbose Gin + GORM logging)
OCTOPUS_DEBUG=true go run main.go start

# Build binary (CGO-free, uses jsoniter for faster JSON)
CGO_ENABLED=0 go build -o octopus -ldflags="-s -w" -tags=jsoniter .

# Run the one existing test
go test ./internal/transformer/model/...
```

### Frontend (web/)

```bash
cd web
pnpm install
pnpm run dev                                          # Dev server on localhost:3000
NEXT_PUBLIC_API_BASE_URL="http://127.0.0.1:8080" pnpm run dev  # With backend
pnpm run build                                        # Static export to web/out/
pnpm run lint                                         # ESLint
```

### Full Release Build

```bash
bash scripts/build.sh build <os> <arch>   # e.g. linux arm64
bash scripts/build.sh release              # All platforms + Docker images
```

The build script: builds frontend → copies `web/out/` to `static/out/` (Go embed) → updates price presets → compiles Go binary with version ldflags.

## Architecture

### Request Flow

```
Client → Gin Router → Relay Handler → Inbound Transformer → Balancer → Outbound Transformer → Upstream Provider
                                                                                                    ↓
Client ← Inbound Transformer ← InternalLLMResponse ← Outbound Transformer ← HTTP Response ←────────┘
```

### Key Packages

- **`internal/relay/`** — Core relay logic: request parsing, balancing, forwarding, SSE streaming
- **`internal/transformer/`** — Protocol translation layer with inbound/outbound adapter pattern
  - `inbound/` — Client-facing formats → canonical `InternalLLMRequest` (openai, anthropic adapters)
  - `outbound/` — Canonical format → upstream provider formats (openai, authropic, gemini, volcengine)
  - Adapters are registered in factory maps via `init()` and looked up by type string
- **`internal/relay/balancer/`** — Load balancing strategies, circuit breaker (exponential backoff), session affinity (sticky routing per API key)
- **`internal/server/router/`** — Declarative route registration via `init()` functions using `router.NewGroupRouter()` and `router.NewRoute()`
- **`internal/task/`** — Generic background task scheduler for periodic jobs (model sync, stats flush, log cleanup, price updates)
- **`internal/model/`** + **`internal/op/`** — GORM models and CRUD operations
- **`internal/conf/`** — Viper-based config from `data/config.json` with `OCTOPUS_` env var overrides
- **`internal/db/migrate/`** — Versioned migration system with before/after AutoMigrate hooks
- **`static/`** — Frontend build output embedded into Go binary via `//go:embed`

### Configuration

- Config file: `data/config.json` (auto-generated on first run)
- Environment variable overrides use `OCTOPUS_` prefix (e.g., `OCTOPUS_SERVER_PORT`, `OCTOPUS_DATABASE_TYPE`)
- Build-time variables injected via ldflags: `conf.Version`, `conf.Commit`, `conf.BuildTime`

## Contribution Rules

- One topic per PR (single feature or single bug fix)
- AI-assisted code is allowed but must be human-reviewed before submission

## Coding Standards

- 详细规则：`.factory/rules/file-reading.md`
- 项目记忆：`.factory/memories.md`

## Notable Details

- The outbound Anthropic adapter directory is named `authropic` (typo) — do not rename without updating all references
- Frontend uses `output: "export"` (pure static HTML, no SSR) with `reactCompiler: true`
- Frontend path alias: `@/*` → `./src/*`
- Builds use `CGO_ENABLED=0` and `-tags=jsoniter` for pure-Go builds with faster JSON serialization
- Comments and documentation are primarily in Chinese (中文)
