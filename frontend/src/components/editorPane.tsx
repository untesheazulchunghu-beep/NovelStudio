import { ChevronRight, History, Maximize2, Minimize2, Save, Target, WandSparkles, Zap } from 'lucide-react'
import type { CSSProperties, ChangeEventHandler, KeyboardEventHandler, RefObject } from 'react'
import { EmptyEditor } from './editor'
import { formatNumber, relativeTime, statusClass } from '../lib/format'
import type { Chapter, Project } from '../types'

interface EditorPaneProps {
  currentChapter?: Chapter
  project?: Project
  projectActionBusy: boolean
  editorLocked: boolean
  settingsDirty: boolean
  sending: boolean
  chapterBodyLoading: boolean
  chapterLoadError: string
  onRetryChapterLoad: () => void
  onCreateChapter: () => void
  onSaveChapter: () => void | Promise<boolean>
  onOpenChapterRevisions: () => void | Promise<void>
  onApplyGeneratedText: () => void | Promise<void>
  focusMode: boolean
  onToggleFocusMode: () => void
  editorTitle: string
  onTitleChange: (value: string) => void
  editorStatus: Chapter['status']
  onStatusChange: (value: string) => void
  editorWordCount: number
  editorSummary: string
  onSummaryChange: (value: string) => void
  editorStyle: CSSProperties
  lineCount: number
  editorRef: RefObject<HTMLTextAreaElement | null>
  editorContent: string
  onContentChange: ChangeEventHandler<HTMLTextAreaElement>
  onSelection: ChangeEventHandler<HTMLTextAreaElement>
  onEditorKeyDown: KeyboardEventHandler<HTMLTextAreaElement>
}

export function EditorPane({
  currentChapter,
  project,
  projectActionBusy,
  editorLocked,
  settingsDirty,
  sending,
  chapterBodyLoading,
  chapterLoadError,
  onRetryChapterLoad,
  onCreateChapter,
  onSaveChapter,
  onOpenChapterRevisions,
  onApplyGeneratedText,
  focusMode,
  onToggleFocusMode,
  editorTitle,
  onTitleChange,
  editorStatus,
  onStatusChange,
  editorWordCount,
  editorSummary,
  onSummaryChange,
  editorStyle,
  lineCount,
  editorRef,
  editorContent,
  onContentChange,
  onSelection,
  onEditorKeyDown,
}: EditorPaneProps) {
  return (
    <section className="editor-pane">
      {currentChapter ? (
        <>
          <div className="editor-toolbar">
            <div className="breadcrumbs"><span className="crumb-muted">{project?.title}</span><ChevronRight size={13} /><span>章节</span><ChevronRight size={13} /><span className="crumb-current">第 {currentChapter.number} 章</span></div>
            <div className="editor-actions"><button className="toolbar-button" onClick={() => void onSaveChapter()} disabled={editorLocked}><Save size={14} /> 保存</button><button className="toolbar-button" onClick={() => void onOpenChapterRevisions()} disabled={editorLocked} title="查看和恢复章节版本"><History size={14} /> 版本</button><button className="toolbar-button primary-soft" onClick={() => void onApplyGeneratedText()} disabled={editorLocked || projectActionBusy || settingsDirty}><WandSparkles size={14} /> {sending ? '生成中' : 'AI 续写'}</button><button className="toolbar-button focus-toggle-button" onClick={onToggleFocusMode} aria-pressed={focusMode} title={focusMode ? '退出专注写作' : '进入专注写作'}>{focusMode ? <Minimize2 size={14} /> : <Maximize2 size={14} />} <span>{focusMode ? '退出专注' : '专注写作'}</span></button></div>
          </div>
          <div className="editor-scroll">
            {chapterBodyLoading && !chapterLoadError && <div className="revision-empty" role="status">正在读取章节正文…</div>}
            {chapterLoadError && <div className="revision-empty" role="alert">{chapterLoadError} <button className="toolbar-button" onClick={onRetryChapterLoad}>重试</button></div>}
            <div className="chapter-heading">
              <input className="chapter-title-input" value={editorTitle} onChange={(event) => onTitleChange(event.target.value)} maxLength={120} disabled={editorLocked} aria-label="章节标题" />
              <div className="chapter-meta"><label className={`chapter-status ${statusClass(editorStatus)}`}><span /><select value={editorStatus} onChange={(event) => onStatusChange(event.target.value)} disabled={editorLocked} aria-label="章节状态"><option value="outline">大纲</option><option value="draft">草稿</option><option value="review">审阅中</option><option value="done">已完成</option></select></label><span className="meta-separator" /><span>{formatNumber(editorWordCount)} 字</span><span className="meta-separator" /><span>最后编辑于 {relativeTime(currentChapter.last_edited_at)}</span></div>
            </div>
            <div className="summary-box"><div className="summary-label"><Target size={13} /> 本章目标</div><textarea value={editorSummary} onChange={(event) => onSummaryChange(event.target.value)} maxLength={1200} disabled={editorLocked} placeholder="写下这一章要改变什么，Agent 会将它作为写作锚点。" rows={2} /></div>
            <div className="editor-area" style={editorStyle}>
              <div className="line-numbers" aria-hidden="true">{Array.from({ length: lineCount }, (_, index) => <span key={index}>{index + 1}</span>)}</div>
              <textarea ref={editorRef} className="chapter-editor" value={editorContent} onChange={onContentChange} onSelect={onSelection} onKeyDown={onEditorKeyDown} disabled={editorLocked} spellCheck={false} placeholder={chapterBodyLoading ? '正在读取章节正文…' : '从这里开始写下你的故事……'} aria-label="章节正文" />
            </div>
          </div>
          <div className="editor-footer"><span><Zap size={12} /> 编辑器已就绪</span><span>Markdown 支持</span><span>⌘ S 保存</span></div>
        </>
      ) : <EmptyEditor onCreate={onCreateChapter} disabled={!project || projectActionBusy} />}
    </section>
  )
}
