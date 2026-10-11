import { ApiError, api } from '../api'
import { maxAgentInputCharacters, maxAgentSelectionCharacters } from '../lib/constants'
import { suggestionsForMode } from '../lib/agent'
import {
  contextAroundSelection,
  countUnicodeCharacters,
  countUnicodeCharactersInRange,
  normalizeEditorLineEndings,
  normalizeUnicodeSelection,
  sliceUnicodeRange,
  sliceUnicodeTail,
} from '../lib/text'
import type { Chapter, ChatContextItems, ChatMessage, WorkspaceResponse } from '../types'
import type { AssistantTab, ChatApplyTarget, ChatRequestRetry, EditorSelection, WritingMode } from '../workbenchTypes'
import type { Dispatch, SetStateAction } from 'react'

type MutableRef<T> = { current: T }

interface AgentActionsOptions {
  data: WorkspaceResponse | null
  contextItems: ChatContextItems
  setData: Dispatch<SetStateAction<WorkspaceResponse | null>>
  projectActionBusy: boolean
  chapterBodyLoading: boolean
  settingsDirty: boolean
  chatInput: string
  setChatInput: Dispatch<SetStateAction<string>>
  updateChatInput: (value: string) => void
  writingModeRef: MutableRef<WritingMode>
  editorContentRef: MutableRef<string>
  currentChapterRef: MutableRef<Chapter | undefined>
  currentChapter: Chapter | undefined
  editorTitle: string
  editorSummary: string
  activeEditorSelection: EditorSelection | null
  activeWorkspaceRef: MutableRef<string>
  chatAutoScrollRef: MutableRef<boolean>
  chatAutoScrollByWorkspaceRef: MutableRef<Map<string, boolean>>
  chatInputRevisionRef: MutableRef<number>
  chatDraftsRef: MutableRef<Map<string, string>>
  chatRequestRetryRef: MutableRef<Map<string, ChatRequestRetry>>
  agentRequestRef: MutableRef<AbortController | null>
  saveErrorMessageRef: MutableRef<string>
  editorRef: MutableRef<HTMLTextAreaElement | null>
  lastOutsideModalFocusRef: MutableRef<HTMLElement | null>
  setEditorSelection: Dispatch<SetStateAction<EditorSelection | null>>
  setChatDraftRevision: Dispatch<SetStateAction<number>>
  setSending: Dispatch<SetStateAction<boolean>>
  setCancellingRequest: Dispatch<SetStateAction<boolean>>
  setCheckingChatRecovery: Dispatch<SetStateAction<boolean>>
  checkingChatRecovery: boolean
  sending: boolean
  setError: Dispatch<SetStateAction<string>>
  setChatApplyTarget: Dispatch<SetStateAction<ChatApplyTarget | null>>
  chatApplyTarget: ChatApplyTarget | null
  rewritePreviewOpen: boolean
  setRewritePreviewOpen: Dispatch<SetStateAction<boolean>>
  setSuggestions: Dispatch<SetStateAction<string[]>>
  setWorkspaceChatUnread: (workspaceID: string | undefined, unread: boolean) => void
  chatIsVisibleAtLatest: (workspaceID?: string) => boolean
  confirmDiscardEditedRewrite: (workspaceID?: string, chapterID?: number) => boolean
  clearChatApplyTarget: () => void
  updateEditor: (next: string) => void
  activateAssistantTab: (tab: AssistantTab) => void
}

