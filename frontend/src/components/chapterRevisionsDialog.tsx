import { History, RotateCcw, Save, X } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import { formatNumber, formatTimestamp, statusClass, statusLabel } from '../lib/format'
import { countUnicodeCharacters, sliceUnicodeHead } from '../lib/text'
import type { Chapter, ChapterRevision, ChapterRevisionSummary } from '../types'

interface ChapterRevisionsDialogProps {
  revisionDialogOpen: boolean
  setRevisionDialogOpen: Dispatch<SetStateAction<boolean>>
  currentChapter: Chapter | undefined
  workspaceID: string
  chapterActionBusy: boolean
  revisionError: string
  revisionLabel: string
  setRevisionLabel: Dispatch<SetStateAction<string>>
  saveChapterRevision: () => Promise<void>
  revisionListLoading: boolean
  chapterRevisions: ChapterRevisionSummary[]
  selectedRevision: ChapterRevision | null
  onSelectRevision: (revision: ChapterRevisionSummary) => void
  revisionPreviewLoading: boolean
  revisionCompareOpen: boolean
  setRevisionCompareOpen: Dispatch<SetStateAction<boolean>>
  revisionCurrentSnapshot: Pick<Chapter, 'title' | 'summary' | 'content' | 'status' | 'word_count'> | null
  restoreChapterRevision: () => Promise<void>
  sending: boolean
}

