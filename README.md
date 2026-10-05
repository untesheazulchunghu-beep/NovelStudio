# NovelStudio

**Windows 原生 AI 驱动小说写作平台** - 最小依赖，最大功能

## ✨ 特性

- 🚀 **极简依赖**: 仅 1 个外部库 (SQLite3, 可选)
- 🤖 **多 AI 提供商**: OpenAI、Anthropic Claude、Ollama 本地模型
- ⚡ **实时流式**: Server-Sent Events 流式响应
- 💾 **内存数据管理**: 无需数据库即可使用
- 🎨 **5 个演示程序**: 从测试到完整工作流

## 🚀 快速开始

### 一键启动
```bash
start.bat
```

### 手动构建
```bash
# 配置
cmake -G "Visual Studio 18 2026" -A x64 -B build

# 构建
cmake --build build --config Release

# 运行
build\bin\Release\DemoWorkflow.exe
```

### 配置 API Keys
```bash
copy config.example.json config.json
# 编辑 config.json，填入你的 API keys
```

## 📦 可执行程序

| 程序 | 大小 | 功能 |
|------|------|------|
| **DemoWorkflow** | 200KB | 🌟 完整写作工作流 |
| **DemoWritingAssistant** | 164KB | AI 助手演示 |
| **TestProviders** | 150KB | 多提供商测试 |
| **TestStreaming** | 117KB | 流式响应测试 |
| **TestNative** | 67KB | 原生实现测试 |

## 🎯 核心功能

### 写作工作流 (DemoWorkflow)
1. ✍️ **创建小说** - AI 生成描述
2. 📖 **写作章节** - 手动或 AI 生成
3. 🔄 **AI 续写** - 基于上下文继续创作
4. 👤 **角色管理** - AI 生成角色描述
5. 📊 **统计信息** - 字数统计、章节列表

### AI 集成
- **OpenAI**: GPT-3.5, GPT-4
- **Anthropic**: Claude 3.5 Sonnet
- **Ollama**: Llama 3.2, Qwen 2.5, 等

## 🏗️ 技术栈

- **语言**: C++20/23
- **构建**: CMake + Visual Studio 2026
- **HTTP**: Windows WinHTTP (原生)
- **JSON**: 自实现解析器 (~500 行)
- **日志**: 自实现 (~200 行)
- **数据**: 内存存储 (可选 SQLite3)

## 📊 项目规模

```
代码统计:
- 总文件数: 38
- 总代码行: 6,052
- 外部依赖: 1 个 (SQLite3, 可选)
- AI 提供商: 3 个
- 演示程序: 5 个
```

## 🎨 架构

```
应用层
├── DemoWorkflow (完整工作流)
└── DemoWritingAssistant (AI 助手)

服务层
├── AIService (统一 AI 接口)
├── ConfigManager (配置管理)
└── DataManager (数据管理)

提供商层
├── OpenAI Provider
├── Anthropic Provider
└── Local/Ollama Provider

基础设施
├── HTTP Client (WinHTTP)
├── JSON Parser (自实现)
├── SSE Parser (流式)
└── Logger (日志)
```

## 📝 使用示例

### 创建小说
```cpp
auto& data = DataManager::GetInstance();
int novelId = data.CreateNovel("我的小说", "作者名");
```

### AI 生成章节
```cpp
auto& ai = AIService::GetInstance();
ai.Initialize(AIService::ProviderType::OpenAI, apiKey, endpoint);

std::string content = ai.GenerateFromPrompt(
    "You are a creative writer.",
    "Write chapter 1 of a sci-fi novel...",
    "gpt-4",
    2000
);
```

### 流式续写
```cpp
ai.CompleteStreaming(messages, 
    [](const std::string& chunk) {
        std::cout << chunk;
    },
    options
);
```

## 🔧 开发

### 依赖减少对比

| 组件 | 之前 | 之后 | 减少 |
|------|------|------|------|
| HTTP | libcurl | WinHTTP | ✓ |
| JSON | nlohmann/json | 自实现 | ✓ |
| 日志 | spdlog | 自实现 | ✓ |
| 数据库 | SQLite3 | SQLite3 | - |
| **总计** | **4 个** | **1 个** | **75%** |

### 构建目标

```bash
# 所有演示 (不需要 SQLite3)
cmake --build build --config Release

# 特定程序
cmake --build build --config Release --target DemoWorkflow
cmake --build build --config Release --target TestNative
```

## 📚 文档

- [开发总结](DEVELOPMENT_SUMMARY.md) - 完整开发过程
- [进度追踪](PROGRESS.md) - Phase 1-2 完成情况
- [原生实现](docs/NATIVE_IMPLEMENTATION.md) - HTTP/JSON 实现细节
- [依赖说明](docs/DEPENDENCIES.md) - 依赖减少策略

## 🎯 路线图

- [x] Phase 1: 依赖减少
- [x] Phase 2: AI 集成
- [ ] Phase 3: UI 界面 (WinUI 3 / Qt 6)
- [ ] Phase 4: 高级功能 (导出, 版本控制)

## 📄 许可

MIT License

## 🙏 致谢

参考项目:
- luobi-ai-writing - Clean Architecture
- AI-Novel-Writer - 多草稿系统
- Cherry Studio - 服务架构
- NovelForge - 工作流设计

---

**开发状态**: Phase 2 完成 ✅  
**可用性**: 生产可用 🚀  
**依赖数**: 1 个 (最小化) 🎯
