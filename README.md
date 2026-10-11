# NovelStudio

NovelStudio 是一个面向长篇小说创作的单用户 Agent 工作台，支持在一个本地数据库中创建和切换多个小说项目。界面借鉴 Visual Studio Code：左侧是当前项目树和角色/便笺，中央是章节编辑器，右侧是可以切换为对话、大纲、上下文的写作 Agent。

当前版本先把本地单用户闭环跑通：多个小说项目及其章节、角色、便笺、对话和模型设置保存在 `data/novelstudio.db`，使用 SQLite 文件，不需要额外数据库服务。旧版 `data/workspace.json` 会在首次启动时导入，原 JSON 文件保留为备份。每个项目通过独立的工作区 ID 隔离；后续可以在 API 层增加用户身份。

## 快速启动

需要 Go 1.23+、Node.js 20+。SQLite 由纯 Go 驱动提供，无需在 Windows 上另外安装 SQLite 或 C 编译器。

```powershell
# 终端 1：Go API（8080）
go run ./cmd/novelstudio

# 终端 2：React/Vite（5173）
cd frontend
npm install
npm run dev
```

打开 <http://localhost:5173>。生产构建可以执行：

```powershell
cd frontend
npm run build
cd ..
New-Item -ItemType Directory -Force bin | Out-Null
go build -o bin/novelstudio.exe ./cmd/novelstudio
bin/novelstudio.exe
```

构建后的 Go 服务会从 `frontend/dist` 提供静态页面，也可以用 `-web` 指定目录。开发时 Vite 会把 `/api` 代理到 `http://127.0.0.1:8080`。

开发代理目标可以通过 `NOVELSTUDIO_API_TARGET` 覆盖，适合在默认 API 端口已被占用时并行启动本项目。

后端测试运行 `go test ./...`，CI 也会执行。服务只接受 `localhost`、`127.0.0.1` 和 `[::1]` 作为请求 Host；使用其他域名或局域网地址访问会收到 403，用于防止 DNS rebinding 页面读取本地工作区。

使用 `build-web.bat` 构建后，可以从仓库根目录或 `bin` 目录启动 `novelstudio.exe`；默认数据库和前端目录会按应用目录定位。按 Ctrl+C 退出时，服务会等待进行中的请求结束，最多等待 10 秒。

需要 Windows 便携包时，运行 `powershell -ExecutionPolicy Bypass -File .\package-portable.ps1`。脚本在 `release/NovelStudio-portable.zip` 生成包含服务、前端静态文件、README 和许可证的压缩包，不会打包本地数据库；如果目标输出已存在，脚本会停止以免覆盖旧包。

服务启动选项可以通过命令行参数或环境变量设置：`-addr` / `NOVELSTUDIO_ADDR` 指定监听地址，`-data` / `NOVELSTUDIO_DATA` 指定 SQLite 文件，`-web` / `NOVELSTUDIO_WEB` 指定前端构建目录。命令行参数优先于环境变量；默认数据库和前端目录位于应用目录下。模型服务商、模型名称和 API Key 在界面的“设置”中按项目保存到本地数据库。

## 功能

