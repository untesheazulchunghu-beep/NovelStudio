# NovelStudio architecture

## Runtime

```text
React/Vite (5173 in development)
        │ fetch /api/*
        ▼
Go net/http server (8080 by default; Vite proxy target is configurable)
        │
        ├── Store: in-memory workspace cache + SQLite transactions
        ├── Agent: local rule engine or OpenAI-compatible adapter
        └── SQLite: data/novelstudio.db
```

The local app supports multiple novel projects in one SQLite database. Each project has a workspace ID, and every workspace API request selects it through `X-Workspace-ID`; requests without the header use the seeded `default` workspace. Content tables carry the workspace ID, while the workspace row also has an `owner_id` field for future account ownership. The project list, creation, metadata update, and deletion endpoints are global to the current local user. Project deletion cascades to its content and refuses to remove the last project.

## Code layout

Backend (`internal/app`, one Go package, split by responsibility):

- `server*.go`: routing and HTTP handlers by resource (projects, chapters, characters and notes, chat), request locks, and JSON helpers in `server_http.go`.
- `chat_prompt.go`, `local_reply.go`, `model_client.go`: prompt construction, the offline rule engine, and the OpenAI-compatible model client.
- `store*.go`: the in-memory workspace cache and its transactional operations (projects, chapters, characters and notes, chat requests, search, validation, normalization).
- `sqlite*.go`: opening and protecting the database, schema migrations, workspace persistence, chat request records, and incremental deltas in `sqlite_delta.go`.
- `export_*.go`, `backup.go`, `chapter_revisions.go`, `models.go`: exporters, backups, checkpoints, and shared types.

Frontend (`frontend/src`):

- `App.tsx`: workbench state and event orchestration.
- `components/`: sidebar, assistant panel, editor placeholder, command palette, and chapter revision dialog.
- `lib/`: pure helpers for text, search, formatting, and agent modes; `*Actions.ts` modules own chat, chapter, project, context-item, and editor workflows.
- `styles.css` imports the ordered CSS sections under `styles/` for the workbench, editor, assistant, panels, dialogs, and responsive layouts.
- `api.ts`, `types.ts` (API shapes), and `workbenchTypes.ts` (UI-local types).

## Agent context

`POST /api/chat` receives the selected writing mode, chapter ID, the current editor title and goal, the current editor tail, an optional selected passage (limited to 1,800 Unicode characters), and the user message. The server adds project references and recent chat turns before calling the configured provider. Edit mode asks for replacement text only. Replies are saved with the conversation, and request results are stored separately from the 60-message history window. A client supplied `request_id` makes retries with the same request fingerprint return the saved user and assistant messages without generating another reply, even after the conversation history is trimmed; results remain until project deletion. Servers in the same process that open the same SQLite file share an in-memory request lock. Separate processes coordinate through a two-minute SQLite lease; waiters reuse completed results, and a crashed owner can be replaced after the lease expires. The transaction fences an expired owner from committing after takeover. A model call that exceeds the lease can still be duplicated, but the unique request result remains single. Reusing an ID for known different content returns HTTP 409 with `chat_request_conflict`; an older matching user message with no stored fingerprint returns `chat_request_unverifiable` so the UI does not assume that the new request was saved. The UI offers an explicit apply action that replaces the original range only while its source text is unchanged.

`POST /api/chapters/:id/generate` receives the current editor title, goal, body, and a writing instruction. Unsaved editor values take precedence over the last saved chapter snapshot; older clients that omit these fields continue to use saved chapter metadata and body. The server combines them with project references and asks the provider to return only text that can be appended to the chapter.

`POST /api/workspace/export/docx` and `POST /api/workspace/export/epub` create WordprocessingML and EPUB 3 packages from a workspace snapshot. The request can replace the current chapter's title and body with unsaved editor values; all other chapters come from SQLite and are written in chapter order. EPUB includes a title page, navigation document, stylesheet, and one XHTML file per chapter. XML-invalid characters return a clear client error rather than producing a damaged document.

`GET /api/workspace` returns chapter metadata without chapter bodies, and the Go workspace cache keeps only that metadata. The editor loads a body through `GET /api/chapters/:id` when that chapter becomes active, drops the previous chapter body from its client cache after navigation, and cancels obsolete requests during rapid navigation. `GET /api/chapters/search?q=...` searches saved chapter titles, summaries, and bodies on the server and returns matching excerpts; the client also checks the active unsaved draft. SQLite FTS5 with the trigram tokenizer indexes chapter text without storing another copy of the source bodies. Terms of three or more characters use the index; short terms, punctuation, and numeric queries retain the direct scan path. `POST /api/workspace/export/markdown` builds the complete Markdown file on the server and accepts an override for the active chapter's unsaved title and body. Backups and the DOCX/EPUB exporters load full snapshots from SQLite only while creating an export.

Chapter checkpoints live in `chapter_revisions`, separate from the active chapter row so normal autosaves do not create noisy history. Authors can save a named checkpoint, inspect its metadata, and load its body for preview. Restoring a checkpoint and saving the current chapter as a recovery point happen in one SQLite transaction. Each chapter keeps its latest 20 checkpoints; deleting a chapter cascades to its history. Project backups include these checkpoints and imports remap their chapter IDs.

The default provider is `local-creative`. It gives deterministic, Chinese-language writing guidance for offline development. `openai` and `compatible` use the standard `/v1/chat/completions` contract when a valid base URL, model name, and key are configured. Incomplete remote settings return a configuration error instead of silently falling back to local replies.

## Persistence

Writes are serialized by the store mutex and committed as a SQLite transaction. After initialization, the store writes only child records that changed; chapter reorders stage temporary unique numbers before applying their final order. Model API keys stay in the local database and are never returned by settings or workspace reads; those responses include only an `api_key_configured` flag for the key. Omitting `api_key` on settings updates preserves the saved value, while sending an empty string clears it. Project backups use a versioned JSON format and restore as a new project with fresh record IDs. API keys are omitted, and credentials in the model endpoint URL's user information, query parameters, and fragment are removed on export and import. The first launch seeds a small working novel. The app attempts to restrict permissions on the prior `data/workspace.json` on every startup. If the database has no workspace, it imports the file while retaining its JSON contents as a backup. SQLite database and sidecar files are also set to owner-only permissions where supported. The default HTTP listener is loopback-only. Browser requests are accepted from the same loopback origin or the local Vite origins (`localhost:5173` and `127.0.0.1:5173`); requests from other origins are rejected. API responses are marked `no-store`, and the server prevents framing and MIME sniffing. Requests without an `Origin` header remain available to local command-line clients.

Settings and workspace responses return a sanitized model URL and a `base_url_credentials_configured` flag. Settings updates retain hidden URL credentials unless the user explicitly clears them. Project, character, note, chapter, and revision-label write APIs enforce the form's text lengths and note categories; backup restores retain existing field values. Metadata and ordering requests are capped at 256 KiB, chapter edits at 16 MiB, Agent requests at 1 MiB, project backup imports at 256 MiB, and legacy JSON migration at 256 MiB. Oversized JSON request bodies return HTTP 413. The HTTP server applies a Content Security Policy that limits scripts and connections to the app's own origin and disables embedded objects. Model API redirects are followed only on the same host, and HTTPS requests are never downgraded to HTTP. Plain HTTP model endpoints are accepted only for localhost or loopback IP addresses; network endpoints require HTTPS.
Legacy note categories imported from backups can be kept when editing a note; changing the category requires one of the supported values.
Word counts treat each Han ideograph as one unit and group contiguous letters, combining marks, numbers, and underscores from other scripts into one word.
