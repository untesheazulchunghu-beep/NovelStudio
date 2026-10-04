# NovelStudio - AI-Powered Novel Writing Platform

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform: Windows](https://img.shields.io/badge/Platform-Windows-blue.svg)](https://www.microsoft.com/windows)
[![C++: 20](https://img.shields.io/badge/C%2B%2B-20-00599C.svg)](https://isocpp.org/)

A modern Windows-native AI-powered novel writing platform built with C++20. NovelStudio combines the power of local data storage with cloud AI services to create an immersive writing environment for authors.

![NovelStudio Screenshot](docs/images/screenshot-placeholder.png)

## ✨ Features

### 📝 Core Writing Tools
- **Rich Text Editor** - Distraction-free writing environment
- **Chapter Organization** - Structured novel management with drag-and-drop
- **Version Control** - Track revisions and maintain writing history
- **Auto-Save** - Never lose your work

### 🤖 AI-Powered Assistance
- **Smart Continuations** - AI-powered text generation
- **Multiple AI Providers** - OpenAI, Anthropic Claude, Local models
- **Context-Aware** - Maintains character and plot consistency
- **Custom Prompts** - Tailored generation templates

### 📊 Organization & Planning
- **Character Database** - Track character details, traits, and arcs
- **Plot Timeline** - Visualize story progression
- **World Building** - Manage settings, locations, and lore
- **Research Notes** - Attach references and inspiration

### 🎯 Productivity Features
- **Word Count Goals** - Daily/weekly targets and tracking
- **Writing Statistics** - Detailed analytics and insights
- **Export Options** - DOCX, PDF, EPUB, Markdown
- **Offline Mode** - Work without internet (AI requires connection)

## 🚀 Getting Started

### Prerequisites

- **Windows 10/11** (64-bit)
- **Visual Studio 2022** or later
- **CMake 3.20+**
- **vcpkg** (package manager)

### Quick Start

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/NovelStudio.git
   cd NovelStudio
   ```

2. **Install dependencies**
   ```bash
   vcpkg install sqlite3:x64-windows curl:x64-windows nlohmann-json:x64-windows
   ```

3. **Build the project**
   ```bash
   build.bat
   ```

4. **Run NovelStudio**
   ```bash
   build\bin\Release\NovelStudio.exe
   ```

For detailed instructions, see [DEVELOPMENT.md](docs/DEVELOPMENT.md).

## 📖 Documentation

- [Architecture Overview](docs/ARCHITECTURE.md)
- [Development Guide](docs/DEVELOPMENT.md)
- [Project Roadmap](docs/ROADMAP.md)
- [Dependencies](docs/DEPENDENCIES.md)
- [Contributing Guidelines](CONTRIBUTING.md)

## 🎨 Inspired By

NovelStudio draws inspiration from several excellent open-source projects:

- **[Cherry Studio](https://github.com/kangfenmao/cherry-studio)** - Architecture patterns and service lifecycle
- **[AI Novel Writer](https://github.com/yym68686/AI-Novel-Writer)** - Version-controlled embeddings and multi-draft system
- **[NovelForge](https://github.com/XzaiCloud/NovelForge)** - Schema-first AI generation and workflow engine
- **[Luobi AI Writing](https://github.com/mefengl/luobi-ai-writing)** - Clean architecture and repository pattern

## 🏗️ Technology Stack

- **Language**: C++20/23
- **Build System**: CMake
- **Database**: SQLite3
- **HTTP Client**: libcurl
- **JSON**: nlohmann/json
- **UI**: WinUI 3 (planned) or Qt6
- **Testing**: Google Test

## 📸 Screenshots

> Coming soon! Currently in development phase.

## 🗺️ Roadmap

### Phase 1: Foundation ✅ (Current)
- [x] Project structure
- [x] Database layer
- [x] AI provider interface
- [ ] Error handling & logging

### Phase 2: User Interface (Q1 2025)
- [ ] Main window and navigation
- [ ] Rich text editor
- [ ] Chapter management UI
- [ ] Settings dialog

### Phase 3: AI Features (Q2 2025)
- [ ] Text generation
- [ ] Context management
- [ ] Multi-provider support
- [ ] Prompt templates

### Phase 4: Advanced Features (Q3 2025)
- [ ] Version control UI
- [ ] Export functionality
- [ ] Analytics dashboard
- [ ] Plugin system

See the full [Roadmap](docs/ROADMAP.md) for details.

## 🤝 Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Ways to Contribute

- 🐛 Report bugs
- 💡 Suggest features
- 📝 Improve documentation
- 🔧 Submit pull requests
- 🧪 Write tests

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- Thanks to all the open-source projects that inspired this work
- The C++ and Windows development communities
- AI providers (OpenAI, Anthropic) for making their APIs accessible
- All contributors and testers

## 📧 Contact

- **Issues**: [GitHub Issues](https://github.com/yourusername/NovelStudio/issues)
- **Discussions**: [GitHub Discussions](https://github.com/yourusername/NovelStudio/discussions)

## ⭐ Star History

If you find NovelStudio useful, please consider giving it a star!

---

**Made with ❤️ for writers everywhere**
