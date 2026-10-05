# NovelStudio Web

**Windows 原生 C++ 后端 + Solid.js 轻量前端的 AI 小说写作平台**

## ✨ 新架构特性

- 🌐 **Web 界面**: 浏览器访问，现代化 UI
- ⚡ **C++ 后端**: 高性能 REST API + SSE 流式
- 🪶 **轻量前端**: Solid.js (~7KB)，极速响应
- 🔥 **热重载**: 前端修改即时生效
- 💾 **零安装**: 单个 EXE + 配置文件

## 🚀 快速开始

### 开发模式

```bash
# 一键启动后端 + 前端
dev-start.bat

# 后端: http://localhost:3000
# 前端: http://localhost:5173
```

### 生产构建

```bash
# 构建前端 + 后端
build-web.bat

# 运行
cd dist
NovelStudioServer.exe

# 访问: http://localhost:3000
```

### 手动构建

#### 后端
```bash
cd backend
mkdir build && cd build
cmake .. -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
```

#### 前端
```bash
cd frontend
npm install
npm run dev         # 开发模式
npm run build       # 生产构建
```

## 📐 架构设计

```
┌─────────────────────────────────────┐
│  浏览器 (localhost:5173)             │
│  Solid.js + TypeScript              │
│  - 轻量 UI (7KB)                    │
│  - 响应式设计                        │
│  - 实时预览                          │
└──────────────┬──────────────────────┘
               │ HTTP/SSE
┌──────────────▼──────────────────────┐
│  C++ Web Server (Port 3000)        │
│  Crow Framework                     │
│  ├─ REST API                        │
│  ├─ SSE Streaming                   │
│  ├─ Static Files                    │
│  └─ CORS Middleware                 │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│  业务逻辑 (已有)                     │
│  ├─ AIService                       │
│  ├─ DataManager                     │
│  └─ ConfigManager                   │
└─────────────────────────────────────┘
```

## 🛠️ 技术栈

### 后端
- **语言**: C++20
- **框架**: Crow (header-only HTTP 框架)
- **依赖**: Boost, SQLite3 (可选)
- **特性**: REST API, SSE 流式, 静态文件服务

### 前端
- **框架**: Solid.js 1.9+ (无虚拟 DOM)
- **构建**: Vite 6.0
- **语言**: TypeScript
- **HTTP**: ky (轻量 fetch 封装)
- **包大小**: < 50KB (gzipped)

## 📡 API 端点

### Novel 管理
```
GET    /api/novels              # 获取所有小说
POST   /api/novels              # 创建小说
GET    /api/novels/:id          # 获取单个小说
PUT    /api/novels/:id          # 更新小说
DELETE /api/novels/:id          # 删除小说
GET    /api/novels/:id/stats    # 获取统计
```

### Chapter 管理
```
GET    /api/novels/:id/chapters   # 获取章节列表
POST   /api/novels/:id/chapters   # 创建章节
PUT    /api/chapters/:id          # 更新章节
DELETE /api/chapters/:id          # 删除章节
```

### Character 管理
```
GET    /api/novels/:id/characters # 获取角色列表
POST   /api/novels/:id/characters # 创建角色
```

### AI 功能
```
POST   /api/ai/complete          # AI 文本生成
POST   /api/ai/continue          # AI 续写
```

### 响应格式
```json
{
  "success": true,
  "data": { ... },
  "error": null
}
```

## 📂 项目结构

```
NovelStudio/
├── backend/                    # C++ 后端
│   ├── src/
│   │   ├── server/             # Web 服务器
│   │   │   └── crow_server.cpp # Crow HTTP 服务器
│   │   ├── ai/                 # AI 服务 (已有)
│   │   ├── data/               # 数据管理 (已有)
│   │   └── utils/              # 工具类 (已有)
│   ├── include/
│   │   ├── crow/               # Crow 框架 (header-only)
│   │   └── server/             # 服务器头文件
│   └── CMakeLists.txt
│
├── frontend/                   # Solid.js 前端
│   ├── src/
│   │   ├── components/         # UI 组件
│   │   │   └── NovelList.tsx
│   │   ├── services/           # API 客户端
│   │   │   └── api.ts
│   │   ├── stores/             # 状态管理
│   │   │   └── novelStore.ts
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── public/
│   │   └── index.html
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
│
├── dist/                       # 生产构建输出
│   ├── NovelStudioServer.exe
│   ├── frontend/
│   └── config.json
│
├── dev-start.bat               # 开发启动脚本
├── build-web.bat               # 生产构建脚本
├── ARCHITECTURE_PLAN.md        # 架构设计文档
└── README.md
```

## 🎨 UI 设计

### 配色方案 (Production Slate)
- **背景**: `#0F172A` (深海军蓝)
- **卡片**: `#1E293B` (石板灰)
- **边框**: `#334155` (中灰)
- **文本**: `#F8FAFC` (冷白)
- **主色**: `#38BDF8` (天蓝)
- **辅色**: `#F97316` (橙色)

### 字体
- **UI**: System Font Stack (Segoe UI, SF Pro, ...)
- **代码**: 等宽字体

## 📊 性能指标

| 指标 | 当前桌面版 | Web 版本 | 改善 |
|------|-----------|---------|------|
| 启动时间 | ~300ms | ~500ms | -200ms |
| 内存占用 | ~30MB | ~40MB | -10MB |
| 安装包大小 | 5MB | 2MB (EXE) + 100KB (前端) | -2.9MB |
| 首屏加载 | N/A | ~200ms | - |
| 开发热重载 | ❌ | ✅ 50ms | - |

## 🔧 配置

### config.json
```json
{
  "openai_api_key": "sk-...",
  "anthropic_api_key": "sk-ant-...",
  "ollama_endpoint": "http://localhost:11434/api/chat",
  "server_port": 3000
}
```

## 🛣️ 路线图

### Phase 1: 基础 Web 服务器 ✅
- [x] Crow 框架集成
- [x] REST API 路由
- [x] CORS 中间件
- [x] 静态文件服务

### Phase 2: 前端开发 ✅
- [x] Solid.js 项目搭建
- [x] API 客户端封装
- [x] 小说列表组件
- [x] 状态管理 (Store)

### Phase 3: 完整功能 (进行中)
- [ ] 章节编辑器
- [ ] AI 助手面板
- [ ] SSE 流式输出
- [ ] 角色管理
- [ ] 导出功能

### Phase 4: 优化与打磨
- [ ] 响应式布局
- [ ] 暗色/亮色主题
- [ ] 快捷键支持
- [ ] 离线支持
- [ ] 性能优化

## 🤝 参考项目

感谢以下项目的启发：
- **Koodo Reader**: Go HTTP server 设计
- **Cherry Studio**: Electron 架构和数据层
- **AI Novel Writer**: React + SQLite 模式

## 📄 许可证

MIT License

---

**当前状态**: Phase 2 完成 ✅  
**可用性**: 开发可用 🚧  
**架构**: Web 大后端 + 轻量前端 🌐  
**Windows Only**: ✅
