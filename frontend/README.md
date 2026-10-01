# 拾卷 · 前端

Vue 3 + TypeScript + Vite 实现的单页应用，为 Novel Studio 后端提供作品库、章节编辑、知识图谱、
创作工作流与成员管理等界面。

## 开发

```bash
npm install
npm run dev     # http://localhost:5173
```

开发服务器通过 `vite.config.ts` 中的代理把 `/api`（包含 WebSocket）转发到
`http://127.0.0.1:8080`，因此需要先启动后端。代理目标可通过修改 `vite.config.ts` 调整。

## 构建

```bash
npm run build     # vue-tsc 类型检查 + vite build，输出到 dist/
npm run preview   # 本地预览构建结果
```

生产环境推荐把 `dist/` 交给后端托管（后端会按 `STATIC_DIR` 查找并做 SPA 回退），即可单进程部署。

## 代码结构

- `src/api.ts`：与后端约定的 REST 客户端与 WebSocket 连接封装，包含全部类型定义。
- `src/App.vue`：应用主体（登录、作品库、章节编辑、知识图谱、工作流、成员与权限）。
- `src/style.css`：全局样式。
- `src/main.ts`：应用入口。
