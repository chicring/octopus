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

### CI 流水线行为（`.github/workflows/release.yaml`）

| 触发条件 | Docker 镜像 | GitHub Release |
|----------|-------------|----------------|
| push `dev` | `dev`, `<short_sha>` | **不创建** |
| push `master` | `latest`, `<tag>`, `<short_sha>` | **创建**（tag_name = 最近 tag） |

**关键**：CI 由 **push 分支** 触发，不是 tag push。所以 push master 就会触发完整发布流程。

### 发布步骤（严格按顺序执行）

```bash
# 1. 确认在 dev 分支，所有修改已提交
git checkout dev
git status

# 2. 查看最新 tag，确定新版本号（当前最新 +1，绝不重复）
git tag --sort=-v:refname | head -5

# 3. 在 dev 上打 tag（tag 必须在 push master 之前创建，CI 用 git describe 获取 tag）
git tag v1.x.x

# 4. 确保 master 存在，合并 dev
git checkout master
git merge dev

# 5. 推送 master + tag（CI 自动构建发布）
git push origin master
git push origin v1.x.x

# 6. 切回 dev 继续开发
git checkout dev
```

### 禁止事项

- **禁止重复打已有 tag**：`git tag` 前必须 `git tag --sort=-v:refname | head -5` 确认版本号
- **禁止 `gh release create`**：会跟 CI 冲突产生 Draft
- **禁止 `git tag -d` + 重建同名 tag**：已推送的 tag 不可变，只能打新版本号
- **禁止在 dev 上手动构建发布**：push dev 只出 Docker 镜像，不出 Release

### 常见问题

- **Draft Release**：CI 用 `git describe --tags --abbrev=0` 找最近 tag，如果 tag 不在当前 commit 上，会创建 Draft。解决：确保 tag 在 push master 之前创建，且指向正确 commit。
- **Release 版本号不对**：CI 把 `git describe` 结果当 tag_name，如果最新 tag 不是你想要的版本，Release 名就会错。解决：发布前确认 `git describe --tags --abbrev=0` 输出正确。

## Architecture Overview

Request flow: `Client → Gin Router → Relay Handler → Inbound Transformer → Balancer → Outbound Transformer → Upstream Provider` (response follows the reverse path). The transformer layer uses an adapter pattern with inbound (client format → canonical) and outbound (canonical → provider format) converters. The balancer supports round-robin, random, failover, and weighted strategies with circuit breaking and session affinity.
