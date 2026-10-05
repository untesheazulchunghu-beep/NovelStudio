# NovelStudio Web 架构改造计划

## 目标
将现有 C++ 桌面应用改造为 **Web 大后端 + 轻量前端** 架构，保持 Windows-only。

## 参考项目分析

### 1. Koodo Reader (Go + React)
- **后端**: Go HTTP server (Gin/原生 net/http)
- **特点**: 
  - 轻量级 HTTP API
  - 文件上传/下载
  - OPDS 协议支持
  - CORS 处理
  - 认证中间件

### 2. Cherry Studio (Electron + TypeScript)
- **架构**: Main Process (Node.js) + Renderer (React)
- **数据层**: 
  - IPC 通信
  - SQLite + Drizzle ORM
  - 四层数据系统 (BootConfig/Cache/Preference/DataApi)
- **AI 集成**: 
  - 多 Provider 支持
  - MCP (Model Context Protocol)
  - Streaming SSE

### 3. AI Novel Writer (Electron + React)
- **技术栈**: Vite + React + better-sqlite3
- **特性**:
  - 向量数据库 (LanceDB)
  - 实时协作
  - 版本控制

## 新架构设计

### 技术选型

#### 后端 (C++ Web Server)
- **框架选择**: **Crow** (header-only, 类似 Express.js)
  - 轻量级 (~3000 行)
  - 现代 C++11/14
  - 路由、中间件、WebSocket、SSE 支持
  - 替代方案: Drogon, oatpp

#### 前端 (轻量框架)
- **框架选择**: **Solid.js** 或 **Preact** 或 **htmx + Alpine.js**
  - Solid.js: 无虚拟 DOM，极快性能 (~7KB)
  - Preact: React 语法，3KB
  - htmx: 超轻量 (~14KB)，HTML 驱动
  
#### 构建工具
- **Vite**: 极速开发服务器
- **esbuild**: 快速打包

### 系统架构

```
┌─────────────────────────────────────────────────────┐
│                  浏览器 (localhost)                  │
│  ┌──────────────────────────────────────────────┐  │
│  │   Solid.js / Preact 前端                     │  │
│  │   - 轻量级 UI 组件                            │  │
│  │   - 状态管理 (Zustand / Nanostores)          │  │
│  │   - HTTP Client (fetch / ky)                 │  │
│  └──────────────────────────────────────────────┘  │
└──────────────────┬──────────────────────────────────┘
                   │ HTTP/SSE
┌──────────────────▼──────────────────────────────────┐
│          C++ Web Server (Crow/Drogon)               │
│  ┌──────────────────────────────────────────────┐  │
│  │   路由层 (REST API)                          │  │
│  │   ├─ /api/novels                             │  │
│  │   ├─ /api/chapters                           │  │
│  │   ├─ /api/characters                         │  │
│  │   ├─ /api/ai/complete (SSE streaming)        │  │
│  │   └─ /api/export                             │  │
│  └──────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────┐  │
│  │   中间件层                                    │  │
│  │   ├─ CORS 处理                               │  │
│  │   ├─ 认证/会话                               │  │
│  │   ├─ 请求日志                                 │  │
│  │   └─ 错误处理                                 │  │
│  └──────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────┐  │
│  │   业务逻辑层                                  │  │
│  │   ├─ AIService (已有)                        │  │
│  │   ├─ DataManager (已有)                      │  │
│  │   ├─ ConfigManager (已有)                    │  │
│  │   └─ ExportService (已有)                    │  │
│  └──────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────┐  │
│  │   数据层                                      │  │
│  │   ├─ SQLite3 (可选)                          │  │
│  │   ├─ 内存存储                                 │  │
│  │   └─ 文件系统                                 │  │
│  └──────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

### API 设计

#### RESTful 端点

```cpp
// Novel 管理
GET    /api/novels           // 获取所有小说
POST   /api/novels           // 创建小说
GET    /api/novels/:id       // 获取单个小说
PUT    /api/novels/:id       // 更新小说
DELETE /api/novels/:id       // 删除小说

// Chapter 管理
GET    /api/novels/:id/chapters
POST   /api/novels/:id/chapters
PUT    /api/chapters/:id
DELETE /api/chapters/:id

// Character 管理
GET    /api/novels/:id/characters
POST   /api/novels/:id/characters
PUT    /api/characters/:id
DELETE /api/characters/:id

