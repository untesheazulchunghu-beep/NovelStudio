# NovelStudio Architecture

## Overview

NovelStudio is a Windows-native AI-powered novel writing platform built with modern C++. It combines local data storage with cloud AI services to provide a powerful writing environment.

## Architecture Layers

### 1. Core Layer (`src/core/`)
- **Application**: Main application lifecycle management
- **Configuration**: App settings and preferences
- **Event System**: Event bus for loose coupling between components

### 2. Database Layer (`src/database/`)
- **Database**: SQLite-based persistence
- **Entities**: Novel, Chapter, Character, Revision
- **Migrations**: Schema versioning

### 3. AI Layer (`src/ai/`)
- **AIProvider**: Abstract interface for AI services
- **OpenAIProvider**: OpenAI API integration
- **AnthropicProvider**: Claude API integration
- **LocalProvider**: Local model support (Ollama, LM Studio)
- **Prompts**: Template system for AI prompts

### 4. UI Layer (`src/ui/`)
Two potential approaches:

#### Option A: WinUI 3 (Recommended)
- Modern Windows-native UI
- Fluent Design System
- XAML-based declarative UI
- Better Windows 11 integration

#### Option B: Qt 6
- Cross-platform (future Linux/Mac support)
- More mature ecosystem
- Rich widget library

### 5. Utils Layer (`src/utils/`)
- **Logger**: Logging system
- **FileIO**: File operations
- **StringUtils**: String manipulation
- **JsonUtils**: JSON parsing/serialization

## Data Flow

```
User Input → UI Layer → Core Logic → Database/AI → Update UI
```

## Key Design Patterns

### 1. Repository Pattern
All database access goes through repository classes that abstract SQL details.

### 2. Strategy Pattern
AI providers implement a common interface, allowing runtime provider switching.

### 3. Observer Pattern
UI components subscribe to data changes through an event system.

### 4. Command Pattern
User actions are encapsulated as commands for undo/redo support.

## Dependencies

### Core Dependencies
- **SQLite3**: Local database
- **libcurl**: HTTP client
- **nlohmann/json**: JSON parsing
- **spdlog**: Logging (planned)

### UI Dependencies (WinUI 3)
- **Microsoft.WindowsAppSDK**
- **Microsoft.Windows.CppWinRT**

### UI Dependencies (Qt alternative)
- **Qt6Core**
- **Qt6Widgets**
- **Qt6Network**

## Future Enhancements

### Phase 1 (Current)
- Basic CRUD operations
- Simple AI integration
- Console-based testing

### Phase 2
- Full UI implementation
- Rich text editor
- Chapter organization

### Phase 3
- AI writing assistant features
- Character/plot tracking
- Export functionality

### Phase 4
- Version control integration
- Collaboration features
- Plugin system

## Inspired By

Based on analysis of reference projects in `ck/`:
- **Cherry Studio**: IoC container pattern, service lifecycle
- **AI Novel Writer**: Version-controlled embeddings, multi-draft system
- **NovelForge**: Schema-first AI generation, workflow engine
- **Luobi**: Repository pattern, clean architecture
