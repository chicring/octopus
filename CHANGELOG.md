# Changelog

## v0.1.2 - Channel Key Metadata and Group Guide by Ying Xinyao

### Feature: 分组模式说明与 Key 级调度能力

- **Issue**: 分组模式只有界面枚举，用户无法快速判断轮询、随机、故障转移、加权分配分别适合什么业务场景；同一渠道多个 Key 支持模型不同，路由仍可能把请求发给错误 Key。
- **Trigger**: 用户在一个渠道内配置多个密钥，并且密钥分别支持不同模型；或者需要在成本、稳定性、主备关系不同的渠道之间选择分组策略。
- **Root Cause**: 原渠道 Key 元数据只记录密钥本身、启用状态和统计值，没有 Key 类型、倍率、模型白名单；分组策略也缺少独立运维说明文档。
- **Fix**:
  - 新增 `docs/group-balancing.md`，说明四种分组模式的适用场景、配置要点和 RCA。
  - `ChannelKey` 新增 `is_cli`、`multiplier`、`models` 字段。
  - 渠道内 Key 选择按模型白名单过滤，并用 `score = total_cost * multiplier` 作为最低成本选择评分。
  - 单 Key 测试会识别模型白名单，不再把不支持目标模型的 Key 判为可用。

### Fix: CLI 凭证与普通模型刷新隔离

- **Issue**: CLI 渠道密钥使用普通模型列表刷新或普通连通性测试时，可能被当作 Bearer API Key 使用，返回 401/404 或无法完成测试。
- **Trigger**: 渠道 Key 是 Codex/CLI OAuth JSON 凭证，用户点击渠道编辑里的“刷新模型”或在非 CLI Provider 下测试。
- **Root Cause**: 前端刷新模型请求没有传 `provider_id`，后端模型列表刷新也没有区分 CLI 凭证和普通 API Key。
- **Fix**:
  - 刷新模型和按配置测试时提交 `provider_id`。
  - 模型列表刷新只使用非 CLI Key；CLI-only 渠道返回明确错误，提示手动配置模型或添加普通 API Key。
  - Codex OAuth/Auth 文件导入的 Key 自动标记为 CLI Key。

### Feature: 渠道官网与用量查询预设配置

- **Issue**: 渠道编辑缺少官网入口、Key 倍率和渠道级用量查询模板配置，无法记录不同供应商的用量查询规则。
- **Trigger**: 用户需要为渠道保存官网、通用/NewAPI/tokenplan/custom 用量查询模板、超时、自动查询间隔和 extractor 代码。
- **Root Cause**: 渠道模型没有预留官网和用量查询配置字段，前端编辑表单也没有这些配置入口。
- **Fix**:
  - `Channel` 新增 `official_url` 与 `usage_query` JSON 配置。
  - 编辑渠道新增官网、用量查询预设、请求地址、手动 API Key、Access Token、User ID、超时、自动查询间隔、模板代码和 extractor 代码字段。
  - 该配置先作为渠道元数据持久化，后续接入现有用量卡片刷新链路执行。

### Feature: 首字超时推荐值

- **Issue**: 分组首字超时只能手动填写固定秒数，用户需要根据近期真实请求表现判断合理阈值。
- **Trigger**: 用户编辑分组时，希望用最近几天同模型的首字时间数据作为配置参考。
- **Root Cause**: 原分组配置没有对 RelayLog 的 `ftut` 首字时间做聚合分析，界面无法把最近性能分布转化为可操作建议。
- **Fix**:
  - 新增 `/api/v1/group/first-token-timeout/recommend` 接口，按选中模型统计最近 3 天成功请求的首字时间。
  - 使用 P95 分位数乘以 1.25 缓冲系数，向上取整为推荐秒数，避免平均值被极端样本污染。
  - 分组编辑器新增“推荐”按钮，展示样本量、P95 和一键套用建议值。

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
