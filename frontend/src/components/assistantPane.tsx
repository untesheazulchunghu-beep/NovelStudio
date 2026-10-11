import type { AssistantTab, ChatApplyTarget, ChatRequestRetry, WritingMode } from '../workbenchTypes'
import { ChatPanel, ContextPanel, OutlinePanel } from './assistant'
import { BrainCircuit, Bot, ListTodo, MessageSquare, PanelRightClose, X } from 'lucide-react'
import type { RefObject, KeyboardEvent } from 'react'
import type { Chapter, Character, ChatMessage, Note, Project, WorldAsset } from '../types'
import type { TimelineEvent } from '../types'
import type { ContextKind } from '../hooks/useAgentContextSelection'

interface AssistantPaneProps {
  assistantTab: AssistantTab
  assistantProviderLabel: string
  sending: boolean
  cancellingRequest: boolean
  chatUnread: boolean
  onCancel: () => void
  onCollapse: () => void
  onActivateTab: (tab: AssistantTab) => void
  onTabKeyDown: (event: KeyboardEvent<HTMLButtonElement>, currentTab: AssistantTab) => void
  chat: ChatMessage[]
  chatInput: string
  onChatInputChange: (value: string) => void
  onSendMessage: (preset?: string) => void
  chatRetry?: ChatRequestRetry
  onCheckChatRecovery: () => void
  checkingChatRecovery: boolean
  onRetryAfterReview: () => void
  onJumpToLatest: () => void
  onChatScroll: () => void
  chatScrollRef: RefObject<HTMLDivElement | null>
  chatEndRef: RefObject<HTMLDivElement | null>
  sendLocked: boolean
  chapterBodyLoading: boolean
  settingsDirty: boolean
  currentChapter?: Chapter
  suggestions: string[]
  writingMode: WritingMode
  onWritingModeChange: (mode: WritingMode) => void
  selectionCharacterCount: number
  selectionHasText: boolean
  onClearSelection: () => void
  chatApplyTarget: ChatApplyTarget | null
  onPreviewRewrite: () => void
  chapters: Chapter[]
  activeChapterId: number | null
  onSelectChapter: (id: number) => void
  onCreateChapter: () => void
  projectActionBusy: boolean
  characters: Character[]
  notes: Note[]
  timeline: TimelineEvent[]
  worldAssets: WorldAsset[]
  selectedContextSets: { character_ids: Set<number>; note_ids: Set<number>; timeline_event_ids: Set<number>; world_asset_ids: Set<number> }
  onToggleContextItem: (kind: ContextKind, id: number) => void
  onSetAllContextItems: (kind: ContextKind, enabled: boolean) => void
  project?: Project
  onEditProject: () => void
  onEditCharacter: (character: Character) => void
  onEditNote: (note: Note) => void
}

