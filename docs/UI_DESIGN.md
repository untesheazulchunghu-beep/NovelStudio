# NovelStudio UI Design Specification

## Overview
This document outlines the UI/UX design for NovelStudio's desktop interface.

## Design Principles

1. **Writer-First**: Minimize distractions, maximize writing space
2. **Context-Aware**: Show relevant information without clutter
3. **Responsive**: Smooth interactions and immediate feedback
4. **Professional**: Clean, modern aesthetic suitable for serious work

## Color Palette

### Dark Theme (Default)
```
Background:     #0F172A (Deep Navy)
Surface:        #1E293B (Slate)
Border:         #334155 (Slate Border)
Text Primary:   #F8FAFC (Off-White)
Text Secondary: #CBD5E1 (Light Slate)

Accent Primary:   #38BDF8 (Sky Blue)
Accent Secondary: #F97316 (Orange)
Success:          #10B981 (Green)
Warning:          #F59E0B (Amber)
Error:            #EF4444 (Red)
```

### Light Theme
```
Background:     #FFFFFF (White)
Surface:        #F8FAFC (Off-White)
Border:         #E2E8F0 (Light Border)
Text Primary:   #0F172A (Dark Navy)
Text Secondary: #64748B (Slate Gray)

(Same accent colors)
```

## Layout Structure

```
┌─────────────────────────────────────────────────────────────┐
│ ┌─────────┐ NovelStudio            [_] [□] [X]             │
│ │  File   │ Edit  View  Insert  Tools  Help                │
├─┴─────────┴──────────────────────────────────────────────┬──┤
│ ┌──────────────────┐                                      │AI│
│ │ 📚 My Novels     │  MAIN EDITOR AREA                   │  │
│ │                  │  ┌────────────────────────────┐     │🤖│
│ │ > Project Alpha  │  │ Chapter 1: The Beginning   │     │  │
│ │   • Chapter 1    │  │ ──────────────────────────│     │💬│
│ │   • Chapter 2    │  │                            │     │  │
│ │   • Chapter 3    │  │ [Writing content here...]  │     │📊│
│ │                  │  │                            │     │  │
│ │ > Project Beta   │  │                            │     │⚙️│
│ │                  │  │                            │     │  │
│ │ [+ New Novel]    │  │                            │     │  │
│ │                  │  └────────────────────────────┘     │  │
│ │ ┌──────────────┐ │                                      │  │
│ │ │ 👤 Characters│ │  Words: 1,234 | Goal: 2,000         │  │
│ │ └──────────────┘ │  Chapter 1 of 10                     │  │
│ └──────────────────┘                                      │  │
└───────────────────────────────────────────────────────────┴──┘
```

## Main Components

### 1. Title Bar
- Application title
- Standard window controls (minimize, maximize, close)
- Quick access icons (save, sync status)

### 2. Menu Bar
- **File**: New, Open, Save, Export, Recent, Exit
- **Edit**: Undo, Redo, Cut, Copy, Paste, Find, Replace
- **View**: Sidebar, AI Panel, Focus Mode, Theme
- **Insert**: Chapter, Character, Note, Image
- **Tools**: Statistics, Preferences, AI Settings
- **Help**: Documentation, Keyboard Shortcuts, About

### 3. Left Sidebar (Novel Navigator)

#### Projects List
- Expandable tree structure
- Novel → Chapters hierarchy
- Drag & drop to reorder
- Context menu on right-click
- Icons for status (draft, reviewing, complete)

#### Quick Actions
- New Novel button
- Search novels/chapters
- Filter by status

#### Characters Panel (Collapsible)
- List of characters for current novel
- Quick view of character details
- Add/edit character button

### 4. Main Editor

#### Editor Toolbar
```
[B] [I] [U] [S] | [H1] [H2] | [""] | [🔗] [📷] | [⚙️]
```
- Basic formatting (bold, italic, underline, strikethrough)
- Heading styles
- Quote blocks
- Insert link, image
- Editor settings

#### Writing Area
- Distraction-free editor
- Line numbers (optional)
- Syntax highlighting for markdown
- Auto-save indicator
- Word count live update
- Chapter title at top
- Clean, readable font (Georgia, Palatino)

#### Status Bar
- Word count (current / goal)
- Chapter progress (1 of 10)
- Character count
- Reading time estimate
- Last saved time

### 5. Right Panel (AI Assistant)

#### Tabs
- 🤖 **AI Chat**: Conversation with AI
- 💬 **Suggestions**: AI-generated ideas
- 📊 **Stats**: Writing statistics
- ⚙️ **Settings**: Quick settings

