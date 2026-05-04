# Codex Auth 文件导入 PRD

## 1. 背景与目标

Octopus 已支持 Codex OAuth 渠道和 usage card。当前 Codex 凭证只能通过浏览器 OAuth 流程逐个添加，无法批量导入。

CLIProxyAPI (CPA) 是同类项目，使用文件系统存储 auth 凭证（`.json` 文件），支持 `.zip` 压缩包一键批量导入。许多用户已有 CPA 格式的 auth 文件集合。

**目标**：在 Codex 渠道内增加 CPA 兼容的 auth 文件导入能力，支持 `.json` 单文件/多文件和 `.zip` 压缩包批量导入。

## 2. CPA 行为调研摘要

基于对 `router-for-me/CLIProxyAPI` 的只读调查：

| 特性 | CPA 行为 |
|------|---------|
| 文件格式 | 扁平 JSON，文件名即凭证标识，`.json` 后缀强制 |
| 批量上传 | multipart/form-data，多文件独立处理 |
| ZIP 支持 | 上传 `.zip` 自动解压，仅提取 `.json`，basename 扁平化 |
| 导入验证 | 解析 JSON + metadata 入库，不主动调用上游验证或刷新 token |
| 重复处理 | 同名文件 upsert（覆盖更新，保留 CreatedAt） |
| 部分失败 | 保留成功项，失败项在响应中报告 |
| JWT 解析 | Codex `id_token` 本地无签名解析，提取 `chatgpt_account_id`/`plan_type` |
| 安全 | 禁止路径穿越、强制 `.json` 后缀、`filepath.Base()` 剥离路径 |

## 3. 文件格式

### 3.1 Codex Auth JSON

```json
{
  "type": "codex",
  "id_token": "eyJhbGciOi...",
  "access_token": "eyJhbGciOi...",
  "refresh_token": "v1.xxx",
  "account_id": "acct_xxxx",
  "email": "user@example.com",
  "last_refresh": "2026-05-04T12:00:00Z",
  "expired": "2026-05-04T13:00:00Z",
  "disabled": false,
  "priority": 1,
  "note": "optional note",
  "proxy_url": "socks5://127.0.0.1:1080",
  "headers": {
    "X-Custom-Header": "value"
  }
}
```

### 3.2 字段说明

**必需字段**：
- `type` — 必须为 `"codex"`，否则拒绝
- `refresh_token` — 必须存在，缺失则导入失败

**核心凭证字段**：
- `id_token` — JWT，可缺失但标记 `incomplete`
- `access_token` — 可缺失但标记 `incomplete`
- `account_id` — 账号标识
- `email` — 显示用
- `expired` — 过期时间，支持 RFC3339 字符串和 Unix 时间戳

**可选控制字段**：
- `disabled` (bool)
- `priority` (int)
- `note` (string)
- `proxy_url` (string)
- `prefix` (string)
- `headers` (map[string]string)
- `request_retry` (int)
- `disable_cooling` (bool)
- `tool_prefix_disabled` (bool)

**时间字段兼容**：`last_refresh` 支持 4 种 key 名：
- `last_refresh`
- `lastRefresh`
- `last_refreshed_at`
- `lastRefreshedAt`

### 3.3 ZIP 包规则

- 仅支持 `.zip`，不支持 `.tar`、`.gz`、`.tar.gz`
- zip 内仅处理 `.json` 文件，其他文件跳过并在结果中标记 `skipped`
- 目录结构不保留：`a/b/codex.json` 按 `codex.json` 处理
- Zip Slip 防护：拒绝 `../`、绝对路径、Windows 卷名、空 basename
- 单文件大小限制 1MB；zip 展开文件数限制 50；总展开 JSON 大小限制 10MB
- zip 内 basename 重复时，第一次成功项为准，后续标记 `duplicate_in_archive`

## 4. Octopus 内部映射

| CPA 字段 | Octopus 映射 | 说明 |
|----------|-------------|------|
| `type` | 校验用 | 必须为 `codex` |
| `refresh_token` | credential 加密存储 | 必需 |
| `access_token` | credential 加密存储 | 可缺失 |
| `id_token` | credential 加密存储 | 可缺失，用于本地解析 email/account_id |
| `email` | key label | 优先级：JSON `email` > JWT claims > `account_id` > 文件名 |
| `account_id` | key metadata | 优先级：JSON `account_id` > JWT `chatgpt_account_id` |
| `expired` | key 过期时间 | 支持 RFC3339 和 Unix 时间戳 |
| `last_refresh` | key metadata | 兼容 4 种 key 名 |
| `disabled` | key metadata | 保存但不影响运行时（本期） |
| `priority`/`note`/`proxy_url`/`headers` | key metadata | 保存但不影响运行时（本期） |

## 5. UI 设计

### 5.1 入口位置

Codex 渠道表单内的 OAuth/auth 凭证区域，与现有"授权添加"按钮同级。

### 5.2 操作区

- **授权添加**：现有 OAuth 流程（不变）
- **导入 auth 文件**：新增按钮，视觉风格与现有小型添加按钮一致
- 文件选择：`accept=".json,.zip"`，支持多选
- 辅助说明：`支持 CPA/CLIProxyAPI Codex auth JSON 或 ZIP，可批量导入`

### 5.3 导入预览

选择文件后展示紧凑表格：