export function AssistantPane({
  assistantTab,
  assistantProviderLabel,
  sending,
  cancellingRequest,
  chatUnread,
  onCancel,
  onCollapse,
  onActivateTab,
  onTabKeyDown,
  chat,
  chatInput,
  onChatInputChange,
  onSendMessage,
  chatRetry,
  onCheckChatRecovery,
  checkingChatRecovery,
  onRetryAfterReview,
  onJumpToLatest,
  onChatScroll,
  chatScrollRef,
  chatEndRef,
  sendLocked,
  chapterBodyLoading,
  settingsDirty,
  currentChapter,
  suggestions,
  writingMode,
  onWritingModeChange,
  selectionCharacterCount,
  selectionHasText,
  onClearSelection,
  chatApplyTarget,
  onPreviewRewrite,
  chapters,
  activeChapterId,
  onSelectChapter,
  onCreateChapter,
  projectActionBusy,
  characters,
  notes,
  timeline,
  worldAssets,
  selectedContextSets,
  onToggleContextItem,
  onSetAllContextItems,
  project,
  onEditProject,
  onEditCharacter,
  onEditNote,
}: AssistantPaneProps) {
  return (
    <aside className="assistant-pane">
      <div className="assistant-header"><div className="assistant-title"><div className="agent-avatar"><Bot size={16} /></div><div><strong>写作 Agent</strong><span role="status" aria-live="polite" aria-atomic="true"><span className={`status-dot ${sending ? 'amber' : 'green'}`} /> {cancellingRequest ? '正在确认取消…' : sending ? '正在请求模型…' : assistantProviderLabel}</span></div></div>{sending && <button className="subtle-icon" onClick={onCancel} title={cancellingRequest ? '正在确认取消' : '取消当前请求'} aria-label={cancellingRequest ? '正在确认取消 Agent 请求' : '取消当前 Agent 请求'} disabled={cancellingRequest}><X size={14} /></button>}<button className="subtle-icon" onClick={onCollapse} title="收起 Agent" aria-label="收起 Agent 面板"><PanelRightClose size={15} /></button></div>
      <div className="assistant-tabs" role="tablist" aria-label="写作 Agent 面板">
        <button id="assistant-tab-chat" role="tab" aria-label={chatUnread ? '对话（有未读回复）' : '对话'} aria-selected={assistantTab === 'chat'} aria-controls="assistant-panel-chat" tabIndex={assistantTab === 'chat' ? 0 : -1} className={assistantTab === 'chat' ? 'active' : ''} onClick={() => onActivateTab('chat')} onKeyDown={(event) => onTabKeyDown(event, 'chat')}><MessageSquare size={13} /> 对话{chatUnread && <span className="assistant-unread-dot" aria-hidden="true" />}</button>
        <button id="assistant-tab-outline" role="tab" aria-selected={assistantTab === 'outline'} aria-controls="assistant-panel-outline" tabIndex={assistantTab === 'outline' ? 0 : -1} className={assistantTab === 'outline' ? 'active' : ''} onClick={() => onActivateTab('outline')} onKeyDown={(event) => onTabKeyDown(event, 'outline')}><ListTodo size={13} /> 大纲</button>
        <button id="assistant-tab-context" role="tab" aria-selected={assistantTab === 'context'} aria-controls="assistant-panel-context" tabIndex={assistantTab === 'context' ? 0 : -1} className={assistantTab === 'context' ? 'active' : ''} onClick={() => onActivateTab('context')} onKeyDown={(event) => onTabKeyDown(event, 'context')}><BrainCircuit size={13} /> 上下文</button>
      </div>
      <ChatPanel chat={chat} input={chatInput} setInput={onChatInputChange} send={onSendMessage} cancel={onCancel} retryPrompt={chatRetry?.safeToRetry && !chatRetry.needsReconcile ? chatRetry.message : undefined} retryNeedsReconcile={Boolean(chatRetry?.needsReconcile)} retryNeedsReview={Boolean(chatRetry?.requiresReview && !chatRetry.needsReconcile)} checkChatRecovery={onCheckChatRecovery} checkingChatRecovery={checkingChatRecovery} retryAfterReview={onRetryAfterReview} sending={sending} cancelling={cancellingRequest} hasUnread={chatUnread} onJumpToLatest={onJumpToLatest} onChatScroll={onChatScroll} chatScrollRef={chatScrollRef} locked={sendLocked || chapterBodyLoading} settingsUnsaved={settingsDirty} hasChapter={Boolean(currentChapter)} suggestions={suggestions} mode={writingMode} setMode={onWritingModeChange} chatEndRef={chatEndRef} hidden={assistantTab !== 'chat'} selectionCharacterCount={selectionCharacterCount} selectionHasText={selectionHasText} clearSelection={onClearSelection} applyTarget={chatApplyTarget} onPreviewRewrite={() => onPreviewRewrite()} />
      <OutlinePanel chapters={chapters} activeChapterId={activeChapterId} setActiveChapterId={onSelectChapter} onCreate={onCreateChapter} busy={projectActionBusy} hidden={assistantTab !== 'outline'} />
      <ContextPanel characters={characters} notes={notes} timeline={timeline} worldAssets={worldAssets} selectedContextSets={selectedContextSets} onToggleContextItem={onToggleContextItem} onSetAllContextItems={onSetAllContextItems} project={project} projectBusy={projectActionBusy} onEditProject={onEditProject} onEditCharacter={onEditCharacter} onEditNote={onEditNote} hidden={assistantTab !== 'context'} />
    </aside>
  )
}
