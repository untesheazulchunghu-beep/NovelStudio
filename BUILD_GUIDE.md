# NovelStudio - 构建和运行指南

## 快速开始

### 方式 1: 自动安装和构建（推荐）

只需运行一个脚本即可完成所有设置：

```bash
setup-and-build.bat
```

这个脚本会自动：
1. ✅ 检测并安装 vcpkg（如果未安装）
2. ✅ 安装所有依赖（SQLite3, curl, nlohmann-json）
3. ✅ 配置 CMake
4. ✅ 编译项目
5. ✅ 创建快速运行脚本

首次运行需要 10-15 分钟（下载依赖），之后重新编译只需几秒。

### 方式 2: 手动构建

如果你已经安装了 vcpkg 和依赖：

```bash
# 创建构建目录
mkdir build
cd build

# 配置 CMake
cmake .. -DCMAKE_TOOLCHAIN_FILE=C:/vcpkg/scripts/buildsystems/vcpkg.cmake

# 编译
cmake --build . --config Release
```

## 运行应用

### 编译后运行

```bash
# 使用快速运行脚本
quick-run.bat

# 或直接运行
build\bin\Release\NovelStudio.exe
```

### 预期输出

启动后，你应该看到：

```
=================================================
NovelStudio v0.1.0
Windows AI Novel Writing Platform
=================================================
[2024-10-04 21:30:45.123] [INFO ] Initializing NovelStudio v0.1.0
[2024-10-04 21:30:45.234] [INFO ] Opening database: novelstudio.db
[2024-10-04 21:30:45.345] [INFO ] Database initialized successfully
[2024-10-04 21:30:45.456] [INFO ] Application initialized successfully
[2024-10-04 21:30:45.567] [INFO ] Database: novelstudio.db
[2024-10-04 21:30:45.678] [INFO ] Database connection verified
[2024-10-04 21:30:45.789] [INFO ] Found 0 novel(s) in database
[2024-10-04 21:30:45.890] [INFO ] Creating sample novel...
[2024-10-04 21:30:46.001] [INFO ] Sample novel created with ID: 1
[2024-10-04 21:30:46.112] [INFO ] Sample chapter created with ID: 1

Press Enter to exit...
```

## 系统要求

### 必需
- Windows 10/11 (64-bit)
- Visual Studio 2022 或 Visual Studio Build Tools 2022
- CMake 3.20+
- Git

### 可选
- Visual Studio Code（用于开发）
- vcpkg（自动安装脚本会安装）

## 依赖库

项目使用以下第三方库：

| 库 | 版本 | 用途 |
|----|------|------|
| SQLite3 | 3.40+ | 本地数据库 |
| libcurl | 8.0+ | HTTP 请求（AI API） |
| nlohmann-json | 3.11+ | JSON 解析 |

所有依赖都通过 vcpkg 自动安装。

## 故障排除

### 问题 1: "CMake not found"
**解决方案**: 安装 CMake
```bash
# 下载并安装 CMake
# https://cmake.org/download/
```

### 问题 2: "git not found"
**解决方案**: 安装 Git
```bash
# 下载并安装 Git for Windows
# https://git-scm.com/download/win
```

### 问题 3: "Visual Studio not found"
**解决方案**: 安装 Visual Studio 2022
```bash
# 安装 Visual Studio 2022 Community（免费）
# https://visualstudio.microsoft.com/downloads/
# 
# 或安装 Build Tools:
# https://visualstudio.microsoft.com/downloads/#build-tools-for-visual-studio-2022
```

### 问题 4: 编译错误
**解决方案**: 清理并重新编译
```bash
# 删除 build 目录
rmdir /s /q build

# 重新运行设置脚本
setup-and-build.bat
```

### 问题 5: vcpkg 安装缓慢
**解决方案**: 使用国内镜像（可选）
```bash
# 在 C:\vcpkg 目录下
git config --global http.proxy http://your-proxy:port
```

## 开发模式

### 快速重新编译

修改代码后，使用快速编译脚本：

```bash
quick-build.bat
```

### 在 Visual Studio 中打开

```bash
# 生成 Visual Studio 解决方案
cd build
cmake .. -G "Visual Studio 17 2022" -A x64

# 打开 NovelStudio.sln
start NovelStudio.sln
```

### 在 VS Code 中开发

1. 打开项目文件夹
2. 安装 C/C++ 扩展
3. 按 `F7` 编译
4. 按 `F5` 调试

## 日志文件

应用运行后会在 `logs/` 目录创建日志：

```
logs/
└── novelstudio.log
```

日志包含：
- 应用启动信息
- 数据库操作
- AI 请求（如果配置了 API）
- 错误和警告

## 数据文件

应用数据存储在：

```
novelstudio.db       # SQLite 数据库
config.json          # 配置文件（需手动创建）
logs/               # 日志目录
```

## 配置 AI API

要启用 AI 功能，需要配置 API 密钥：

1. 复制配置模板：
   ```bash
   copy config.example.json config.json
   ```

2. 编辑 `config.json`，填入你的 API 密钥：
   ```json
   {
     "ai": {
       "defaultProvider": "openai",
       "apiKey": "sk-your-api-key-here",
       "defaultModel": "gpt-3.5-turbo"
     }
   }
   ```

3. 重新运行应用

## 下一步

- 📖 阅读 [ARCHITECTURE.md](docs/ARCHITECTURE.md) 了解系统架构
- 🛠️ 阅读 [DEVELOPMENT.md](docs/DEVELOPMENT.md) 学习开发
- 🗺️ 查看 [ROADMAP.md](docs/ROADMAP.md) 了解未来计划

## 获取帮助

- GitHub Issues: [项目地址]/issues
- 文档: docs/ 目录
- 示例代码: tests/ 目录

---

**首次编译可能需要 10-15 分钟来下载依赖，请耐心等待。**
