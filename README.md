# 拾卷 · Novel Studio

一个面向长篇创作的 AI 小说工作台：把人物关系、世界设定与章节正文组织在一起，并通过可审核的
生成任务与可视化工作流，让 AI 参与到实际写作流程中。

- **后端**：Go 1.27 + `net/http` + SQLite（`modernc.org/sqlite`，纯 Go 无 CGO）
- **前端**：Vue 3 + TypeScript + Vite
- **实时能力**：任务事件通过 WebSocket 流式推送，支持断线重连与游标回放

## 功能概览

| 模块 | 说明 |
| --- | --- |
| 账户与工作区 | 邮箱注册/登录（会话 Cookie + HMAC 签名），工作区角色：所有者 / 编辑 / 审阅者 / 只读 |
| 作品库 | 创建小说项目，设置题材、计划章节数与 Agent 写入模式（人工审核 / 自动写入） |
| 回收站 | 作品与章节可移入回收站并由编辑/所有者恢复；工作流可删除 |
| 写作指南 | 作品级文风与硬性约束（建议用「禁止…」「必须…」的负向指令），每次生成都会注入 |
| 章节蓝图 | 每章的目标、关键事件与章末钩子；只改蓝图不影响正文版本号。生成时注入上一章结尾，并按蓝图点名的人物、地点挑选相关图谱与未回收伏笔 |
| 章节编辑 | 章节正文编辑、2.5 秒防抖自动保存与本地草稿缓存、版本冲突对比、历史版本记录与恢复 |
| 知识图谱 | 人物 / 地点 / 组织 / 物件 / 设定实体及关系。生成时只注入本章蓝图点名的人物、地点及相关关系，不再按固定条数截取前缀 |
| 创作工作流 | 可视化编排「资料 → 生成 → 自检 → 输出」节点，并按依赖顺序逐个执行。资料节点不调用模型；生成和审稿节点各自调用一次，后一个节点能看到前一个节点的结果。审稿不通过会退回前一个生成节点，次数有上限。输出节点按人工审核、自动写入或演示模式决定是否改正文。可直接创建「章节起草」「起草 → 审稿 → 修改」「只润色」。这里的审稿节点只区分通过 / 不通过，不是按关键事件逐条核对的审稿清单 |
| 关键事件审稿 | 从生成任务入口发起，不走工作流节点，不改正文，也不产生章节提案。蓝图「关键事件」的每个非空行是一条，结果为已完成、未完成或待核实，并保存模型判断、引文和位置。已完成只接受当前正文里的逐字引文；引文对得上本身不会变成已完成。没有非空关键事件时给出空清单 |
| 连续性检查 | 从章节编辑器单独发起，按当前正文挑选相关图谱事实和已定稿章节记忆，提示时间线、人物状态与设定冲突。显示正文与来源引文的逐字核对结果；只供作者参考，不改正文、不创建提案 |
| 模型接入设置 | 首个账户可在 UI 配置本机共享的 OpenAI 兼容 API 地址、密钥、模型与生成参数；测试连接后立即生效，密钥不会由设置接口回显 |
| 长篇记忆 | 应用提案或自动写入后，为该章保存摘要、出场人物、新增设定与伏笔变化；再次定稿会替换这一章的当前记录。手动编辑或恢复正文版本会将旧记忆标为待刷新，并从后续生成上下文中排除；可在章节编辑器按需刷新。未应用的提案和未确认的演示生成不写入记忆 |
| 生成任务 | 批量或按工作流生成章节；实时进度、Token 流、取消；逐章状态，失败后可只重试尚未成功的章节；模型临时故障自动重试并提供分类提示。提示词在上下文预算内注入近期章节摘要、蓝图相关的图谱事实和尚未回收的伏笔；详情列出本次注入项和未注入项。结果进入提案审核或自动写入 |
| 提案审核 | 按段落组对比生成提案，可选择采纳部分修改或整份提案，标题可单独采纳；应用前保留旧版本并校验章节版本；过期提案可查看基准、当前和提案版本，编辑者可手工整理三方合并稿，也可按原要求重新生成 |

