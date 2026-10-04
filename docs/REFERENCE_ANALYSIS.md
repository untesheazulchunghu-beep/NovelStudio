# NovelStudio - 参考项目分析与借鉴

## 参考项目概览

基于对 `ck/` 文件夹中参考项目的深入分析，以下是主要发现和可借鉴之处。

## 1. Cherry Studio - 最完善的桌面架构

### 关键特性
- **四层数据系统**: BootConfig → Cache → Preference → DataApi
- **IoC 容器**: 依赖注入、生命周期管理、自动资源清理
- **严格代码组织**: core/data/ai/features/services 分层
- **Monorepo 设计**: 多个可复用的 packages

### 可借鉴到 NovelStudio
```cpp
// 服务生命周期管理
class ServiceManager {
    enum class Phase { Background, BeforeReady, WhenReady };
    void RegisterService(ServiceBase* service, Phase phase, int priority);
    void InitializeAll();
};

// 四层数据分离
class BootConfig;    // 启动前配置（同步）
class Cache;         // 运行时缓存（内存）
class Preference;    // 用户设置（SQLite）
class DataApi;       // 业务数据（SQLite + ORM）
```

## 2. AI Novel Writer - 版本化向量空间

### 关键特性
- **版本化嵌入空间**: 每个模型/维度组合独立空间
- **安全模型升级**: 新旧代际并存，验证后切换
- **多草稿系统**: 每章保留多个候选稿
- **来源追踪**: 区分"用户填写"、"AI提炼"、"旧数据"

### 可借鉴到 NovelStudio
```cpp
// 版本化向量存储
struct EmbeddingSpace {
    std::string modelFingerprint;  // 模型+维度+语义哈希
    int generation;                 // 代际版本
    bool isActive;                  // 是否激活
    std::vector<Embedding> vectors;
};

// 草稿管理
struct ChapterDraft {
    int64_t chapterId;
    int draftNumber;               // 第几个草稿
    std::string content;
    std::string source;            // "user" | "ai" | "unknown"
    float aiConfidence;            // AI 生成的置信度
};
```

## 3. NovelForge - Schema驱动与工作流

### 关键特性
- **Schema-first 卡片系统**: AI 按 JSON Schema 生成结构化内容
- **代码式工作流**: Python 代码定义流程，支持版本控制
- **工作流 Agent**: 自然语言描述 → AI 生成工作流代码
- **Neo4j 知识图谱**: 追踪角色关系、事件时间线

### 可借鉴到 NovelStudio
```cpp
// Schema驱动生成
struct GenerationSchema {
    std::string type;              // "character" | "scene" | "chapter"
    nlohmann::json schema;         // JSON Schema 定义
    nlohmann::json data;           // 实际数据
};

// 工作流引擎
class Workflow {
public:
    using Task = std::function<nlohmann::json(const nlohmann::json&)>;
    
    void AddStep(const std::string& name, Task task);
    void AddCondition(const std::string& condition, 
                      const std::string& trueStep, 
                      const std::string& falseStep);
    nlohmann::json Execute(const nlohmann::json& input);
};
```

## 4. Luobi AI Writing - 轻量级架构

### 关键特性
- **Repository 模式**: 清晰的数据访问层
- **三重后期管线**: Rewrite → Refine → Review
- **四分屏 IDE**: 文件树 + 编辑器 + AI + 终端
- **MCP 协议集成**: 扩展 AI 能力

### 可借鉴到 NovelStudio
```cpp
// 三阶段润色
enum class PostProcessStage {
    Rewrite,   // 重写：结构调整
    Refine,    // 精修：语言打磨
    Review     // 审阅：质量评估
};

struct PostProcessResult {
    PostProcessStage stage;
    std::string input;
    std::string output;
    std::vector<std::string> suggestions;
    float qualityScore;
};
```

## NovelStudio 架构改进方案

### 第一优先级：四层数据系统（借鉴 Cherry Studio）

