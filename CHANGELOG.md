# Changelog

## v0.1.1 - Gemini Native Protocol Fix by Ying Xinyao

### Fix: Gemini 原生协议模型列表与内容生成 404

- **Issue**: 使用 Gemini CLI/SDK 通过 Gemini 原生协议访问 Octopus 时，`/v1beta/models` 获取不到模型，`/v1beta/models/{model}:generateContent` 返回 404。
- **Trigger**: 客户端使用 Gemini API 兼容路径和 `?key=` / `x-goog-api-key` 鉴权访问代理，而不是 OpenAI `/v1/chat/completions` 或 Anthropic `/v1/messages` 入口。
- **Root Cause**: 服务端只注册了 OpenAI/Anthropic 入站路由；虽然已有 Gemini 出站 provider 和 `/models` 上游拉取逻辑，但没有 Gemini 原生入站 adapter、`/v1beta/models` 模型列表响应、Gemini 路径模型名解析和 Gemini API Key 鉴权分支，导致请求在 Gin 路由层或鉴权层提前失败。
- **Fix**:
  - 新增 `internal/transformer/inbound/gemini/messages.go`，支持 Gemini `contents` 请求解析、响应序列化和 SSE 输出。
  - 注册 `InboundTypeGemini` 工厂，并在 relay 中读取 Gemini 路径里的模型名和 stream 方法。
  - 新增 `/v1beta/models`、`/v1beta/models/{model}:generateContent`、`/v1beta/models/{model}:streamGenerateContent` 路由。
  - `APIKeyAuth` 增加 `x-goog-api-key` 和 `?key=` 兼容，模型列表按 Gemini `models/{name}` 格式返回。

### Release: 自用仓库版本与交付链路

- **Issue**: 版本信息、更新检查地址、Docker 镜像名和 README 仍指向 upstream 仓库与 Docker Hub 镜像。
- **Trigger**: 在 `loserrc/octopus` 自用仓库中发布时，应用内更新检查会访问 `chicring/octopus`，GitHub Actions 会推送 `chruxc/octopus`。
- **Root Cause**: 仓库迁移后，发布元数据没有集中更新，CI 仍采用 upstream 的 dev/tag 发布约定和 Docker Hub 目标。
- **Fix**:
  - 更新默认版本为 `v0.1.1`，作者为 `Ying Xinyao`，仓库地址为 `https://github.com/loserrc/octopus`。
  - 更新自动检查更新接口到 `loserrc/octopus` Releases。
  - 发布 workflow 支持 `yingxinyao/gemini-native-release` 分支、tag 和手动触发。
  - GitHub Release 仅上传 Linux `x86_64` 与 `arm64` 二进制压缩包，Docker 镜像推送到 `ghcr.io/loserrc/octopus` 的 `linux/amd64` 与 `linux/arm64` 多架构镜像。

## Previous Changes Carried Forward

- **fix**: 收敛 relay 日志记录语义。
- **fix**: 修复 DeepSeek 数组内容 thinking 回传。
- **fix**: 保留空 thinking 以兼容 DeepSeek 回传。
- **fix**: 避免透传复写破坏缓存前缀。
- **refactor**: 收敛 `reasoning_content` 回传兼容逻辑。
- **fix**: 修复 DeepSeek 非透传 thinking 回传。
- **fix**: 修复 DeepSeek thinking 包裹内容回传。
- **fix**: 流式响应无 usage 时正确结束 Anthropic 消息。
- **fix**: 不为 DeepSeek reasoning 合成 thinking signature。
- **fix**: 保留 DeepSeek 工具调用 `reasoning_content`。
- **fix**: 修复日志详情加载卡住。
- **feat**: 优化响应日志展示。
