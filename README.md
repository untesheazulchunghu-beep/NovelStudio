# NovelStudio - Windows AI Novel Writing Platform

A Windows-native AI-powered novel writing platform built with modern C++.

## Features

- **AI-Powered Writing Assistant**: Integrated LLM support for creative writing
- **Rich Text Editor**: Native Windows editing experience
- **Chapter Management**: Organize and manage novel chapters
- **Character & Plot Tracking**: Keep track of characters, plots, and story elements
- **Version Control**: Track revisions and history
- **Export Options**: Export to multiple formats (TXT, DOCX, PDF)

## Technology Stack

- **Language**: C++20/23
- **UI Framework**: To be determined (WinUI 3 / Qt)
- **Database**: SQLite
- **HTTP Client**: cpr / cpp-httplib
- **JSON**: nlohmann/json
- **Build System**: CMake + Visual Studio 2022

## Project Structure

```
NovelStudio/
├── src/
│   ├── core/           # Core business logic
│   ├── ui/             # UI components
│   ├── ai/             # AI/LLM integration
│   ├── database/       # Database layer
│   └── utils/          # Utilities
├── include/            # Header files
├── tests/              # Unit tests
├── resources/          # Assets, icons, etc.
└── docs/               # Documentation
```

## Building

Requirements:
- Visual Studio 2022 or later
- Windows 10/11 SDK
- CMake 3.20+

```bash
mkdir build
cd build
cmake ..
cmake --build . --config Release
```

## License

TBD
