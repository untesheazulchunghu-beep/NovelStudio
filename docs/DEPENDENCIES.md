# Third-party Dependencies

This document tracks external libraries and their usage.

## Core Dependencies

### Database
- **SQLite3** - Embedded database
  - License: Public Domain
  - Version: 3.40+
  - Usage: All data persistence

### HTTP Client
- **libcurl** - HTTP/HTTPS client
  - License: curl License (MIT-style)
  - Version: 8.0+
  - Usage: AI API communication

### JSON
- **nlohmann/json** - JSON library
  - License: MIT
  - Version: 3.11+
  - Usage: API request/response parsing

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

### vcpkg (Recommended)
```bash
vcpkg install sqlite3 curl nlohmann-json spdlog gtest
```

### Manual
See individual library documentation for manual installation.

## License Compatibility

All dependencies are compatible with MIT license distribution.