export function createAgentActions(options: AgentActionsOptions) {
  const {
    data,
    contextItems,
    setData,
    projectActionBusy,
    chapterBodyLoading,
    settingsDirty,
    chatInput,
    setChatInput,
    updateChatInput,
    writingModeRef,
    editorContentRef,
    currentChapterRef,
    currentChapter,
    editorTitle,
    editorSummary,
    activeEditorSelection,
    activeWorkspaceRef,
    chatAutoScrollRef,
    chatAutoScrollByWorkspaceRef,
    chatInputRevisionRef,
    chatDraftsRef,
    chatRequestRetryRef,
    agentRequestRef,
    saveErrorMessageRef,
    editorRef,
    lastOutsideModalFocusRef,
    setEditorSelection,
    setChatDraftRevision,
    setSending,
    setCancellingRequest,
    setCheckingChatRecovery,
    checkingChatRecovery,
    sending,
    setError,
    setChatApplyTarget,
    chatApplyTarget,
    rewritePreviewOpen,
    setRewritePreviewOpen,
    setSuggestions,
    setWorkspaceChatUnread,
    chatIsVisibleAtLatest,
    confirmDiscardEditedRewrite,
    clearChatApplyTarget,
    updateEditor,
    activateAssistantTab,
  } = options

  const sendMessage = async (preset?: string, rewriteDiscardApproved = false) => {
    const submittedInput = preset === undefined ? chatInput : null
    const message = (preset ?? chatInput).trim()
    const inputAtSend = chatInput
    const workspaceId = data?.workspace.workspace_id
    if (!message || projectActionBusy || chapterBodyLoading || !workspaceId || agentRequestRef.current) return
    if (settingsDirty) {
      setError('请先保存模型设置，再发送 Agent 请求')
      return
    }
    if (countUnicodeCharacters(message) > maxAgentInputCharacters) {
      setError(`Agent 输入不能超过 ${maxAgentInputCharacters} 个字符`)
      return
    }
    const requestMode = writingModeRef.current
    const editorContentAtSend = editorContentRef.current
    const selectionRange = activeEditorSelection
      ? { ...activeEditorSelection, ...normalizeUnicodeSelection(editorContentAtSend, activeEditorSelection.start, activeEditorSelection.end) }
      : null
    if (selectionRange && (selectionRange.start !== activeEditorSelection?.start || selectionRange.end !== activeEditorSelection?.end)) {
      setEditorSelection(selectionRange)
    }
    const selectionRangeIsValid = selectionRange !== null && selectionRange.start >= 0 && selectionRange.start < selectionRange.end && selectionRange.end <= editorContentAtSend.length
    const selectedCharacterCount = selectionRangeIsValid && selectionRange ? countUnicodeCharactersInRange(editorContentAtSend, selectionRange.start, selectionRange.end, maxAgentSelectionCharacters) : 0
    if (requestMode === 'edit' && selectedCharacterCount === 0) {
      setError('请先在编辑器正文中选中需要改写的内容')
      return
    }
    if (requestMode === 'edit' && selectedCharacterCount > maxAgentSelectionCharacters) {
      setError(`改写选区不能超过 ${maxAgentSelectionCharacters} 个字符，请缩小选区后重试`)
      return
    }
    const selectedText = selectionRangeIsValid && selectionRange ? sliceUnicodeRange(editorContentAtSend, selectionRange.start, selectionRange.end, maxAgentSelectionCharacters) : ''
    if (requestMode === 'edit' && selectedText.trim() === '') {
      setError('改写选区不能只包含空白字符，请选中正文内容')
      return
    }
    const selectionForRequest = selectedText
    const canApplySelection = requestMode === 'edit' && selectionRange !== null && selectionRangeIsValid && selectedText.trim() !== '' && selectedCharacterCount <= maxAgentSelectionCharacters && (data?.workspace.settings.provider === 'openai' || data?.workspace.settings.provider === 'compatible')
    const inputRevision = chatInputRevisionRef.current
    const chapterIDForRequest = currentChapter?.id ?? 0
    const contextForRequest = currentChapter
      ? selectionRangeIsValid && selectionRange
        ? contextAroundSelection(editorContentAtSend, selectionRange.start, selectionRange.end)
        : sliceUnicodeTail(editorContentAtSend, 1800)
      : ''
    const chapterTitleForRequest = currentChapter ? editorTitle.trim() || currentChapter.title : ''
    const chapterSummaryForRequest = currentChapter ? editorSummary : ''
    const contextItemsSignature = {
      character_ids: [...contextItems.character_ids].sort((left, right) => left - right),
      note_ids: [...contextItems.note_ids].sort((left, right) => left - right),
      timeline_event_ids: [...contextItems.timeline_event_ids].sort((left, right) => left - right),
      world_asset_ids: [...contextItems.world_asset_ids].sort((left, right) => left - right),
    }
    const requestContextSignature = JSON.stringify([
      [data?.workspace.project.title, data?.workspace.project.genre, data?.workspace.project.premise],
      data?.workspace.characters.slice(0, 16).map((character) => [character.name, character.role, character.description, character.traits.slice(0, 20)]),
      data?.workspace.notes.slice(0, 20).map((note) => [note.title, note.kind, note.body]),
      data?.workspace.timeline.slice(0, 24).map((event) => [event.id, event.title, event.time_label, event.description, event.status, event.chapter_id]),
      contextItemsSignature,
      [data?.workspace.settings.provider, data?.workspace.settings.model, data?.workspace.settings.base_url, data?.workspace.settings.temperature],
      data?.workspace.chat.filter((item) => item.role !== 'system').slice(-8).map((item) => [item.role, item.content]),
      currentChapter?.number,
    ])
    const requestSignature = JSON.stringify([message, requestMode, chapterIDForRequest, contextForRequest, selectionForRequest, chapterTitleForRequest, chapterSummaryForRequest, requestContextSignature])
    const pendingRequest = chatRequestRetryRef.current.get(workspaceId)
    if (pendingRequest?.needsReconcile) {
      setError('请先检查最新聊天记录，确认上次请求的结果后再发送。')
      return
    }
    if (pendingRequest?.requiresReview) {
      setError('请先检查聊天记录，再使用下方的“已检查，生成新请求 ID 重发”操作。')
      return
    }
    if (pendingRequest && pendingRequest.signature !== requestSignature) chatRequestRetryRef.current.delete(workspaceId)
    const requestID = pendingRequest?.signature === requestSignature ? pendingRequest.requestID : crypto.randomUUID()
    if (!rewriteDiscardApproved && !confirmDiscardEditedRewrite(workspaceId)) return
    setError((current) => current === saveErrorMessageRef.current ? current : '')
    clearChatApplyTarget()
    const optimisticMessageID = -Date.now()
    const streamingMessageID = optimisticMessageID - 1
    const controller = new AbortController()
    agentRequestRef.current = controller
    chatAutoScrollRef.current = true
    chatAutoScrollByWorkspaceRef.current.set(workspaceId, true)
    setWorkspaceChatUnread(workspaceId, false)
    if (submittedInput !== null) setChatInput('')
    const optimisticMessage: ChatMessage = {
      id: optimisticMessageID,
      role: 'user',
      content: message,
      created_at: new Date().toISOString(),
      mode: requestMode,
      request_id: requestID,
    }
    setData((current) => current && current.workspace.workspace_id === workspaceId
      ? { ...current, workspace: { ...current.workspace, chat: [...current.workspace.chat, optimisticMessage].slice(-60) } }
      : current)
    setSending(true)
    try {
      let streamedContent = ''
      const response = await api.chatStream(
        workspaceId,
        requestID,
        message,
        requestMode,
        chapterIDForRequest,
        contextForRequest,
        selectionForRequest,
        chapterTitleForRequest,
        chapterSummaryForRequest,
        contextItems,
        {
          signal: controller.signal,
          onDelta: (text) => {
            streamedContent += text
            setData((current) => {
              if (!current || current.workspace.workspace_id !== workspaceId) return current
              const assistant: ChatMessage = {
                id: streamingMessageID,
                role: 'assistant',
                content: streamedContent,
                created_at: new Date().toISOString(),
                mode: requestMode,
              }
              const withoutStream = current.workspace.chat.filter((item) => item.id !== streamingMessageID)
              return { ...current, workspace: { ...current.workspace, chat: [...withoutStream, assistant].slice(-60) } }
            })
          },
        },
      )
      setError((current) => current === saveErrorMessageRef.current ? current : '')
      if (chatRequestRetryRef.current.get(workspaceId)?.requestID === requestID) chatRequestRetryRef.current.delete(workspaceId)
      setWorkspaceChatUnread(workspaceId, !chatIsVisibleAtLatest(workspaceId))
      if (canApplySelection && selectionRange && response.message.id !== undefined) {
        const latestContent = editorContentRef.current
        const selectionStillCurrent = activeWorkspaceRef.current === workspaceId &&
          currentChapterRef.current?.id === selectionRange.chapterId &&
          latestContent.slice(selectionRange.start, selectionRange.end) === selectedText &&
          contextAroundSelection(latestContent, selectionRange.start, selectionRange.end) === contextForRequest
        if (selectionStillCurrent) {
          setChatApplyTarget({
            workspaceId,
            chapterId: selectionRange.chapterId,
            start: selectionRange.start,
            end: selectionRange.end,
            messageId: response.message.id,
            original: selectedText,
            replacement: normalizeEditorLineEndings(response.message.content),
            generatedReplacement: normalizeEditorLineEndings(response.message.content),
            context: contextForRequest,
          })
        } else {
          setError('生成期间选区内容或邻近正文已变化，本次回复不能直接替换。')
        }
      }
      setData((current) => {
        if (!current || current.workspace.workspace_id !== workspaceId) return current
        return {
          ...current,
          workspace: {
            ...current.workspace,
            chat: (() => {
              const currentChat = current.workspace.chat.filter((item) => item.id !== optimisticMessageID && item.id !== streamingMessageID)
              const responseUserMessage = response.user_message ?? optimisticMessage
              if (typeof responseUserMessage.id !== 'number' || typeof response.message.id !== 'number') {
                return [...currentChat, responseUserMessage, response.message].slice(-60)
              }
              const knownMessages = currentChat.filter((item): item is ChatMessage & { id: number } => typeof item.id === 'number')
              const latestMessageID = knownMessages.reduce((latest, item) => Math.max(latest, item.id), 0)
              const messagesByID = new Map<number, ChatMessage>()
              knownMessages.forEach((item) => messagesByID.set(item.id, item))
              messagesByID.set(responseUserMessage.id, responseUserMessage)
              messagesByID.set(response.message.id as number, response.message)
              const mergedChat = [
                ...currentChat.filter((item) => typeof item.id !== 'number'),
                ...[...messagesByID.values()].sort((left, right) => (left.id ?? 0) - (right.id ?? 0)),
              ]
              return responseUserMessage.id < latestMessageID ? mergedChat : mergedChat.slice(-60)
            })(),
          },
        }
      })
      if (submittedInput !== null && chatInputRevisionRef.current === inputRevision) {
        if (chatDraftsRef.current.delete(workspaceId)) setChatDraftRevision((revision) => revision + 1)
      }
      if (activeWorkspaceRef.current === workspaceId && writingModeRef.current === requestMode) {
        const nextSuggestions = Array.isArray(response.suggestions) && response.suggestions.length > 0
          ? response.suggestions.filter((suggestion): suggestion is string => typeof suggestion === 'string' && suggestion.trim().length > 0)
          : []
        setSuggestions(nextSuggestions.length > 0 ? nextSuggestions : suggestionsForMode(requestMode))
      }
    } catch (reason) {
      const requestIDConflict = reason instanceof ApiError && reason.status === 409
      const knownRequestIDConflict = requestIDConflict && reason instanceof ApiError && reason.code === 'chat_request_conflict'
      const requiresReview = requestIDConflict && !knownRequestIDConflict
      let reconciled = false
      let submittedMessageWasPersisted = false
      let assistantReplyWasPersisted = false
      try {
        const refreshed = await api.workspace(workspaceId, AbortSignal.timeout(5000))
        if (activeWorkspaceRef.current === workspaceId) {
          const persistedUserMessageIndex = requestIDConflict ? -1 : refreshed.workspace.chat.findIndex((chatMessage) =>
            chatMessage.role === 'user' && chatMessage.request_id === requestID,
          )
          submittedMessageWasPersisted = persistedUserMessageIndex >= 0
          assistantReplyWasPersisted = persistedUserMessageIndex >= 0 &&
            refreshed.workspace.chat[persistedUserMessageIndex + 1]?.role === 'assistant' &&
            refreshed.workspace.chat[persistedUserMessageIndex + 1]?.mode === requestMode
          if (assistantReplyWasPersisted && !chatIsVisibleAtLatest(workspaceId)) setWorkspaceChatUnread(workspaceId, true)
          setData((current) => current && current.workspace.workspace_id === workspaceId
            ? { ...current, workspace: { ...current.workspace, chat: refreshed.workspace.chat } }
            : current)
          reconciled = true
        }
      } catch {
        // Keep the optimistic rollback path when the server cannot be reached.
      }
      if (!reconciled) {
        setData((current) => current && current.workspace.workspace_id === workspaceId
          ? { ...current, workspace: { ...current.workspace, chat: current.workspace.chat.filter((item) => item.id !== optimisticMessageID && item.id !== streamingMessageID) } }
          : current)
      }
      if (submittedMessageWasPersisted && assistantReplyWasPersisted) {
        if (chatRequestRetryRef.current.get(workspaceId)?.requestID === requestID) chatRequestRetryRef.current.delete(workspaceId)
      } else {
        chatRequestRetryRef.current.set(workspaceId, {
          signature: requestSignature,
          requestID: knownRequestIDConflict ? crypto.randomUUID() : requestID,
          message,
          mode: requestMode,
          safeToRetry: knownRequestIDConflict || (!requestIDConflict && reconciled && !submittedMessageWasPersisted),
          requiresReview: requiresReview || (submittedMessageWasPersisted && !assistantReplyWasPersisted),
          requiresFreshRequestID: requiresReview,
          needsReconcile: !reconciled,
        })
      }
      if (submittedInput !== null && chatInputRevisionRef.current === inputRevision) {
        if (submittedMessageWasPersisted) {
          if (chatDraftsRef.current.delete(workspaceId)) setChatDraftRevision((revision) => revision + 1)
        } else {
          setChatInput(submittedInput)
        }
      } else if (submittedInput === null && !inputAtSend.trim() && chatInputRevisionRef.current === inputRevision && !submittedMessageWasPersisted) {
        updateChatInput(message)
      }
      if (!reconciled) {
        setError('暂时无法确认这条消息是否已保存。恢复连接后请先检查聊天记录，再决定是否重新发送。')
      } else if (requestIDConflict && !knownRequestIDConflict) {
        setError('这条请求与旧聊天记录无法安全匹配。聊天记录已更新，输入已保留；请先检查记录再决定是否重新发送。')
      } else if (!controller.signal.aborted) {
        setError(reason instanceof Error ? reason.message : 'Agent 暂时无法响应')
      }
    } finally {
      if (agentRequestRef.current === controller) agentRequestRef.current = null
      setCancellingRequest(false)
      setSending(false)
    }
  }

  const checkChatRecovery = async () => {
    const workspaceId = data?.workspace.workspace_id
    const retry = workspaceId ? chatRequestRetryRef.current.get(workspaceId) : undefined
    if (!workspaceId || !retry?.needsReconcile || checkingChatRecovery || sending) return
    setCheckingChatRecovery(true)
    try {
      const refreshed = await api.workspace(workspaceId, AbortSignal.timeout(5000))
      if (activeWorkspaceRef.current !== workspaceId) return
      const persistedUserMessageIndex = refreshed.workspace.chat.findIndex((chatMessage) =>
        chatMessage.role === 'user' && chatMessage.request_id === retry.requestID,
      )
      const assistantReplyWasPersisted = persistedUserMessageIndex >= 0 &&
        refreshed.workspace.chat[persistedUserMessageIndex + 1]?.role === 'assistant' &&
        refreshed.workspace.chat[persistedUserMessageIndex + 1]?.mode === retry.mode
      setData((current) => current && current.workspace.workspace_id === workspaceId
        ? { ...current, workspace: { ...current.workspace, chat: refreshed.workspace.chat } }
        : current)
      if (assistantReplyWasPersisted && !retry.requiresFreshRequestID) {
        chatRequestRetryRef.current.delete(workspaceId)
        setWorkspaceChatUnread(workspaceId, !chatIsVisibleAtLatest(workspaceId))
        setError((current) => current.startsWith('暂时无法确认这条消息是否已保存') || current.startsWith('仍无法检查聊天记录') ? '' : current)
      } else if (persistedUserMessageIndex >= 0 || retry.requiresReview) {
        chatRequestRetryRef.current.set(workspaceId, { ...retry, safeToRetry: false, requiresReview: true, needsReconcile: false })
        setError('聊天记录已同步，但仍需检查这条请求是否已有回复，再决定是否重发。')
      } else {
        chatRequestRetryRef.current.set(workspaceId, { ...retry, safeToRetry: true, needsReconcile: false })
        setError((current) => current.startsWith('暂时无法确认这条消息是否已保存') || current.startsWith('仍无法检查聊天记录') ? '' : current)
      }
    } catch (reason) {
      setError(reason instanceof Error ? '仍无法检查聊天记录：' + reason.message : '仍无法检查聊天记录，请恢复连接后重试。')
    } finally {
      setCheckingChatRecovery(false)
    }
  }

  const retryAfterReview = () => {
    const workspaceId = data?.workspace.workspace_id
    const retry = workspaceId ? chatRequestRetryRef.current.get(workspaceId) : undefined
    if (!workspaceId || !retry?.requiresReview || retry.needsReconcile || sending) return
    if (!confirmDiscardEditedRewrite(workspaceId)) return
    chatRequestRetryRef.current.delete(workspaceId)
    void sendMessage(chatInput.trim() === retry.message ? undefined : retry.message, true)
  }

  const applyChatRewrite = (target: ChatApplyTarget) => {
    if (rewritePreviewOpen) lastOutsideModalFocusRef.current = editorRef.current
    const currentContent = editorContentRef.current
    if (activeWorkspaceRef.current !== target.workspaceId || currentChapterRef.current?.id !== target.chapterId) {
      setError('当前章节已切换，请在对应章节重新生成改写。')
      clearChatApplyTarget()
      return
    }
    if (target.start < 0 || target.start >= target.end || target.end > currentContent.length || currentContent.slice(target.start, target.end) !== target.original) {
      setError('选区内容已变化，未应用改写。请重新选择正文并发送请求。')
      clearChatApplyTarget()
      return
    }
    if (contextAroundSelection(currentContent, target.start, target.end) !== target.context) {
      setError('选区附近的正文已变化，未应用改写。请检查上下文后重新生成。')
      clearChatApplyTarget()
      return
    }
    const updatedContent = currentContent.slice(0, target.start) + target.replacement + currentContent.slice(target.end)
    updateEditor(updatedContent)
    setEditorSelection({ chapterId: target.chapterId, start: target.start, end: target.start + target.replacement.length })
    clearChatApplyTarget()
    setError('')
    window.requestAnimationFrame(() => {
      const editor = editorRef.current
      if (!editor) return
      editor.focus()
      editor.setSelectionRange(target.start, target.start + target.replacement.length)
    })
  }

  const confirmRewritePreview = () => {
    if (chatApplyTarget) applyChatRewrite(chatApplyTarget)
  }
  const cancelAgentRequest = () => {
    const controller = agentRequestRef.current
    if (!controller || controller.signal.aborted) return
    setCancellingRequest(true)
    controller.abort()
  }

  const applyGeneratedText = async () => {
    const workspaceId = data?.workspace.workspace_id
    if (!currentChapter || currentChapter.content === undefined || !workspaceId || projectActionBusy || agentRequestRef.current) return
    if (!confirmDiscardEditedRewrite(workspaceId, currentChapter.id)) return
    if (settingsDirty) {
      setError('请先保存模型设置，再发送 Agent 请求')
      return
    }
    const instruction = chatInput.trim() || '继续当前场景，保持人物动机一致。'
    if (countUnicodeCharacters(instruction) > maxAgentInputCharacters) {
      setError(`续写要求不能超过 ${maxAgentInputCharacters} 个字符`)
      return
    }
    const chapterId = currentChapter.id
    const controller = new AbortController()
    agentRequestRef.current = controller
    setSending(true)
    try {
      const generated = await api.chapter.generate(
        workspaceId,
        chapterId,
        instruction,
        sliceUnicodeTail(editorContentRef.current, 1800),
        editorTitle.trim() || currentChapter.title,
        editorSummary,
        contextItems,
        controller.signal,
      )
      if (activeWorkspaceRef.current !== workspaceId || currentChapterRef.current?.id !== chapterId) return
      setError((current) => current === saveErrorMessageRef.current ? current : '')
      const latestContent = editorContentRef.current
      const separator = latestContent.trim() ? '\n\n' : ''
      clearChatApplyTarget()
      updateEditor(`${latestContent}${separator}${generated.text}`)
      activateAssistantTab('chat')
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '生成正文失败')
    } finally {
      if (agentRequestRef.current === controller) agentRequestRef.current = null
      setCancellingRequest(false)
      setSending(false)
    }
  }

  return {
    sendMessage,
    checkChatRecovery,
    retryAfterReview,
    applyChatRewrite,
    confirmRewritePreview,
    cancelAgentRequest,
    applyGeneratedText,
  }
}