## 权限矩阵

权限在小说所属工作区内生效；只有工作区成员能访问项目。

| 操作 | 所有者 | 编辑 | 审阅者 | 只读 |
| --- | --- | --- | --- | --- |
| 查看作品、章节、图谱、工作流、任务事件与导出 | 是 | 是 | 是 | 是 |
| 查看已删除作品 | 是 | 是 | 是 | 是 |
| 查看已删除章节 | 是 | 是 | 否 | 否 |
| 编辑章节、写作指南、图谱、工作流；调整章节顺序 | 是 | 是 | 否 | 否 |
| 启动/取消/重试生成，重新生成过期提案 | 是 | 是 | 否 | 否 |
| 移入回收站或恢复作品/章节，永久删除工作流 | 是 | 是 | 否 | 否 |
| 应用或拒绝提案 | 是 | 是 | 是 | 否 |
| 查看工作区成员 | 是 | 是 | 是 | 是 |
| 添加成员、修改角色、移除成员 | 是 | 否 | 否 | 否 |

审阅者可以应用提案：审核队列面向审阅角色，且应用操作会保存被替换的正文版本并受 `expectedRevision` 乐观锁保护。

## 目录结构

```
backend/
  cmd/server/         HTTP 服务入口
  internal/auth/      会话 Cookie 签发与校验
  internal/config/    环境变量配置
  internal/database/  SQLite 连接与 schema
  internal/llm/       OpenAI 兼容的流式模型客户端（未配置密钥时进入演示模式）
  internal/novel/     HTTP 路由与各领域处理器
  internal/ratelimit/ 认证接口的固定窗口限流
  internal/task/      生成任务的调度、事件持久化与订阅
frontend/
  src/api.ts          后端 REST/WS 客户端
  src/App.vue         组合登录、作品库、编辑器、图谱、工作流与成员页面
  src/components/     各界面与提案、任务、回收站、模型设置面板
```

## 环境要求

- Go **1.27+**
- Node.js **20+**（开发前端）

## 快速开始

### 1. 启动后端

```bash
cd backend
cp .env.example .env        # PowerShell 可用：Copy-Item .env.example .env
go run ./cmd/server
```

后端会读取当前工作目录的 `.env`；已有系统环境变量优先。服务默认只监听本机 `127.0.0.1:8080`，
数据写入 `data/novel-studio.db`。未配置 `LLM_API_KEY` 时，生成任务会
返回演示文本，方便先跑通流程。旧版本的数据库会在启动时自动补齐新增字段（如 `writing_guide`、
`goal`）和章节记忆表，无需手动迁移。

数据库结构按 `schema_migrations` 版本逐次升级。检测到已有数据库需要升级时，服务会先用 SQLite
`VACUUM INTO` 在数据库旁创建一致性备份（文件名带 `.pre-migration-` 和 UTC 时间戳，包含 WAL 中已提交的数据），
再执行迁移；请确保磁盘空间充足，并在确认备份前不要删除这些文件。恢复时先停止服务，将备份复制到新的数据库路径，
再通过 `DB_PATH` 指向该文件，避免覆盖仍可能有 WAL / SHM 文件的原数据库。

### 2. 启动前端（开发模式）

```bash
cd frontend
npm install
npm run dev
```

打开 http://localhost:5173 ，Vite 会把 `/api`（含 WebSocket）代理到 `http://127.0.0.1:8080`。

### 3. 本机打包运行（Windows）

PowerShell 脚本会构建前后端，并生成可搬移的 `release/` 本机运行目录（服务、备份工具、前端静态文件、
`.env` 和数据目录）。首次打包会生成随机会话密钥；已有 `.env` 和数据库会保留。运行前请先停止旧实例：

```bash
powershell -ExecutionPolicy Bypass -File .\scripts\build-local.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\smoke-local.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\run-local.ps1
```

