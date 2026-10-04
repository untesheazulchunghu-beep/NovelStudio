# NovelStudio - 快速启动指南

## 🚀 现在开始！

构建脚本已准备就绪。由于这是首次构建，需要完成以下步骤：

### 选项 1: 完全自动化（推荐）

打开 PowerShell 或命令提示符，运行：

```cmd
cd C:\Users\w\Desktop\xz
setup-and-build.bat
```

这将自动完成所有设置，大约需要 10-15 分钟。

### 选项 2: 使用已有的 vcpkg

如果你已经安装了 vcpkg（在其他位置），可以直接构建：

```cmd
cd C:\Users\w\Desktop\xz
mkdir build
cd build
cmake .. -DCMAKE_TOOLCHAIN_FILE=你的vcpkg路径/scripts/buildsystems/vcpkg.cmake
cmake --build . --config Release
```

### 选项 3: 逐步手动构建

#### 步骤 1: 安装 vcpkg

```cmd
git clone https://github.com/Microsoft/vcpkg.git C:\vcpkg
cd C:\vcpkg
.\bootstrap-vcpkg.bat
.\vcpkg integrate install
```

#### 步骤 2: 安装依赖

```cmd
C:\vcpkg\vcpkg install sqlite3:x64-windows
C:\vcpkg\vcpkg install curl:x64-windows  
C:\vcpkg\vcpkg install nlohmann-json:x64-windows
```

#### 步骤 3: 构建项目

```cmd
cd C:\Users\w\Desktop\xz
mkdir build
cd build
cmake .. -DCMAKE_TOOLCHAIN_FILE=C:/vcpkg/scripts/buildsystems/vcpkg.cmake -G "Visual Studio 17 2022" -A x64
cmake --build . --config Release
```

#### 步骤 4: 运行

```cmd
build\bin\Release\NovelStudio.exe
```

## 📝 预期输出

应用启动后，你会看到彩色日志输出：

```
=================================================
NovelStudio v0.1.0
Windows AI Novel Writing Platform
=================================================
[2024-10-04 21:30:45.123] [INFO ] Initializing NovelStudio v0.1.0
[2024-10-04 21:30:45.234] [INFO ] Opening database: novelstudio.db
[2024-10-04 21:30:45.345] [INFO ] Database initialized successfully
[2024-10-04 21:30:45.456] [INFO ] Application initialized successfully
```

应用会自动创建示例小说和章节来演示功能。

## 🔧 如果遇到问题

### 问题: "git 不是内部或外部命令"
**解决**: 安装 Git for Windows
- https://git-scm.com/download/win

### 问题: "cmake 不是内部或外部命令"  
**解决**: CMake 应该已安装，但可能需要重启终端

### 问题: "找不到 Visual Studio"
**解决**: 安装 Visual Studio 2022
- Community 版本（免费）: https://visualstudio.microsoft.com/downloads/
- 或安装 Build Tools: https://visualstudio.microsoft.com/downloads/#build-tools-for-visual-studio-2022
- 确保勾选 "Desktop development with C++"

### 问题: 构建过程中断
**解决**: 删除 build 文件夹后重试
```cmd
rmdir /s /q build
setup-and-build.bat
```

## 📚 更多信息

- 详细构建指南: `BUILD_GUIDE.md`
- 项目架构: `docs/ARCHITECTURE.md`
- 开发指南: `docs/DEVELOPMENT.md`

## 🎯 下一步

构建成功后：
1. 运行应用体验基本功能
2. 查看生成的 `novelstudio.db` 数据库
3. 检查 `logs/novelstudio.log` 日志文件
4. 阅读文档了解更多功能

---

**提示**: 首次构建需要下载约 100MB 的依赖包，请确保网络畅通。
