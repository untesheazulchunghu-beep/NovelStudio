# NovelStudio 发布验收清单

此清单用于发布 Go + React 版本前的人工验收。构建检查由 `.github/workflows/build.yml` 在 Ubuntu 和 Windows 上执行；发布者仍需确认下列运行行为与用户数据保护。

## 构建产物

- [ ] 从干净检出运行 `powershell -ExecutionPolicy Bypass -File .\package-portable.ps1`，确认生成 `release/NovelStudio-portable.zip`。
- [ ] 解压便携包，确认其中包含 `novelstudio.exe`、`frontend/dist`、README 和许可证。
- [ ] 将可执行文件与 `frontend/dist` 放入同一个应用目录；从另一个工作目录启动，确认静态页面和 `/api` 都可访问。
- [ ] 确认发布包不包含 `data/novelstudio.db`、模型 API Key、日志或开发依赖目录。
- [ ] 确认 Go 服务默认只监听回环地址，浏览器页面可从 `http://localhost:8080` 使用。

## 写作流程

- [ ] 首次启动后能创建项目和章节；编辑标题、目标和正文后等待自动保存，再重启并确认内容保留。
- [ ] 离线状态下使用本地 Agent；配置兼容模型后发送请求，并确认取消请求可用。
- [ ] 保存章节检查点、修改正文、预览并恢复；恢复前的正文可作为新检查点找回。
- [ ] 导出 Markdown，检查项目标题、章节顺序、中文内容及当前未保存正文。
- [ ] 导出 DOCX，在 Word 或 LibreOffice 中打开，检查项目标题、章节顺序、中文文本、段落和当前未保存正文。
- [ ] 导出 EPUB，在常见电子书阅读器中打开，检查首个 ZIP 条目、标题页、目录链接、章节顺序、中文文本和当前未保存正文。
- [ ] 在设置中切换字体、字号、浅色主题和专注写作模式，刷新页面后确认偏好保留；检查窄屏布局和键盘焦点仍可用。

## 数据迁移与恢复

- [ ] 对已有 `data/workspace.json` 的安装目录启动一次，确认内容导入 SQLite 且原 JSON 保留。
- [ ] 导出项目 JSON 备份并导入为新项目，检查章节、版本、角色、便笺和最近对话。
- [ ] 创建、编辑和删除时间线事件，关联章节后导出并恢复备份，确认事件和章节关联保留；在 Agent 上下文面板逐项勾选角色、便笺和时间线，确认来源提示与实际请求一致，未勾选资料不会带入。
- [ ] 切换到另一个项目再切回，确认 Agent 上下文选择按项目隔离并在刷新后保留。
- [ ] 检查备份未包含 API Key，以及模型 URL 中的用户名、密码和凭据类参数。
- [ ] 保留安装目录的 `data/`，将新版本应用文件覆盖到目录中；确认现有数据库仍在且可正常打开。
- [ ] 用旧版本数据库启动新版本，确认迁移前生成 `novelstudio.db.pre-migration-v*.bak`，且升级后原项目仍可打开。

## 退出与错误处理

- [ ] 在模型请求进行中按 Ctrl+C，确认服务等待请求结束或在 10 秒后退出。
- [ ] 停止 Go 服务后刷新前端，确认连接错误页提供重试入口和启动命令。
- [ ] 使用无效模型配置和无效备份分别操作，确认错误能被用户看见，且已有章节和项目数据仍可访问。


## World assets

- [ ] Create and edit a place, organization, item, and concept; verify chapter links and related asset links survive refresh.
- [ ] Delete a chapter and confirm its world asset links are cleared; delete a related asset and confirm incoming links are removed.
- [ ] Export and import a backup containing world assets, then verify chapter and relation IDs are remapped.
- [ ] Select and clear world assets in the Agent context panel; verify only selected assets are sent.
- [ ] Switch between the world asset card view and relation view; verify outgoing and incoming links are readable on desktop and narrow screens.
- [ ] Click a linked chapter to return to the editor, then click a linked asset to open its editor dialog.