| 文件 | 来源 | 账号 | Account ID | 状态 |
|------|------|------|-----------|------|
| `codex-a.json` | JSON | `a@example.com` | `acct_xxx` | 可导入 |
| `auths.zip/a.json` | ZIP | `a@example.com` | `acct_xxx` | 可导入 |
| `bad.json` | JSON | `-` | `-` | 缺少 refresh_token |

前端可做轻量 JSON 解析预览，但以后端校验结果为准。

### 5.4 导入结果

- 成功数、更新数、失败数、跳过数
- 每项结果状态：`imported`、`updated`、`failed`、`skipped`、`duplicate_in_archive`、`incomplete`
- 成功导入后刷新凭证列表

### 5.5 凭证列表显示

每个凭证：
- Badge：`OAuth` 或 `Auth 文件`
- 主信息：email，小字号
- 副信息：account_id、过期时间、来源文件名
- 操作：删除
- 不显示完整 token

## 6. API 设计

### 6.1 导入接口

```http
POST /api/v1/channel/:id/codex/auth-files/import
Content-Type: multipart/form-data
files: one or many .json/.zip files
```

兼容 CPA 风格 `file` 单字段上传 `.json` 或 `.zip`。

### 6.2 响应格式

```json
{
  "success": true,
  "data": {
    "imported": 2,
    "updated": 1,
    "failed": 1,
    "skipped": 1,
    "results": [
      {
        "file": "codex-a.json",
        "source": "json",
        "status": "imported",
        "email": "a@example.com",
        "account_id": "acct_xxx"
      },
      {
        "file": "auths.zip/bad.txt",
        "source": "zip",
        "status": "skipped",
        "error": "not a json file"
      },
      {
        "file": "bad.json",
        "source": "json",
        "status": "failed",
        "error": "missing refresh_token"
      }
    ]
  }
}
```

### 6.3 语义

- 多文件独立处理，非事务
- 全部成功返回 200
- 部分失败仍返回 200，`results` 中表达失败项
- 单文件完全失败返回 400
- 同 channel 内按 `account_id` 去重；无 account_id 时按 `email + refresh_token hash` 去重
- 无效 JSON 不覆盖现有凭证

## 7. 后端处理流程

```
1. 接收 multipart 请求
2. 遍历所有上传文件
3. 对每个文件：
   a. 检测是否为 .zip
   b. 如果是 zip → 解压 → 遍历 .json 条目（basename 扁平化 + 安全过滤）
   c. 如果是 .json → 直接处理
4. 对每个 .json：
   a. 解析 JSON
   b. 校验 type == "codex"
   c. 校验 refresh_token 存在
   d. 解析 id_token JWT（本地，不验签）提取 email/account_id/plan_type
   e. 合并 email/account_id（JSON 优先 > JWT）
   f. 构建凭证 JSON（与现有 Codex OAuth credential 格式一致）
   g. 加密存储
   h. Upsert 到 channel keys（按 account_id 去重）
5. 汇总结果返回
```

## 8. 安全要求

- 后缀白名单：`.json`、`.zip`
- 文件大小限制：单文件 1MB，zip 展开总大小 10MB
- 展开数量限制：50 个文件
- Zip Slip 防护：拒绝危险路径
- token 类字段入库前走现有加密逻辑
- API/UI/log 只展示脱敏值或 metadata，不回显完整 token
- 导入错误信息不得包含 token 原文

## 9. 实施拆分

1. **主 agent**：创建本 PRD 文档
2. **implementer**：按 PRD 实现
   - 后端：Codex auth JSON parser/normalizer、ZIP 展开逻辑、导入 API、channel key upsert
   - 前端：文件选择器（accept .json/.zip）、导入预览、导入结果、凭证列表展示
   - i18n 文案
   - 测试
3. **checker**：按 PRD 逐项验收
4. **主 agent**：根据 checker 结果修复遗漏，跑最终 validators

## 10. 验收清单（Checker 用）

- [ ] 能导入单个 CPA Codex `.json` auth 文件
- [ ] 能多选 `.json` 批量导入
- [ ] 能导入 CPA 风格 `.zip`，正确展开其中 `.json`
- [ ] zip 内非 JSON 文件标记 `skipped`
- [ ] zip 内危险路径被拒绝
- [ ] zip 内重复 basename 标记 `duplicate_in_archive`
- [ ] 无效 JSON 不覆盖现有凭证
- [ ] `type` 非 `codex` 被拒绝
- [ ] 缺少 `refresh_token` 被拒绝
- [ ] 缺少 `access_token`/`id_token` 标记 `incomplete` 但仍导入
- [ ] `email` 优先级正确：JSON > JWT > account_id > 文件名
- [ ] `account_id` 优先级正确：JSON > JWT
- [ ] `last_refresh` 兼容 4 种 key 名
- [ ] `expired` 支持 RFC3339 和 Unix 时间戳
- [ ] 重复账号更新，不产生不可识别重复项
- [ ] 导入后凭证列表显示 email/account_id/来源/过期时间
- [ ] 导入后凭证列表不泄露完整 token
- [ ] 导入后 Codex outbound/usage card 能继续使用该 credential
- [ ] UI 入口在渠道表单 Codex 凭证区域内
- [ ] 文件选择器 accept=".json,.zip"
- [ ] 导入结果展示成功/失败/跳过数
- [ ] 前端 `pnpm run build` 通过
- [ ] 后端相关 Go tests 通过
- [ ] 日志中无 token 原文