打开 http://127.0.0.1:8080 即可使用。运行目录默认为 `release/`，可通过 `-OutputDirectory` 指定其他位置；
请将目录放在当前用户有写入权限的位置。注册首个账户后，进入侧栏「API 设置」，填写兼容接口地址、模型和 API Key，先测试连接再保存；此后新生成任务即时使用新配置。生成目录内的
`Start-NovelStudio.ps1` 可独立启动或随整个目录搬移，数据库路径相对于该目录解析。

本机冒烟脚本会使用临时数据库验证健康检查、SPA 路由 / 前端资源及登录后的模型设置接口，结束后清理测试数据：

开发者如需手动构建，先执行 `cd frontend && npm ci && npm run build`，再执行
`cd ../backend && go build -o novelstudio.exe ./cmd/server && go build -o novelstudio-backup.exe ./cmd/backup`。
从 `backend/` 启动时，静态资源默认从 `../frontend/dist` 查找。

## 环境变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ADDR` | `127.0.0.1:8080` | 监听地址；本机默认不暴露到局域网 |
| `APP_ENV` | — | 设为 `production` 时强制要求 `SESSION_SECRET`，并启用安全 Cookie |
| `DB_PATH` | `data/novel-studio.db` | SQLite 数据文件路径 |
| `SESSION_SECRET` | 随机生成 | 会话签名密钥；生产环境必须显式设置，否则启动即报错 |
| `SESSION_SECURE` | 跟随 `APP_ENV` | 仅在 HTTPS 下发送会话 Cookie |
| `ALLOWED_ORIGINS` | `localhost:5173,127.0.0.1:5173` | 允许的跨域来源主机（逗号分隔），同时用于 WebSocket Origin 校验 |
| `STATIC_DIR` | `frontend/dist` | 前端构建产物目录；开发源码目录下也会回退查找 `../frontend/dist` |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | OpenAI 兼容接口地址 |
| `LLM_API_KEY` | 空 | 模型密钥；为空时进入演示模式 |
| `LLM_MODEL` | `gpt-4o-mini` | 模型名称 |
| `LLM_TEMPERATURE` | `0.7` | 生成随机度，范围 0–2 |
| `LLM_MAX_TOKENS` | `0` | 单次最大输出 token，`0` 表示交由模型默认值 |
| `LLM_TIMEOUT` | `480` | 模型请求超时秒数 |
| `CONTEXT_BUDGET_RUNES` | `12000` | 生成时注入上下文的字数预算，按近期摘要、相关图谱、未回收伏笔三份均分，某一部分超长不会占掉其他部分的额度 |

完整示例见 [`backend/.env.example`](backend/.env.example)。
上述模型参数也可在登录后的「API 设置」页面修改；修改会写入当前运行目录的 `.env`，并立即用于新任务，不需要重启。
这是一组本机全局配置，所有账户和工作区共用；为避免协作者意外替换 API Key，只有本机首个注册账户可以修改和测试连接。

## 安全说明

- 密码使用 bcrypt 哈希存储；会话 Cookie 为 `HttpOnly` + `SameSite=Lax`，签名采用 HMAC-SHA256。
- 登出会吊销该账户所有设备上的已有会话；需要在其他设备重新登录。
- 作品与章节删除采用软删除，数据和历史版本保留在回收站；仅工作区所有者/编辑可以删除或恢复。
- 登录 / 注册接口按客户端 IP 限流（默认每分钟 20 次），超限返回 `429 RATE_LIMITED`。
- 所有响应附带 `X-Content-Type-Options`、`X-Frame-Options`、`Referrer-Policy`、
  `Permissions-Policy` 等安全头；SPA 页面额外附带 CSP。
- 请求体限制为 2 MB，且拒绝未知字段与多段 JSON。
- 跨域采用显式白名单（`ALLOWED_ORIGINS`），仅回显匹配的来源，不会使用通配符。
- 模型 API Key 保存在本机 `.env` 文件中（明文）；设置读取接口只返回是否已配置，不返回密钥。仅本机首个注册账户可通过 UI 修改模型配置；请限制运行目录的文件访问权限，并不要把 `.env` 分享或提交到版本库。