```cpp
namespace NovelStudio {

// Layer 1: Boot Configuration (JSON, 启动前加载)
class BootConfig {
public:
    struct Settings {
        bool enableGPU;
        std::string logLevel;
        int maxMemoryMB;
    };
    static Settings Load(const std::string& path);
};

// Layer 2: Cache (内存/跨组件)
class CacheManager {
public:
    void Set(const std::string& key, const nlohmann::json& value, int ttlSeconds = 0);
    std::optional<nlohmann::json> Get(const std::string& key);
    void Clear();
};

// Layer 3: Preference (用户设置)
class PreferenceStore {
public:
    void SetString(const std::string& key, const std::string& value);
    void SetInt(const std::string& key, int value);
    std::string GetString(const std::string& key, const std::string& defaultValue = "");
    int GetInt(const std::string& key, int defaultValue = 0);
};

// Layer 4: DataApi (业务数据，已实现为 Database 类)
// 保持现有 Database 类不变

} // namespace NovelStudio
```

### 第二优先级：AI Provider 增强

```cpp
// 多 Provider 支持
class AIProviderRegistry {
public:
    void Register(const std::string& name, std::unique_ptr<AIProvider> provider);
    AIProvider* GetProvider(const std::string& name);
    std::vector<std::string> ListProviders();
};

// Provider 中间件链
class AIMiddleware {
public:
    virtual ~AIMiddleware() = default;
    virtual std::string Process(const std::string& input, 
                                std::function<std::string(const std::string&)> next) = 0;
};

// 使用示例：
// CacheMiddleware -> RetryMiddleware -> RateLimitMiddleware -> OpenAIProvider
```

### 第三优先级：版本化草稿系统

```cpp
// 草稿版本管理
class DraftManager {
public:
    int64_t CreateDraft(int64_t chapterId, const std::string& content, 
                        const std::string& source);
    std::vector<ChapterDraft> GetDrafts(int64_t chapterId);
    bool SelectDraft(int64_t draftId);  // 设为当前稿
    bool CompareDrafts(int64_t draft1, int64_t draft2, std::string& diff);
};
```

### 第四优先级：Schema驱动生成

```cpp
// AI 生成约束
struct GenerationConstraints {
    nlohmann::json schema;             // JSON Schema
    std::vector<std::string> examples; // 示例输出
    int maxAttempts = 3;               // 最大重试次数
};

class StructuredAIProvider : public AIProvider {
public:
    nlohmann::json GenerateStructured(const std::vector<AIMessage>& messages,
                                      const GenerationConstraints& constraints);
};
```

## 实现优先级建议

### 立即实现（1-2周）
1. ✅ 基础数据库层（已完成）
2. ✅ AI Provider 接口（已完成）
3. ⏳ 四层数据系统
4. ⏳ 配置文件支持
5. ⏳ 日志系统

### 短期实现（3-4周）
6. 多 Provider 支持（OpenAI + Claude + Local）
7. 草稿版本系统
8. 中间件链（缓存、重试、限流）
9. UI 框架选择与基础窗口

### 中期实现（2-3月）
10. Schema驱动生成
11. 工作流引擎
12. 知识图谱（角色关系）
13. 向量检索（RAG）

### 长期实现（3-6月）
14. 完整 UI/UX
15. 导出功能（DOCX/PDF/EPUB）
16. 统计分析
17. 插件系统

## 关键技术对比

| 特性 | Cherry Studio | AI Novel Writer | NovelForge | Luobi | NovelStudio 计划 |
|-----|---------------|-----------------|------------|-------|------------------|
| 语言 | TS+Electron | TS+Electron | Python+Vue | TS+Electron | C++20 |
| 数据库 | SQLite+Drizzle | SQLite+better-sqlite3 | PostgreSQL | SQLite | SQLite |
| 向量 | - | LanceDB | - | LanceDB | 计划 Faiss/Hnswlib |
| UI | React 19 | React 19 | Vue 3 | React 19 | WinUI 3/Qt |
| IoC | ✅ | ❌ | ❌ | ❌ | 计划实现 |
| 工作流 | MCP扩展 | 内置流程 | 代码式+可视化 | MCP扩展 | 计划代码式 |

## 总结

NovelStudio 的 C++ 实现可以结合：
- **Cherry Studio** 的架构严谨性
- **AI Novel Writer** 的版本管理
- **NovelForge** 的工作流能力
- **Luobi** 的简洁实用性

同时利用 C++ 的性能优势，打造一个真正的 Windows 原生、高性能的 AI 小说写作平台。
