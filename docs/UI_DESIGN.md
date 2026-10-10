# UI design notes

The workbench uses the information hierarchy familiar from VS Code and other writing IDEs:

- **Activity bar**: switch between files, chapters, characters, notes, and settings.
- **Explorer**: keeps the active project tree visible while the editor changes chapters.
- **Editor**: title, chapter status, chapter objective, line rail, plain text writing surface, word count, save state, and a version history dialog for checkpoints, side-by-side comparison with the current draft, and safe restores.
- **Agent panel**: chat, outline, and context tabs share the current chapter and project context. Chat modes cover writing, planning, review, and selected-text rewriting; rewrite replies can be edited in a preview dialog and applied to the editor selection after confirmation. Unread replies stay marked until the chat is visible at its latest position; uncertain request results offer a chat-history sync before retrying.
- **Status bar**: local single-user state, target progress, encoding, and stack.
- **Quick search and help**: a `Ctrl/⌘ + K` command palette searches current project content and runs common workspace actions; the help panel lists keyboard shortcuts.

The dark palette uses slate surfaces with a restrained lavender accent. The layout collapses the explorer into an overlay on smaller screens and turns the Agent panel into an overlay, so the writing surface remains usable on a laptop.