#### AI Chat Tab
```
┌──────────────────────────┐
│ Continue this scene...   │
│ ▶ Generate              │
├──────────────────────────┤
│ AI: Here's a suggestion: │
│ "The door creaked open..." │
│ [Use] [Regenerate]       │
├──────────────────────────┤
│ [Type your request...]   │
└──────────────────────────┘
```

#### Stats Tab
- Words written today/this week
- Writing streak
- Progress chart
- Time spent writing
- Average words per session

### 6. Focus Mode
- Hide sidebar and AI panel
- Fullscreen editor
- Minimal UI (ESC to exit)
- Transparent overlay controls on hover

## Interactions

### Writing Flow
1. User selects chapter from sidebar
2. Editor loads chapter content
3. User types naturally
4. Auto-save every 30 seconds
5. AI suggests on demand or auto-complete

### AI Interactions
1. User highlights text → right-click → "Ask AI"
2. Or type in AI panel
3. AI responds with suggestions
4. User can accept, modify, or reject

### Chapter Management
1. Drag chapters to reorder
2. Right-click for options:
   - Rename
   - Duplicate
   - Move to...
   - Delete
   - Export

## Keyboard Shortcuts

### Essential
- `Ctrl+N`: New novel
- `Ctrl+S`: Save
- `Ctrl+Shift+S`: Save as
- `Ctrl+E`: Export
- `Ctrl+,`: Preferences

### Editor
- `Ctrl+B`: Bold
- `Ctrl+I`: Italic
- `Ctrl+U`: Underline
- `Ctrl+K`: Insert link
- `Ctrl+/`: Comment

### Navigation
- `Ctrl+Tab`: Next chapter
- `Ctrl+Shift+Tab`: Previous chapter
- `Ctrl+P`: Quick open chapter
- `Ctrl+F`: Find in chapter
- `Ctrl+Shift+F`: Find in all chapters

### AI
- `Ctrl+Space`: Trigger AI suggestion
- `Ctrl+Shift+A`: Open AI chat
- `Alt+Enter`: Accept suggestion

### View
- `F11`: Focus mode
- `Ctrl+B` (sidebar): Toggle sidebar
- `Ctrl+Shift+P`: Toggle AI panel
- `Ctrl+\`: Toggle dark/light theme

## Responsive States

### Loading
- Skeleton screens for content loading
- Spinner for AI generation
- Progress bar for exports

### Empty States
- "No novels yet" with create button
- "Start writing..." placeholder in editor
- Helpful tips for first-time users

### Error States
- Toast notifications for errors
- Inline validation for forms
- Retry buttons for failed operations

## Accessibility

- Full keyboard navigation
- Screen reader support
- High contrast mode
- Adjustable font sizes
- Focus indicators
- ARIA labels

## Typography

### Headings
- **Display**: Sora, 32px, Bold
- **H1**: Sora, 24px, SemiBold
- **H2**: Sora, 20px, Medium
- **H3**: Sora, 18px, Medium

### Body
- **Editor**: Georgia, 16px, Regular, 1.8 line-height
- **UI Text**: Inter, 14px, Regular
- **Captions**: Inter, 12px, Regular
- **Code**: Consolas, 14px, Regular

## Iconography

- Use **Lucide Icons** or **Material Design Icons**
- 16px for inline icons
- 20px for toolbar icons
- 24px for feature icons
- Consistent stroke width (2px)

## Animations

### Micro-interactions
- Button hover: scale(1.05) + brightness
- Panel slide: 200ms ease-out
- Tooltip: fade in 100ms
- Modal: fade + scale from center

### Loading
- Skeleton pulse: 1.5s ease-in-out infinite
- Spinner rotation: 1s linear infinite
- Progress bar: smooth transition

## Tooltips

- Delay: 500ms
- Position: Below element (above if no room)
- Dark background with white text
- Small arrow pointing to element
- Include keyboard shortcut if applicable

## Implementation Notes

### Technology Choices
**Option A: WinUI 3** (Recommended for Windows-only)
- Native Windows 11 look
- Fluent Design System
- XAML declarative UI
- Good performance

**Option B: Qt 6** (For future cross-platform)
- QML for modern UI
- Qt Widgets for classic approach
- Cross-platform ready
- Rich ecosystem

### Component Library
- Build custom components matching design
- Reusable button, input, dropdown, modal
- Consistent styling via theme system
- Dark/light mode switching

### State Management
- Centralized app state
- Reactive updates to UI
- Undo/redo stack
- Persistent preferences

## Next Steps

1. Create mockups in Figma/Sketch
2. Build component library
3. Implement main window shell
4. Add editor with basic functionality
5. Integrate AI panel
6. Polish interactions and animations
7. User testing and iteration