- **章节工作区**：章节树、标题和本章目标、正文编辑、字数统计、自动保存、`⌘/Ctrl + S` 保存；编辑器支持 `Tab / Shift + Tab` 缩进或反向缩进选中行；临时保存失败后会自动退避重试，编辑内容变化时重新计时；可删除章节并用上下移动按钮调整顺序。
- **章节版本**：作者可为当前章节保存检查点、预览正文、与打开版本窗口时的当前稿并排对照，并恢复历史版本；每章最多保留最近 20 个版本。恢复前系统会自动保存当前稿，版本也包含在项目备份中。
- **出版导出**：将当前项目按章节顺序导出为 Markdown（`.md`）、Word 文档（`.docx`）或 EPUB 电子书（`.epub`）；这些格式都包含当前编辑器中尚未自动保存的章节标题和正文。
- **项目备份与恢复**：可导出包含项目资料、章节及版本、角色、便笺、时间线事件、最近 60 条对话和模型设置的 JSON 备份，并将备份作为新项目导入；为避免把凭据写入备份，API Key、接口地址中的用户名/密码和凭据类查询参数都会移除，恢复后请检查模型设置。
- **快速搜索与帮助**：点击顶部搜索框或按 `Ctrl/⌘ + K` 搜索当前项目的章节正文、角色、便笺、时间线和世界观资料（包括当前章节未保存的编辑），也可通过命令快速新建内容、切换设置、导出 Markdown/Word/EPUB/备份或导入备份；帮助面板列出常用键盘操作。
- **多项目管理**：从顶部选择器切换小说项目，或新建项目；可修改项目名称、副标题、题材、故事设定和目标字数；每个项目有独立的章节、上下文、聊天记录和模型设置，切换项目时未发送的 Agent 输入会在当前页面会话中保留。
- **Agent 四种模式**：写作、规划、审阅、改写；对话会带入当前项目设定、章节摘要、最近编辑内容、编辑器选区及其附近正文，角色、便笺和时间线资料由作者在上下文面板中逐项选择，聊天记录会标出 Agent 回复所用的模式。改写模式需要先选中不超过 1800 字的正文，Agent 给出替换稿后可在预览中调整，再由作者确认应用；应用前会再次确认选区原文和附近正文未变化。本地规则引擎会提示配置兼容模型。单条输入最多 8000 个字符，进行中的模型请求可以取消。
- **正文生成**：点击「AI 续写」把生成内容追加到当前章节，也可以在聊天中请求续写、摘要或润色；续写和聊天共用上下文面板的资料选择。
- **项目上下文**：查看项目设定，并创建、编辑和删除角色档案与分类便笺；上下文面板可以按项目保存资料选择，并显示本次 Agent 对话会使用的来源。
- **故事时间线**：按项目记录关键事件、故事时间、状态和关联章节；时间线事件可在命令面板中搜索，并随项目备份恢复，默认不会自动加入 Agent 上下文。
- **模型设置**：默认使用本地规则引擎；设置为 `openai` 或 `compatible` 后，可填写 OpenAI 兼容接口和模型。`openai` 服务商需要 API Key；`compatible` 服务商可按接口要求填写，本机免鉴权接口可以留空。密钥保存在本地数据库，读取设置时只返回是否已配置；留空会保留现有密钥，也可以显式清除。
- **编辑器偏好**：可在设置中保存正文大小、字体样式、界面主题和专注写作模式；浅色主题适合明亮环境，专注模式暂时隐藏资源侧栏和 Agent 面板，退出按钮始终保留在编辑器工具栏。
- **本地持久化**：Go 后端以 SQLite 事务保存项目、章节、角色、便笺、时间线、对话和设置，重启后保留工作区。

## API 概览

开发阶段与后续里程碑见[开发路线图](docs/ROADMAP.md)；发布前请按[验收清单](docs/RELEASE_CHECKLIST.md)检查运行包和数据迁移。

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET/POST | `/api/projects` | 列出项目或新建小说项目 |
| POST | `/api/projects/import` | 将版本化 JSON 备份恢复为新项目 |
| PUT | `/api/projects/:workspace_id` | 更新项目资料和目标字数 |
| DELETE | `/api/projects/:workspace_id` | 删除项目及其全部数据；不能删除最后一个项目 |
| GET | `/api/workspace` | 读取工作区和章节目录摘要；章节正文按需读取 |
| GET | `/api/workspace/backup` | 下载当前项目的版本化 JSON 备份（移除 API Key 和接口地址中的凭据） |
| POST | `/api/workspace/export/markdown` | 导出完整 Markdown，并合并当前章节未保存的标题和正文 |
| POST | `/api/workspace/export/docx` | 导出 Word 文档，并合并请求中当前章节的未保存正文 |
| POST | `/api/workspace/export/epub` | 导出带目录和章节导航的 EPUB，并合并当前章节的未保存正文 |
| GET | `/api/health` | 服务健康检查 |
| POST/PUT/DELETE | `/api/chapters`、`/api/chapters/:id` | 章节 CRUD |
| GET | `/api/chapters/:id` | 按需读取单章正文 |
| GET | `/api/chapters/search?q=...` | 搜索章节标题、摘要和正文，返回匹配片段 |
| PUT | `/api/chapters/order` | 按 `chapter_ids` 顺序调整章节编号 |
| POST | `/api/chapters/:id/generate` | 生成当前章节正文；可带入编辑器中的标题、目标和正文 |
| GET/POST | `/api/chapters/:id/revisions` | 查看版本摘要或为当前章节保存版本 |
| GET | `/api/chapters/:id/revisions/:revision_id` | 读取一个版本的完整正文 |
| POST | `/api/chapters/:id/revisions/:revision_id/restore` | 恢复版本，并自动保存恢复前内容 |
| POST | `/api/chat` | Agent 对话并持久化消息 |
| GET/PUT | `/api/settings` | 模型服务设置 |
| GET/POST | `/api/characters`、`/api/notes` | 读取或新增上下文资料 |
| GET/PUT/DELETE | `/api/characters/:id`、`/api/notes/:id` | 读取、修改或删除上下文资料 |
| GET/POST | `/api/timeline` | 读取或新增故事时间线事件 |
| GET/PUT/DELETE | `/api/timeline/:id` | 读取、修改或删除时间线事件 |
| GET/POST | `/api/world-assets` | 读取或新增世界观资料 |
| GET/PUT/DELETE | `/api/world-assets/:id` | 读取、修改或删除世界观资料 |

