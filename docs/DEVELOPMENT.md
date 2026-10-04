# NovelStudio Development Guide

## Prerequisites

### Required Software
- **Visual Studio 2022** (17.8 or later)
  - Desktop development with C++ workload
  - Windows 10/11 SDK
  - C++ CMake tools

- **vcpkg** (Package Manager)
  ```bash
  git clone https://github.com/Microsoft/vcpkg.git
  cd vcpkg
  ./bootstrap-vcpkg.bat
  ./vcpkg integrate install
  ```

### Optional Tools
- **Git** for version control
- **CLion** or **VS Code** (alternative IDEs)
- **Doxygen** for documentation generation

## Initial Setup

### 1. Clone Repository
```bash
git clone <repository-url>
cd NovelStudio
```

### 2. Install Dependencies
```bash
vcpkg install sqlite3:x64-windows
vcpkg install curl:x64-windows
vcpkg install nlohmann-json:x64-windows
vcpkg install spdlog:x64-windows
vcpkg install gtest:x64-windows
```

### 3. Configure CMake
```bash
mkdir build
cd build
cmake .. -DCMAKE_TOOLCHAIN_FILE=[vcpkg root]/scripts/buildsystems/vcpkg.cmake
```

### 4. Build Project
```bash
# Debug build
cmake --build . --config Debug

# Release build
cmake --build . --config Release
```

## Project Structure

```
NovelStudio/
├── src/
│   ├── core/           # Application core
│   ├── ui/             # User interface
│   ├── ai/             # AI integration
│   ├── database/       # Data persistence
│   └── utils/          # Utilities
├── include/            # Public headers
├── tests/              # Unit tests
├── resources/          # Assets, icons
├── docs/               # Documentation
├── third_party/        # External libs (if needed)
└── CMakeLists.txt
```

## Building with Visual Studio

### Method 1: CMake Project
1. Open Visual Studio 2022
2. File → Open → CMake
3. Select `CMakeLists.txt`
4. Choose configuration (Debug/Release)
5. Build → Build All

### Method 2: Generate Solution
```bash
cd build
cmake .. -G "Visual Studio 17 2022" -A x64
```
Then open `NovelStudio.sln` in Visual Studio.

## Running Tests

```bash
cd build
ctest --output-on-failure
```

Or in Visual Studio:
- Test → Run All Tests

## Code Style

### Formatting
- **Indentation**: 4 spaces
- **Line length**: 100 characters max
- **Naming conventions**:
  - Classes: `PascalCase`
  - Functions: `PascalCase`
  - Variables: `camelCase`
  - Constants: `UPPER_CASE`
  - Private members: `camelCase_` (trailing underscore)

### Example
```cpp
class NovelManager {
public:
    bool CreateNovel(const std::string& title);
    
private:
    void UpdateCache();
    std::string databasePath_;
    static const int MAX_CHAPTERS = 1000;
};
```

## Debugging

### Visual Studio
1. Set breakpoints in source files
2. Press F5 to start debugging
3. Use Watch windows for variables
4. Check Output window for logs

### Common Issues

**Issue**: CMake can't find SQLite
```bash
# Solution: Specify vcpkg toolchain
cmake .. -DCMAKE_TOOLCHAIN_FILE=C:/path/to/vcpkg/scripts/buildsystems/vcpkg.cmake
```

**Issue**: Link errors with curl
```bash
# Solution: Install correct architecture
vcpkg install curl:x64-windows
```

**Issue**: Windows SDK not found
- Install Windows 10/11 SDK via Visual Studio Installer

## Database Development

### Schema Changes
1. Modify `src/database/database.cpp` → `InitializeTables()`
2. Add migration logic if needed
3. Update entity structs in `include/database.h`
4. Test with fresh database

### Viewing Database
```bash
sqlite3 novelstudio.db
.schema
SELECT * FROM novels;
```

## AI Provider Testing

### Mock Testing
Create `config.json`:
```json
{
  "ai": {
    "provider": "openai",
    "apiKey": "sk-...",
    "model": "gpt-3.5-turbo",
    "temperature": 0.7
  }
}
```

### Testing without API
Use local provider (future):
```cpp
auto provider = AIProvider::CreateLocal();
provider->Initialize("", "http://localhost:11434");
```

## Contributing

### Workflow
1. Create feature branch: `git checkout -b feature/my-feature`
2. Make changes
3. Write tests
4. Commit: `git commit -m "feat: add my feature"`
5. Push: `git push origin feature/my-feature`
6. Create pull request

### Commit Message Format
```
type(scope): subject

body (optional)

footer (optional)
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

## Performance Profiling

### Visual Studio Profiler
1. Debug → Performance Profiler
2. Select CPU Usage or Memory Usage
3. Start profiling
4. Analyze results

### Memory Leak Detection
Enable CRT debug heap:
```cpp
#define _CRTDBG_MAP_ALLOC
#include <crtdbg.h>

int main() {
    _CrtSetDbgFlag(_CRTDBG_ALLOC_MEM_DF | _CRTDBG_LEAK_CHECK_DF);
    // Your code
}
```

## Continuous Integration

### GitHub Actions (Future)
```yaml
# .github/workflows/build.yml
name: Build
on: [push, pull_request]
jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v3
      - name: Setup vcpkg
        run: |
          git clone https://github.com/Microsoft/vcpkg.git
          .\vcpkg\bootstrap-vcpkg.bat
      - name: Build
        run: |
          mkdir build
          cd build
          cmake .. -DCMAKE_TOOLCHAIN_FILE=../vcpkg/scripts/buildsystems/vcpkg.cmake
          cmake --build . --config Release
```

## Resources

- [Modern CMake](https://cliutils.gitlab.io/modern-cmake/)
- [C++ Core Guidelines](https://isocpp.github.io/CppCoreGuidelines/)
- [vcpkg Documentation](https://vcpkg.io/)
- [SQLite Documentation](https://www.sqlite.org/docs.html)
- [OpenAI API Reference](https://platform.openai.com/docs/api-reference)

## Getting Help

- Check `docs/` folder for architecture and design docs
- Review `tests/` for usage examples
- Open an issue on GitHub
- Contact the development team