export function ChapterRevisionsDialog({
  revisionDialogOpen,
  setRevisionDialogOpen,
  currentChapter,
  workspaceID,
  chapterActionBusy,
  revisionError,
  revisionLabel,
  setRevisionLabel,
  saveChapterRevision,
  revisionListLoading,
  chapterRevisions,
  selectedRevision,
  onSelectRevision,
  revisionPreviewLoading,
  revisionCompareOpen,
  setRevisionCompareOpen,
  revisionCurrentSnapshot,
  restoreChapterRevision,
  sending,
}: ChapterRevisionsDialogProps) {
  return (
    <>
      {revisionDialogOpen && currentChapter && workspaceID && (
        <div className="modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget && !chapterActionBusy) setRevisionDialogOpen(false) }}>
          <section className="project-dialog chapter-revisions-dialog" role="dialog" aria-modal="true" aria-labelledby="chapter-revisions-title" aria-describedby="chapter-revisions-description">
            <header><div><span className="dialog-kicker">CHAPTER HISTORY</span><h2 id="chapter-revisions-title">章节版本</h2></div><button className="subtle-icon" onClick={() => setRevisionDialogOpen(false)} aria-label="关闭章节版本" disabled={chapterActionBusy}><X size={16} /></button></header>
            <p className="revision-dialog-description" id="chapter-revisions-description">为当前章节保存检查点。恢复旧版本时，当前内容会自动保存为一个可恢复的版本。</p>
            {revisionError && <p className="dialog-error" role="alert">{revisionError}</p>}
            <div className="revision-save-row"><label>版本名称<input value={revisionLabel} onChange={(event) => setRevisionLabel(event.target.value)} maxLength={80} disabled={chapterActionBusy} /></label><button className="primary-button" onClick={() => void saveChapterRevision()} disabled={chapterActionBusy || sending}><Save size={14} /> 保存当前版本</button></div>
            <div className="revision-columns">
              <div className="revision-list" aria-label="版本列表">
                {revisionListLoading && <div className="revision-empty">正在读取版本…</div>}
                {!revisionListLoading && chapterRevisions.length === 0 && <div className="revision-empty">还没有保存的版本。保存后，可以比较和恢复章节正文、标题与目标。</div>}
                {!revisionListLoading && chapterRevisions.map((revision) => <button className={`revision-item ${selectedRevision?.id === revision.id ? 'selected' : ''}`} key={revision.id} onClick={() => void onSelectRevision(revision)} aria-pressed={selectedRevision?.id === revision.id} disabled={chapterActionBusy || revisionPreviewLoading && selectedRevision?.id === revision.id}>
                  <span className="revision-item-top"><strong>{revision.label}</strong><small>{formatTimestamp(revision.created_at)}</small></span><span className="revision-item-title">{revision.title || '未命名章节'}</span><span className="revision-item-meta">{statusLabel(revision.status)} · {formatNumber(revision.word_count)} 字</span>{revision.summary && <span className="revision-item-summary">{revision.summary}</span>}
                </button>)}
              </div>
              <div className="revision-preview">
                {revisionPreviewLoading && <div className="revision-empty">正在读取版本正文…</div>}
                {!revisionPreviewLoading && !selectedRevision && <div className="revision-empty">选择左侧版本查看内容。</div>}
                {!revisionPreviewLoading && selectedRevision && revisionCompareOpen && revisionCurrentSnapshot && <>
                  <div className="revision-compare-toolbar">
                    <span>对照保存版本与打开此窗口时的当前稿</span>
                    <button className="toolbar-button" onClick={() => setRevisionCompareOpen(false)}>返回版本预览</button>
                  </div>
                  <div className="revision-compare-columns">
                    <article className="revision-compare-pane">
                      <div className="revision-compare-heading"><span>保存版本</span><strong>{selectedRevision.title || '未命名章节'}</strong><small>{formatTimestamp(selectedRevision.created_at)} · {formatNumber(selectedRevision.word_count)} 字 · {statusLabel(selectedRevision.status)}</small>{selectedRevision.summary && <p>{selectedRevision.summary}</p>}</div>
                      <pre>{sliceUnicodeHead(selectedRevision.content, 100000) || '（此版本正文为空）'}</pre>
                      {countUnicodeCharacters(selectedRevision.content) > 100000 && <small className="revision-truncated">对照仅显示前 100,000 个字符。</small>}
                    </article>
                    <article className="revision-compare-pane">
                      <div className="revision-compare-heading"><span>当前稿</span><strong>{revisionCurrentSnapshot.title}</strong><small>{formatNumber(revisionCurrentSnapshot.word_count)} 字 · {statusLabel(revisionCurrentSnapshot.status)}</small>{revisionCurrentSnapshot.summary && <p>{revisionCurrentSnapshot.summary}</p>}</div>
                      <pre>{sliceUnicodeHead(revisionCurrentSnapshot.content ?? '', 100000) || '（当前稿正文为空）'}</pre>
                      {countUnicodeCharacters(revisionCurrentSnapshot.content ?? '') > 100000 && <small className="revision-truncated">对照仅显示前 100,000 个字符。</small>}
                    </article>
                  </div>
                  <button className="primary-button revision-restore-button" onClick={() => void restoreChapterRevision()} disabled={chapterActionBusy || sending}><RotateCcw size={14} /> 恢复此版本</button>
                </>}
                {!revisionPreviewLoading && selectedRevision && !revisionCompareOpen && <>
                  <div className="revision-preview-heading"><div><strong>{selectedRevision.label}</strong><small>{formatTimestamp(selectedRevision.created_at)} · {formatNumber(selectedRevision.word_count)} 字</small></div><span className={`chapter-status-text ${statusClass(selectedRevision.status)}`}>{statusLabel(selectedRevision.status)}</span></div>
                  <h3>{selectedRevision.title}</h3>
                  {selectedRevision.summary && <p className="revision-preview-summary">{selectedRevision.summary}</p>}
                  <pre>{sliceUnicodeHead(selectedRevision.content, 2400) || '（此版本正文为空）'}</pre>
                  {countUnicodeCharacters(selectedRevision.content) > 2400 && <small className="revision-truncated">预览已截取前 2,400 个字符。</small>}
                  <div className="revision-preview-actions"><button className="toolbar-button" onClick={() => setRevisionCompareOpen(true)} disabled={!revisionCurrentSnapshot}><History size={13} /> 与当前稿对照</button><button className="primary-button" onClick={() => void restoreChapterRevision()} disabled={chapterActionBusy || sending}><RotateCcw size={14} /> 恢复此版本</button></div>
                </>}
              </div>
            </div>
            <div className="dialog-actions"><button className="toolbar-button" onClick={() => setRevisionDialogOpen(false)} disabled={chapterActionBusy}>关闭</button></div>
          </section>
        </div>
      )}
    </>
  )
}