工作区 API 使用 `X-Workspace-ID` 请求头选择项目；缺少该请求头时使用 `default` 工作区。`GET /api/projects` 返回每个项目的 `workspace_id`，调用方应将其作为不透明标识使用。创建和更新项目的请求体可包含 `title`、`subtitle`、`genre`、`premise` 和 `word_target`。对话和续写请求可以传入尚未保存的 `chapter_title`、`chapter_summary` 和 `context`，让 Agent 使用当前编辑器中的内容；`/api/chat` 还可传入 `context_items`（角色、便笺、时间线事件和世界观资料 ID），由调用方明确控制可选资料，省略该字段时兼容旧客户端行为。DOCX 和 EPUB 导出从 SQLite 读取项目和章节，并通过请求体覆盖当前章节标题与正文，避免等待自动保存后再导出。EPUB 包含标题页、目录、章节 XHTML 和样式表。章节版本按章节隔离；恢复操作在同一 SQLite 事务中保存恢复前的章节并更新正文。对话请求可带 `request_id`；同一提交重试时应复用该 ID，服务端按请求指纹返回已保存的用户消息和 Agent 回复。请求结果单独保存在 SQLite，因此聊天窗口裁剪到最近 60 条后，重试仍会返回原回复；结果保留到项目删除。同一进程会按数据库共享请求锁；不同进程通过 SQLite 租约协调相同请求，等待者会复用已保存回复。租约为两分钟，进程异常退出后可在到期后接管；租约失效的旧持有者不能提交结果。若模型调用异常长于租约，仍可能出现重复调用，但 SQLite 只接受一份回复。已校验为不同内容的 ID 会返回 HTTP 409（`chat_request_conflict`）；命中相同用户消息但旧记录没有指纹时会返回 `chat_request_unverifiable`，前端保留输入并要求先检查聊天记录；用户显式确认后才会使用新 ID 重发。若网络中断导致结果无法确认，前端会保留原请求并先同步聊天记录；确认记录中没有该请求后才开放同 ID 重试。删除项目会级联删除其章节版本、章节、角色、便笺、聊天记录、请求结果和模型设置，并至少保留一个项目。

## 参考项目

`ck/AI-Novel-Writer-master` 和 `ck/luobi-ai-writing-main` 提供了长篇创作流程、版本留存、作者确认与 IDE 式布局参考；本项目以轻量的章节检查点和恢复保护承接版本管理思路，代码和数据结构独立实现。

## 目录

```text
cmd/novelstudio/       Go 入口和 HTTP 服务
internal/app/          工作区模型、SQLite store、API 路由和 Agent
frontend/src/          React 工作台、API 客户端和主题样式
data/                  运行时 SQLite 数据库（首次启动自动创建，不提交）
```

### World assets

The workspace now includes reusable world assets for places, organizations, items, and concepts. Each asset can be linked to chapters and related assets, inspected in a card or relation view, searched from the command palette, included explicitly in Agent context, and preserved in project backups. The API is available at /api/world-assets and /api/world-assets/:id.

### Database upgrades

When an existing SQLite database has an older schema, startup creates a protected `novelstudio.db.pre-migration-v*.bak` snapshot before applying migrations. New databases and in-memory databases do not create this file.