// AI 功能
POST   /api/ai/complete       // 生成文本
POST   /api/ai/stream         // SSE 流式生成
POST   /api/ai/continue       // 续写

// 统计
GET    /api/novels/:id/stats

// 导出
POST   /api/novels/:id/export  // 导出为 DOCX/PDF/TXT
```

#### 响应格式

```json
{
  "success": true,
  "data": { ... },
  "error": null
}
```

### 前端结构

```
frontend/
├── src/
│   ├── components/          # UI 组件
│   │   ├── NovelList.tsx
│   │   ├── ChapterEditor.tsx
│   │   ├── CharacterPanel.tsx
│   │   └── AIAssistant.tsx
│   ├── stores/              # 状态管理
│   │   ├── novelStore.ts
│   │   └── aiStore.ts
│   ├── services/            # API 调用
│   │   ├── api.ts
│   │   └── sse.ts
│   ├── App.tsx
│   └── main.tsx
├── public/
│   └── index.html
├── package.json
└── vite.config.ts
```

## 实施步骤

### Phase 1: 基础 Web 服务器 (1-2 天)
- [ ] 引入 Crow 框架
- [ ] 实现基础 HTTP 服务器
- [ ] 静态文件服务
- [ ] CORS 中间件
- [ ] 错误处理

### Phase 2: REST API (2-3 天)
- [ ] Novel CRUD 端点
- [ ] Chapter CRUD 端点
- [ ] Character CRUD 端点
- [ ] 统计端点
- [ ] 导出端点

### Phase 3: AI 集成 (2 天)
- [ ] AI 完成端点
- [ ] SSE 流式端点
- [ ] 多 Provider 支持

### Phase 4: 前端开发 (3-4 天)
- [ ] 创建 Vite + Solid.js 项目
- [ ] API 客户端封装
- [ ] 小说列表页
- [ ] 章节编辑器
- [ ] AI 助手面板
- [ ] 响应式布局

### Phase 5: 集成测试 (1-2 天)
- [ ] 端到端测试
- [ ] 性能优化
- [ ] 文档完善

## 技术优势

### vs 当前 C++ 桌面版
- ✅ **Web 访问**: 浏览器直接使用，无需安装
- ✅ **现代 UI**: 基于 Web 技术，更容易开发
- ✅ **热重载**: 前端修改即时生效
- ✅ **保持性能**: C++ 后端依然高性能
- ✅ **Windows 专属**: 利用 Windows API 优势

### vs 纯 Electron
- ✅ **更小内存**: 无需打包 Chromium
- ✅ **更快启动**: C++ 原生启动
- ✅ **更小体积**: 可执行文件 < 5MB

## 依赖管理

### 新增依赖 (仅 1 个)
- **Crow**: Header-only HTTP 框架 (无需编译)

### 现有依赖保持
- SQLite3 (可选)

### 前端依赖 (dev only)
- Vite
- Solid.js / Preact
- TypeScript

## 文件结构

```
NovelStudio/
├── backend/                 # C++ 后端
│   ├── src/
│   │   ├── server/          # Web 服务器
│   │   │   ├── crow_server.cpp
│   │   │   ├── routes.cpp
│   │   │   └── middleware.cpp
│   │   ├── services/        # 业务逻辑 (已有)
│   │   └── main.cpp
│   ├── include/
│   └── CMakeLists.txt
├── frontend/                # 前端
│   ├── src/
│   ├── public/
│   ├── package.json
│   └── vite.config.ts
├── dist/                    # 构建输出
│   ├── backend/
│   │   └── NovelStudio.exe
│   └── frontend/
│       └── index.html
└── README.md
```

## 打包部署

### 开发模式
```bash
# 后端
cd backend && cmake --build build --target NovelStudioServer
build\bin\Release\NovelStudioServer.exe

# 前端 (另一个终端)
cd frontend && npm run dev
```

### 生产模式
```bash
# 构建前端
cd frontend && npm run build

# 构建后端 (嵌入前端资源)
cd backend && cmake --build build --config Release

# 单个可执行文件
dist\NovelStudioServer.exe
```

## 预期成果

- **单文件部署**: 1 个 EXE + 1 个 frontend 文件夹
- **快速启动**: < 500ms
- **低内存占用**: < 50MB
- **现代 UI**: 响应式、流畅动画
- **开发效率**: 前端热重载，后端快速编译
