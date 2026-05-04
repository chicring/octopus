# Repository Guidelines

## Project Structure & Module Organization

```
octopus/
├── main.go                    # Entry point
├── internal/
│   ├── relay/                 # Core relay: request parsing, balancing, SSE streaming
│   │   └── balancer/          # Load balancing (round-robin/random/failover/weighted), circuit breaker, session affinity
│   ├── transformer/           # Protocol translation (inbound/outbound adapter pattern)
│   │   ├── inbound/           # Client formats → canonical InternalLLMRequest (openai, anthropic)
│   │   ├── outbound/          # Canonical → upstream formats (openai, authropic, gemini, volcengine, codex)
│   │   └── model/             # Shared canonical models
│   ├── server/                # HTTP server (Gin), handlers, middleware, router
│   ├── model/                 # GORM models
│   ├── op/                    # CRUD operations on models
│   ├── conf/                  # Viper config (data/config.json + OCTOPUS_ env overrides)
│   ├── db/migrate/            # Versioned DB migrations with AutoMigrate hooks
│   ├── task/                  # Background task scheduler (model sync, stats flush, log cleanup)
│   ├── provider/              # Provider registration and auth
│   ├── price/                 # Price presets (auto-generated Go from JSON)
│   └── usagecard/             # Usage card generation
├── web/                       # Frontend: Next.js 16 (static export), React 19, shadcn/ui, Tailwind v4, zustand
│   └── src/                   # Path alias: @/* → ./src/*
├── static/                    # Frontend build output (Go embed via //go:embed)
├── scripts/build.sh           # Release build script (frontend → embed → Go binary)
└── data/                      # Runtime data (config.json, SQLite DB)
```

## Build, Test, and Development Commands

```bash
# Backend
go run main.go start                          # Run in dev mode
OCTOPUS_DEBUG=true go run main.go start       # Debug mode (verbose Gin + GORM logging)
CGO_ENABLED=0 go build -o octopus -ldflags="-s -w" -tags=jsoniter .  # Production build
go test ./internal/transformer/model/...      # Run existing tests
go test ./internal/relay/...                  # Relay tests (client_detect, inbound_reset, stats_count)

# Frontend
cd web && pnpm install                        # Install dependencies
cd web && pnpm run dev                        # Dev server (localhost:3000)
cd web && NEXT_PUBLIC_API_BASE_URL="http://127.0.0.1:8080" pnpm run dev  # With backend
cd web && pnpm run build                      # Static export → web/out/
cd web && pnpm run lint                       # ESLint

# Full Release
bash scripts/build.sh build <os> <arch>       # Single platform (e.g. linux arm64)
bash scripts/build.sh release                 # All platforms + Docker images
```

## Coding Style & Naming Conventions

- **Go**: Follow standard Go conventions. Use `jsoniter` build tag for JSON serialization. Comments primarily in Chinese (中文).
- **Frontend**: TypeScript strict mode, React 19, shadcn/ui components, Tailwind CSS v4. Path alias `@/*` → `./src/*`.
- **Adapters**: Registered in factory maps via `init()` functions, looked up by type string.
- **Routes**: Declarative registration via `router.NewGroupRouter()` and `router.NewRoute()` in `init()` functions.
- **Config**: Environment variable overrides use `OCTOPUS_` prefix. Build-time vars via ldflags: `conf.Version`, `conf.Commit`, `conf.BuildTime`.
- **Important**: The outbound Anthropic adapter directory is `authropic` (typo) — do not rename without updating all references.

## Testing Guidelines

- Go tests use standard `testing` package. Run with `go test ./<path>/...`.
- Key test files: `internal/relay/client_detect_test.go`, `internal/relay/inbound_reset_test.go`, `internal/relay/stats_count_test.go`, `internal/transformer/model/`.
- Frontend: ESLint via `pnpm run lint`. No unit test framework currently configured.

## Commit & Pull Request Guidelines

- **One topic per PR** — single feature or single bug fix only. Split multiple topics into separate PRs.
- **Commit messages**: Use conventional prefixes — `fix:`, `feat:`, `chore:` followed by a concise description (often in Chinese).
- **AI-assisted code**: Allowed but must be human-reviewed before submission.
- **Pre-submission checklist**: PR contains only one change topic; AI-generated content has been reviewed.

## Release & Tag 发布流程

CI 流水线（`.github/workflows/release.yaml`）会自动构建和发布，**不需要手动构建**。

### 正确步骤

1. 在 `dev` 分支上开发、测试、提交
2. 合并 `dev` → `master`（`git checkout master && git merge dev` 或通过 PR）
3. 在 `master` 上打 tag：`git tag v1.x.x && git push origin v1.x.x`
4. CI 自动构建 → 上传 GitHub Release 附件 + 推 Docker 镜像

### 注意事项

- **不要在 `dev` 上手动 `gh release create`**，会跟 CI 流水线冲突产生 Draft
- **打 tag 前先确认最新版本号**：`git tag --sort=-v:refname | head -5`
- push 到 `dev` 只触发 Docker 镜像构建（tag: `dev`, `short_sha`），不上传 Release 附件
- push 到 `master` + tag 才触发完整发布（Release 附件 + Docker）

## Architecture Overview

Request flow: `Client → Gin Router → Relay Handler → Inbound Transformer → Balancer → Outbound Transformer → Upstream Provider` (response follows the reverse path). The transformer layer uses an adapter pattern with inbound (client format → canonical) and outbound (canonical → provider format) converters. The balancer supports round-robin, random, failover, and weighted strategies with circuit breaking and session affinity.