## 测试与验证

```bash
cd backend && go vet ./... && go test ./...
cd frontend && npm test && npm run lint && npm run format:check && npm run build
```

GitHub Actions 会在 push 和 pull request 上运行相同的后端检查，以及前端测试、ESLint、Prettier 和生产构建。

本机打包目录内附带在线备份工具，可在服务运行时创建包含 WAL 已提交数据的一致性快照；默认备份写入数据库旁，
使用带 UTC 时间戳的新文件名，不会覆盖已有文件。在开发模式下从 `backend/` 运行，打包后从 `release/` 运行：

```bash
.\novelstudio-backup.exe
# 可选：指定备份文件路径
.\novelstudio-backup.exe -out .\data\manual-backup.sqlite
```

从源码运行可用 `go run ./cmd/backup`。请将备份文件复制到应用数据目录之外的安全位置，并定期验证能够从备份启动一个独立实例。

## 主要 API

所有接口以 `/api/v1` 为前缀，除注册/登录外均需登录会话。

- `POST /auth/register`、`POST /auth/login`、`POST /auth/logout`、`GET /me`
- `GET|POST /novels`、`GET|PUT /novels/{id}`（含 `writingGuide`）
- `GET /novels/{id}/export?format=txt|md`（按章节顺序导出未删除章节；只读及以上角色可下载）
- `GET /novels/trash`、`DELETE /novels/{id}`、`POST /novels/{id}/restore`
- `GET|POST /novels/{id}/chapters`、`PUT|DELETE /novels/{id}/chapters/{chapterID}`、`PUT /novels/{id}/chapters/order`（更新使用 `expectedRevision`，并可传 `expectedUpdatedAt` 检测蓝图并发修改）
- `POST /novels/{id}/chapters/{chapterID}/memory/refresh`（基于当前正文重建该章记忆；只允许所有者 / 编辑，需配置真实模型）
- `GET /novels/{id}/chapters/trash`、`POST /novels/{id}/chapters/{chapterID}/restore`
- `GET /novels/{id}/chapters/{chapterID}/revisions`、`POST .../revisions/{revision}/restore`
- `GET /novels/{id}/graph`、`POST /novels/{id}/graph/nodes`、`PUT|DELETE /novels/{id}/graph/nodes/{nodeID}`、`POST /novels/{id}/graph/edges`、`PUT|DELETE /novels/{id}/graph/edges/{edgeID}`
- `GET|POST /novels/{id}/workflows`、`PUT|DELETE /novels/{id}/workflows/{workflowID}`、`POST .../run`
- `GET|PUT /settings/llm`、`POST /settings/llm/test`（登录后查看本机模型 API；仅首个账户可修改 / 测试）
- `POST /novels/{id}/tasks/batch`、`GET /novels/{id}/tasks`、`GET /tasks/{taskID}`
- `GET /tasks/{taskID}/events`、`GET /tasks/{taskID}/ws`（WebSocket）、`POST /tasks/{taskID}/cancel|retry`
- `GET /novels/{id}/proposals`、`POST .../proposals/{proposalID}/apply|reject`（`apply` 空请求体表示应用整份提案；也可传 `{"selectedHunks":[0,2],"applyTitle":true}` 只采纳差异视图中选中的段落组，并可单独选择标题）
- `GET .../proposals/{proposalID}/merge-preview`、`POST .../proposals/{proposalID}/merge`（读取提案基准版本、当前正文和提案正文；所有者 / 编辑可提交手工整理后的合并稿，并通过 `expectedRevision` 防止预览后的并发覆盖）
- `POST /novels/{id}/proposals/{proposalID}/regenerate`（基于原任务说明重生成过期提案）
- `GET /healthz`（含数据库连通性检查）

## 许可证

本项目以 [MIT License](LICENSE) 授权。
