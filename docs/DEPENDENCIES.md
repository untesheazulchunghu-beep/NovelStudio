# Third-party Dependencies

This document tracks external libraries and their usage.

## Core Dependencies

### Database
- **SQLite3** - Embedded database
  - License: Public Domain
  - Version: 3.40+
  - Usage: All data persistence

### HTTP Client
- ~~**libcurl**~~ - **REMOVED** ✅
- **WinHTTP (Windows native)** - HTTP/HTTPS client
  - License: Built into Windows
  - Usage: AI API communication
  - Benefits: No external dependencies, native Windows integration

### JSON
- ~~**nlohmann/json**~~ - **REMOVED** ✅
- **Custom lightweight JSON parser** - In-house implementation
  - License: MIT (part of project)
  - Location: `src/utils/json_utils.cpp`
  - Usage: API request/response parsing
  - Benefits: No external dependencies, ~500 lines of code

## Optional Dependencies

### UI Framework Option A: WinUI 3
- **Microsoft.WindowsAppSDK**
  - License: MIT
  - Version: 1.4+
  - Usage: Modern Windows UI

- **WinRT C++**
  - License: MIT
  - Version: 2.0+
  - Usage: Windows Runtime projections

### UI Framework Option B: Qt
- **Qt6**
  - License: LGPL v3 or Commercial
  - Version: 6.5+
  - Modules: Core, Widgets, Network, Sql

### Logging
- **spdlog** - Fast C++ logging
  - License: MIT
  - Version: 1.12+
  - Usage: Application logging

## Development Dependencies

### Testing
- **Google Test**
  - License: BSD-3-Clause
  - Version: 1.14+
  - Usage: Unit tests

### Build
- **CMake**
  - License: BSD-3-Clause
  - Version: 3.20+
  - Usage: Build system

## Installation

### vcpkg (Minimal Setup)
```bash
# Only SQLite3 is required as external dependency
vcpkg install sqlite3:x64-windows

# Optional: For testing
vcpkg install gtest:x64-windows
```

### Manual
SQLite3 can also be built from source or downloaded as amalgamation.

## Dependency Reduction Strategy

We've minimized external dependencies by:
1. **Using Windows native APIs** - WinHTTP for HTTP/HTTPS
2. **Custom implementations** - Lightweight JSON parser (~500 lines)
3. **Standard library first** - Using C++20 features instead of libraries

This approach reduces:
- Build complexity
- Binary size
- Potential security vulnerabilities
- Update/maintenance burden

## License Compatibility

All dependencies are compatible with MIT license distribution.
