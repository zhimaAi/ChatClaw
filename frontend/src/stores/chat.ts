import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { Events } from '@wailsio/runtime'
import {
  ChatService,
  type Message,
  type ImagePayload,
  SendMessageInput,
  EditAndResendInput,
} from '@bindings/chatclaw/internal/services/chat'

// Message status constants
export const MessageStatus = {
  PENDING: 'pending',
  STREAMING: 'streaming',
  SUCCESS: 'success',
  ERROR: 'error',
  CANCELLED: 'cancelled',
} as const

// Message role constants
export const MessageRole = {
  USER: 'user',
  ASSISTANT: 'assistant',
  SYSTEM: 'system',
  TOOL: 'tool',
} as const

// Event types from backend
export const ChatEventType = {
  START: 'chat:start',
  CHUNK: 'chat:chunk',
  THINKING: 'chat:thinking',
  TOOL: 'chat:tool',
  RETRIEVAL: 'chat:retrieval',
  COMPLETE: 'chat:complete',
  STOPPED: 'chat:stopped',
  ERROR: 'chat:error',
  USER_MESSAGE: 'chat:user-message',
} as const

// Tool call info for display
export interface ToolCallInfo {
  toolCallId: string
  toolName: string
  argsJson?: string
  resultJson?: string
  status: 'calling' | 'completed' | 'error'
  agentName?: string
  childToolCalls?: ToolCallInfo[]
  childContent?: string
  childThinkingContent?: string
  childSegments?: MessageSegment[]
}

// Retrieval item info for display (chat mode knowledge/memory retrieval)
export interface RetrievalItemInfo {
  source: 'knowledge' | 'memory'
  content: string
  score: number
}

// Message segment for interleaved thinking/content/tool-call/retrieval display
export type MessageSegment =
  | { type: 'thinking'; content: string }
  | { type: 'content'; content: string }
  | { type: 'tools'; toolCalls: ToolCallInfo[] }
  | { type: 'retrieval'; items: RetrievalItemInfo[] }

// Streaming message state (for the currently generating message)
export interface StreamingMessageState {
  messageId: number
  requestId: string
  content: string
  thinkingContent: string
  toolCalls: ToolCallInfo[]
  segments: MessageSegment[] // Ordered segments for interleaved rendering
  status: string
  isOpenClaw?: boolean // OpenClaw messages are not persisted in local DB
}

// Auto-decrementing counter for local optimistic message IDs (always negative to avoid collision with backend IDs)
let localMessageCounter = 0

function deepCloneToolCall(tc: ToolCallInfo): ToolCallInfo {
  return {
    ...tc,
    childToolCalls: tc.childToolCalls?.map((c) => ({ ...c })),
    childContent: tc.childContent,
    childThinkingContent: tc.childThinkingContent,
  }
}

function persistStreamingSegments(
  segmentsByMessage: Record<number, MessageSegment[]>,
  messageId: number,
  segments: MessageSegment[]
) {
  segmentsByMessage[messageId] = segments.map((seg) => {
    if (seg.type === 'thinking') {
      return { type: 'thinking' as const, content: seg.content }
    } else if (seg.type === 'content') {
      return { type: 'content' as const, content: seg.content }
    } else if (seg.type === 'retrieval') {
      return { type: 'retrieval' as const, items: seg.items.map((item) => ({ ...item })) }
    } else {
      return { type: 'tools' as const, toolCalls: seg.toolCalls.map(deepCloneToolCall) }
    }
  })
}

export const useChatStore = defineStore('chat', () => {
  // Messages by conversation ID
  const messagesByConversation = ref<Record<number, Message[]>>({})

  // Streaming state by conversation ID (only one per conversation)
  const streamingByConversation = ref<Record<number, StreamingMessageState>>({})

  // Active request ID by conversation (to match events)
  const activeRequestByConversation = ref<Record<number, string>>({})

  // Track which conversations are OpenClaw (messages not persisted in local DB)
  const openClawConversations = ref<Set<number>>(new Set())

  // Loading state by conversation
  const loadingByConversation = ref<Record<number, boolean>>({})

  // Error state by conversation
  const errorByConversation = ref<Record<number, string | null>>({})

  // Error key by message ID (for specific error messages like "max iterations exceeded")
  const errorKeyByMessage = ref<Record<number, string>>({})

  // Error detail by message ID (actual error message for user to see)
  const errorDetailByMessage = ref<Record<number, string>>({})

  // Persisted segments by message ID (for interleaved display after streaming ends)
  const segmentsByMessage = ref<Record<number, MessageSegment[]>>({})

  // Get messages for a conversation
  const getMessages = (conversationId: number) => {
    return computed(() => messagesByConversation.value[conversationId] ?? [])
  }

  // Get streaming state for a conversation
  const getStreaming = (conversationId: number) => {
    return computed(() => streamingByConversation.value[conversationId])
  }

  // Check if conversation is generating
  const isGenerating = (conversationId: number) => {
    return computed(() => !!streamingByConversation.value[conversationId])
  }

  // Load messages from backend
  const loadMessages = async (conversationId: number) => {
    if (conversationId <= 0) return

    loadingByConversation.value[conversationId] = true
    errorByConversation.value[conversationId] = null

    try {
      const isOpenClaw = openClawConversations.value.has(conversationId)
      const messages = isOpenClaw
        ? await ChatService.GetOpenClawMessages(conversationId)
        : await ChatService.GetMessages(conversationId)
      const fetched = messages ?? []
      const current = messagesByConversation.value[conversationId] ?? []
      const streaming = streamingByConversation.value[conversationId]

      if (isOpenClaw) {
        // OpenClaw messages come from Gateway (authoritative source).
        if (streaming) {
          // While streaming, don't touch messages — streaming events manage state.
        } else if (fetched.length > 0) {
          // Not streaming: Gateway history is authoritative — replace entirely.
          // Clean up old segment data for messages that will be replaced.
          for (const msg of current) {
            delete segmentsByMessage.value[msg.id]
          }
          messagesByConversation.value[conversationId] = fetched
        } else if (fetched.length === 0 && current.length > 0) {
          // Gateway returned empty but we have local messages (e.g. optimistic
          // user message just sent). Keep them until Gateway catches up.
        }
      } else {
      const normalizeContent = (v: unknown) => String(v ?? '').trim()

      // IMPORTANT:
      // On first app launch / first message, there are multiple async flows running:
      // - ChatMessageList watches conversationId and calls loadMessages()
      // - sendMessage() optimistically appends a local user message (negative ID)
      // - backend emits streaming events to upsert the assistant placeholder
      //
      // If loadMessages() blindly replaces the array, it can temporarily wipe the optimistic
      // user message and/or streaming assistant placeholder, making the UI look empty until
      // a later reload (often when streaming completes).
      //
      // Strategy:
      // - If backend returns empty but we already have local messages, do not overwrite.
      // - Otherwise, merge fetched messages with local optimistic messages and streaming placeholder.
      if (fetched.length === 0 && current.length > 0) {
        // Keep current messages; avoid clobbering optimistic/streaming state.
      } else {
        const fetchedIds = new Set(fetched.map((m) => m.id))
        const merged: Message[] = [...fetched]

        // Keep local optimistic messages (negative IDs) unless the backend already has the same one.
        // This avoids duplicates after a later reload.
        for (const msg of current) {
          if (msg.id >= 0) continue
          const existsOnServer = fetched.some(
            (fm) =>
              fm.role === msg.role && normalizeContent(fm.content) === normalizeContent(msg.content)
          )
          if (!existsOnServer) {
            merged.push(msg)
          }
        }

        // Keep streaming assistant placeholder/message if it's not in fetched yet.
        if (streaming && !fetchedIds.has(streaming.messageId)) {
          const localStreamingMsg = current.find((m) => m.id === streaming.messageId)
          if (localStreamingMsg) {
            merged.push(localStreamingMsg)
          }
        }

        messagesByConversation.value[conversationId] = merged
      }
      }

      // Build a map of tool results from tool-role messages (tool_call_id → content)
      const toolResultMap = new Map<string, string>()
      for (const msg of fetched) {
        if (msg.role === 'tool' && msg.tool_call_id) {
          toolResultMap.set(msg.tool_call_id, msg.content)
        }
      }

      // Parse and restore segments from backend for each assistant message
      for (const msg of fetched) {
        if (msg.role === 'assistant' && msg.segments) {
          try {
            const rawSegments = JSON.parse(msg.segments) as Array<{
              type: string
              content?: string
              tool_call_ids?: string[]
              retrieval_items?: Array<{ source: string; content: string; score: number }>
            }>
            if (Array.isArray(rawSegments) && rawSegments.length > 0) {
              // Parse tool_calls from the message to build ToolCallInfo map
              const toolCallMap = new Map<string, ToolCallInfo>()
              if (msg.tool_calls) {
                try {
                  const toolCalls = JSON.parse(msg.tool_calls) as Array<{
                    ID?: string
                    id?: string
                    Function?: { Name?: string; Arguments?: string }
                    function?: { name?: string; arguments?: string }
                  }>
                  for (const tc of toolCalls) {
                    const id = tc.ID || tc.id || ''
                    const name = tc.Function?.Name || tc.function?.name || ''
                    const args = tc.Function?.Arguments || tc.function?.arguments || ''
                    if (id) {
                      toolCallMap.set(id, {
                        toolCallId: id,
                        toolName: name,
                        argsJson: args,
                        resultJson: toolResultMap.get(id),
                        status: 'completed',
                      })
                    }
                  }
                } catch {
                  // Ignore parse errors for tool_calls
                }
              }

              // Convert backend segments to frontend format
              const frontendSegments: MessageSegment[] = rawSegments.map((seg) => {
                if (seg.type === 'thinking') {
                  return { type: 'thinking' as const, content: seg.content || '' }
                } else if (seg.type === 'content') {
                  return { type: 'content' as const, content: seg.content || '' }
                } else if (seg.type === 'tools') {
                  const toolCalls: ToolCallInfo[] = (seg.tool_call_ids || [])
                    .map((id) => toolCallMap.get(id))
                    .filter((tc): tc is ToolCallInfo => tc !== undefined)
                  return { type: 'tools' as const, toolCalls }
                } else if (seg.type === 'retrieval') {
                  const items: RetrievalItemInfo[] = (seg.retrieval_items || []).map((item) => ({
                    source: item.source as 'knowledge' | 'memory',
                    content: item.content,
                    score: item.score,
                  }))
                  return { type: 'retrieval' as const, items }
                }
                // Fallback for unknown segment types
                return { type: 'content' as const, content: '' }
              })

              segmentsByMessage.value[msg.id] = frontendSegments
            }
          } catch {
            // Ignore parse errors for segments
          }
        }
      }
    } catch (error: unknown) {
      errorByConversation.value[conversationId] =
        error instanceof Error ? error.message : 'Failed to load messages'
      // Do not clear existing messages on transient failures; keeping UI stable is preferred.
      if (!messagesByConversation.value[conversationId]) {
        messagesByConversation.value[conversationId] = []
      }
    } finally {
      loadingByConversation.value[conversationId] = false
    }
  }

  const ensureConversationMessages = (conversationId: number) => {
    if (!messagesByConversation.value[conversationId]) {
      messagesByConversation.value[conversationId] = []
    }
  }

  const appendMessage = (conversationId: number, msg: Message) => {
    ensureConversationMessages(conversationId)
    messagesByConversation.value[conversationId] = [
      ...messagesByConversation.value[conversationId],
      msg,
    ]
  }

  const upsertMessage = (conversationId: number, messageId: number, patch: Partial<Message>) => {
    ensureConversationMessages(conversationId)
    const current = messagesByConversation.value[conversationId]
    const idx = current.findIndex((m) => m.id === messageId)
    if (idx >= 0) {
      const next = [...current]
      next[idx] = { ...next[idx], ...patch } as Message
      messagesByConversation.value[conversationId] = next
      return
    }

    // Insert as new message if not found (fallback)
    messagesByConversation.value[conversationId] = [
      ...current,
      { ...(patch as any), id: messageId } as Message,
    ]
  }

  const removeMessage = (conversationId: number, messageId: number) => {
    ensureConversationMessages(conversationId)
    messagesByConversation.value[conversationId] = messagesByConversation.value[
      conversationId
    ].filter((m) => m.id !== messageId)
  }

  // Append a local-only message (used by team mode SSE integration).
  const appendLocalMessage = (conversationId: number, role: string, content: string) => {
    if (conversationId === 0) return 0
    localMessageCounter -= 1
    const localMessageId = localMessageCounter
    appendMessage(conversationId, {
      id: localMessageId,
      conversation_id: conversationId,
      role,
      content: content.trim(),
      status: MessageStatus.SUCCESS,
      thinking_content: '',
      tool_calls: '[]',
      input_tokens: 0,
      output_tokens: 0,
      created_at: null as any,
      updated_at: null as any,
    } as any)
    return localMessageId
  }

  // Send a new message
  const sendMessage = async (
    conversationId: number,
    content: string,
    tabId: string,
    images?: Array<{
      id: string
      file: File
      mimeType: string
      base64: string
      dataUrl: string
      fileName: string
      size: number
    }>,
    files?: Array<{
      id: string
      file: File
      mimeType: string
      base64: string
      fileName: string
      size: number
    }>
  ) => {
    const hasContent =
      content.trim() !== '' || (images && images.length > 0) || (files && files.length > 0)
    if (conversationId <= 0 || !hasContent) return null

    // Map images to ImagePayload format
    const imagePayloads =
      images?.map((img) => ({
        kind: 'image',
        source: 'inline_base64',
        mime_type: img.mimeType,
        base64: img.base64,
        file_name: img.fileName,
        size: img.size,
      })) || []

    // Map files to ImagePayload format (reusing images field)
    const filePayloads =
      files?.map((f) => ({
        kind: 'file',
        source: 'inline_base64',
        mime_type: f.mimeType,
        base64: f.base64,
        file_name: f.fileName,
        original_name: f.fileName,
        size: f.size,
      })) || []

    const allPayloads = [...imagePayloads, ...filePayloads]

    // Optimistically append user message (backend inserts user msg, but doesn't emit an event for it)
    localMessageCounter -= 1
    const localUserMessageId = localMessageCounter
    appendMessage(conversationId, {
      id: localUserMessageId,
      conversation_id: conversationId,
      role: MessageRole.USER,
      content: content.trim(),
      status: MessageStatus.SUCCESS,
      thinking_content: '',
      tool_calls: '[]',
      images_json: JSON.stringify(allPayloads),
      input_tokens: 0,
      output_tokens: 0,
      created_at: null as any,
      updated_at: null as any,
    } as any)

    try {
      const result = await ChatService.SendMessage(
        new SendMessageInput({
          conversation_id: conversationId,
          content: content.trim(),
          tab_id: tabId,
          images: allPayloads,
        })
      )

      if (result) {
        activeRequestByConversation.value[conversationId] = result.request_id
      }

      return result
    } catch (error: unknown) {
      // Rollback optimistic message on failure
      removeMessage(conversationId, localUserMessageId)
      throw error
    }
  }

  // Send a message via the OpenClaw Gateway chat agent
  const sendOpenClawMessage = async (
    conversationId: number,
    content: string,
    tabId: string,
    images?: Array<{
      id: string
      file: File
      mimeType: string
      base64: string
      dataUrl: string
      fileName: string
      size: number
    }>,
    files?: Array<{
      id: string
      file: File
      mimeType: string
      base64: string
      fileName: string
      size: number
    }>
  ) => {
    const hasContent =
      content.trim() !== '' || (images && images.length > 0) || (files && files.length > 0)
    if (conversationId <= 0 || !hasContent) return null

    const imagePayloads =
      images?.map((img) => ({
        kind: 'image',
        source: 'inline_base64',
        mime_type: img.mimeType,
        base64: img.base64,
        file_name: img.fileName,
        size: img.size,
      })) || []

    const filePayloads =
      files?.map((f) => ({
        kind: 'file',
        source: 'inline_base64',
        mime_type: f.mimeType,
        base64: f.base64,
        file_name: f.fileName,
        original_name: f.fileName,
        size: f.size,
      })) || []

    const allPayloads = [...imagePayloads, ...filePayloads]

    localMessageCounter -= 1
    const localUserMessageId = localMessageCounter
    appendMessage(conversationId, {
      id: localUserMessageId,
      conversation_id: conversationId,
      role: MessageRole.USER,
      content: content.trim(),
      status: MessageStatus.SUCCESS,
      thinking_content: '',
      tool_calls: '[]',
      images_json: JSON.stringify(allPayloads),
      input_tokens: 0,
      output_tokens: 0,
      created_at: null as any,
      updated_at: null as any,
    } as any)

    openClawConversations.value.add(conversationId)

    try {
      const result = await ChatService.SendOpenClawMessage(
        new SendMessageInput({
          conversation_id: conversationId,
          content: content.trim(),
          tab_id: tabId,
          images: allPayloads,
        })
      )

      if (result) {
        activeRequestByConversation.value[conversationId] = result.request_id
      }

      return result
    } catch (error: unknown) {
      removeMessage(conversationId, localUserMessageId)
      throw error
    }
  }

  // Edit and resend a message
  const editAndResend = async (
    conversationId: number,
    messageId: number,
    newContent: string,
    tabId: string,
    images?: ImagePayload[]
  ) => {
    if (conversationId <= 0 || messageId <= 0 || !newContent.trim()) return null

    // Get existing images from the message being edited
    const current = messagesByConversation.value[conversationId]
    const msgToEdit = current?.find((m) => m.id === messageId)
    const existingImagesJson = msgToEdit?.images_json

    // Parse existing images if any
    let existingImages: ImagePayload[] = []
    if (existingImagesJson) {
      try {
        existingImages = JSON.parse(existingImagesJson) as ImagePayload[]
      } catch (e) {
        console.warn('Failed to parse existing images_json:', e)
      }
    }

    // Map new images to ImagePayload format (preserve all fields including file attachments)
    const newImagePayloads: ImagePayload[] =
      images?.map((img) => ({
        id: img.id,
        kind: img.kind || 'image',
        source: img.source || 'inline_base64',
        mime_type: img.mime_type,
        base64: img.base64,
        data_url: img.data_url,
        file_name: img.file_name,
        file_path: img.file_path,
        original_name: img.original_name,
        size: img.size,
      })) || []

    // Combine existing images with new images (if new images are provided, they replace existing)
    const imagePayloads: ImagePayload[] =
      newImagePayloads.length > 0 ? newImagePayloads : existingImages

    // Optimistic UX: immediately update the edited message and truncate following messages
    ensureConversationMessages(conversationId)
    const idx = current?.findIndex((m) => m.id === messageId) ?? -1
    if (idx >= 0 && current) {
      const next = [...current]
      next[idx] = {
        ...next[idx],
        content: newContent.trim(),
        images_json: JSON.stringify(imagePayloads),
      } as Message
      messagesByConversation.value[conversationId] = next.slice(0, idx + 1)
    }

    // Clear any existing streaming state so UI immediately reflects "new run"
    delete streamingByConversation.value[conversationId]
    delete activeRequestByConversation.value[conversationId]

    try {
      const result = await ChatService.EditAndResend(
        new EditAndResendInput({
          conversation_id: conversationId,
          message_id: messageId,
          new_content: newContent.trim(),
          tab_id: tabId,
          images: imagePayloads,
        })
      )

      if (result) {
        activeRequestByConversation.value[conversationId] = result.request_id
      }

      return result
    } catch (error: unknown) {
      // If failed, reload from backend to restore consistent state
      void loadMessages(conversationId)
      throw error
    }
  }

  // Edit and resend a message via OpenClaw API
  const editAndResendOpenClaw = async (
    conversationId: number,
    messageId: number,
    newContent: string,
    tabId: string,
    images?: ImagePayload[]
  ) => {
    const hasContent = newContent.trim() !== '' || (images && images.length > 0)
    if (conversationId <= 0 || messageId <= 0 || !hasContent) return null

    const current = messagesByConversation.value[conversationId]
    const msgToEdit = current?.find((m) => m.id === messageId)
    const existingImagesJson = msgToEdit?.images_json

    let existingImages: ImagePayload[] = []
    if (existingImagesJson) {
      try {
        existingImages = JSON.parse(existingImagesJson) as ImagePayload[]
      } catch (e) {
        console.warn('Failed to parse existing images_json:', e)
      }
    }

    const newImagePayloads: ImagePayload[] =
      images?.map((img) => ({
        id: img.id,
        kind: img.kind || 'image',
        source: img.source || 'inline_base64',
        mime_type: img.mime_type,
        base64: img.base64,
        data_url: img.data_url,
        file_name: img.file_name,
        file_path: img.file_path,
        original_name: img.original_name,
        size: img.size,
      })) || []

    const imagePayloads: ImagePayload[] =
      newImagePayloads.length > 0 ? newImagePayloads : existingImages

    ensureConversationMessages(conversationId)
    const idx = current?.findIndex((m) => m.id === messageId) ?? -1
    if (idx >= 0 && current) {
      const next = [...current]
      next[idx] = {
        ...next[idx],
        content: newContent.trim(),
        images_json: JSON.stringify(imagePayloads),
      } as Message
      messagesByConversation.value[conversationId] = next.slice(0, idx + 1)
    }

    delete streamingByConversation.value[conversationId]
    delete activeRequestByConversation.value[conversationId]

    openClawConversations.value.add(conversationId)

    try {
      const result = await ChatService.EditAndResendOpenClaw(
        new EditAndResendInput({
          conversation_id: conversationId,
          message_id: messageId,
          new_content: newContent.trim(),
          tab_id: tabId,
          images: imagePayloads,
        })
      )

      if (result) {
        activeRequestByConversation.value[conversationId] = result.request_id
      }

      return result
    } catch (error: unknown) {
      void loadMessages(conversationId)
      throw error
    }
  }

  // Stop generation
  const stopGeneration = async (conversationId: number) => {
    if (conversationId <= 0) return

    try {
      await ChatService.StopGeneration(conversationId)
    } catch (error: unknown) {
      // Ignore errors when stopping (might already be stopped)
      console.warn('Stop generation error:', error)
    }
  }

  const markOpenClawConversation = (conversationId: number) => {
    openClawConversations.value.add(conversationId)
  }

  // Clear messages for a conversation
  const clearMessages = (conversationId: number) => {
    // Get message IDs before clearing to clean up auxiliary maps
    const messages = messagesByConversation.value[conversationId] ?? []
    for (const msg of messages) {
      delete segmentsByMessage.value[msg.id]
      delete errorKeyByMessage.value[msg.id]
      delete errorDetailByMessage.value[msg.id]
    }

    messagesByConversation.value[conversationId] = []
    delete streamingByConversation.value[conversationId]
    delete activeRequestByConversation.value[conversationId]
  }

  // Handle user message event (emitted by backend after inserting user message).
  // When the user sends from the UI, an optimistic message with a negative ID
  // already exists — replace it with the real backend ID instead of duplicating.
  const handleChatUserMessage = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, message_id, content, images_json } = data

    const messages = messagesByConversation.value[conversation_id]
    if (messages) {
      const optimisticIdx = messages.findIndex(
        (m) => m.id < 0 && m.role === MessageRole.USER && m.content === (content || '')
      )
      if (optimisticIdx >= 0) {
        const next = [...messages]
        next[optimisticIdx] = { ...next[optimisticIdx], id: message_id } as Message
        messagesByConversation.value[conversation_id] = next
        return
      }
    }

    upsertMessage(conversation_id, message_id, {
      id: message_id,
      conversation_id,
      role: MessageRole.USER,
      content: content || '',
      status: MessageStatus.SUCCESS,
      thinking_content: '',
      tool_calls: '[]',
      images_json: images_json || '[]',
      input_tokens: 0,
      output_tokens: 0,
      created_at: null as any,
      updated_at: null as any,
    } as any)
  }

  // Handle chat events from backend
  const handleChatStart = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, message_id } = data

    // Initialize streaming state
    streamingByConversation.value[conversation_id] = {
      messageId: message_id,
      requestId: request_id,
      content: '',
      thinkingContent: '',
      toolCalls: [],
      segments: [],
      status: MessageStatus.STREAMING,
    }

    // Ensure assistant placeholder exists in message list so streaming updates don't "jump"
    upsertMessage(conversation_id, message_id, {
      id: message_id,
      conversation_id,
      role: MessageRole.ASSISTANT,
      content: '',
      status: MessageStatus.STREAMING,
      thinking_content: '',
      tool_calls: '[]',
      input_tokens: 0,
      output_tokens: 0,
      created_at: null as any,
      updated_at: null as any,
    } as any)

  }

  const SUB_AGENT_NAMES = new Set(['general_purpose', 'bash'])

  const extractSubAgentName = (runPath?: string[]): string | undefined => {
    if (!Array.isArray(runPath) || runPath.length < 2) return undefined
    const agentName = runPath[1]
    return SUB_AGENT_NAMES.has(agentName) ? agentName : undefined
  }

  const findSubAgentToolCall = (
    streaming: StreamingMessageState,
    agentName: string,
    parentToolCallId?: string
  ): ToolCallInfo | undefined => {
    if (parentToolCallId) {
      return streaming.toolCalls.find((tc) => tc.toolCallId === parentToolCallId)
    }
    let lastCalling: ToolCallInfo | undefined
    let lastMatch: ToolCallInfo | undefined
    for (const tc of streaming.toolCalls) {
      if (tc.toolName !== agentName) continue
      if (tc.status === 'calling') lastCalling = tc
      else if (tc.status === 'completed') lastMatch = tc
    }
    return lastCalling ?? lastMatch
  }

  const handleChatChunk = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta, run_path, parent_tool_call_id } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      const chunk = delta || ''
      if (!chunk) return

      const subAgentName = extractSubAgentName(run_path)

      if (subAgentName) {
        const parent = findSubAgentToolCall(streaming, subAgentName, parent_tool_call_id)
        if (parent) {
          parent.childContent = (parent.childContent || '') + chunk
          if (!parent.childSegments) parent.childSegments = []
          const lastChild = parent.childSegments[parent.childSegments.length - 1]
          if (lastChild && lastChild.type === 'content') {
            lastChild.content += chunk
          } else {
            parent.childSegments.push({ type: 'content', content: chunk })
          }
          return
        }
      }

      streaming.content += chunk

      const lastSeg = streaming.segments[streaming.segments.length - 1]
      if (lastSeg && lastSeg.type === 'content') {
        lastSeg.content += chunk
      } else {
        streaming.segments.push({ type: 'content', content: chunk })
      }

      upsertMessage(conversation_id, streaming.messageId, {
        content: streaming.content,
      })
    }
  }

  const handleChatThinking = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta, new_block, run_path, parent_tool_call_id } =
      data
    const streaming = streamingByConversation.value[conversation_id]

    console.log('[chat-thinking] received', {
      conversation_id,
      request_id,
      deltaLen: delta?.length,
      deltaPreview: delta?.slice(0, 80),
      new_block,
      hasStreaming: !!streaming,
      streamingRequestId: streaming?.requestId,
      requestMatch: streaming?.requestId === request_id,
      currentSegments: streaming?.segments?.map((s: any) => s.type),
    })

    if (streaming && streaming.requestId === request_id) {
      const chunk = delta || ''
      if (!chunk) return

      const subAgentName = extractSubAgentName(run_path)

      if (subAgentName) {
        const parent = findSubAgentToolCall(streaming, subAgentName, parent_tool_call_id)
        if (parent) {
          parent.childThinkingContent = (parent.childThinkingContent || '') + chunk
          if (!parent.childSegments) parent.childSegments = []
          const lastChild = parent.childSegments[parent.childSegments.length - 1]
          if (lastChild && lastChild.type === 'thinking' && !new_block) {
            lastChild.content += chunk
          } else {
            parent.childSegments.push({ type: 'thinking', content: chunk })
          }
          return
        }
      }

      streaming.thinkingContent += chunk

      const lastSeg = streaming.segments[streaming.segments.length - 1]

      console.log('[chat-thinking] appending to segments', {
        new_block,
        lastSegType: lastSeg?.type,
        action: new_block
          ? 'push new thinking segment'
          : lastSeg?.type === 'thinking'
            ? 'append to existing'
            : 'push new thinking segment',
        totalSegments: streaming.segments.length,
        thinkingContentLen: streaming.thinkingContent.length,
      })

      if (new_block) {
        streaming.segments.push({ type: 'thinking', content: chunk })
      } else if (lastSeg && lastSeg.type === 'thinking') {
        lastSeg.content += chunk
      } else {
        streaming.segments.push({ type: 'thinking', content: chunk })
      }

      upsertMessage(conversation_id, streaming.messageId, {
        thinking_content: streaming.thinkingContent,
      } as any)
    } else {
      console.warn('[chat-thinking] DROPPED - no matching streaming state', {
        conversation_id,
        request_id,
        hasStreaming: !!streaming,
        streamingRequestId: streaming?.requestId,
      })
    }
  }

  const handleChatTool = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const {
      conversation_id,
      request_id,
      type,
      tool_call_id,
      tool_name,
      args_json,
      result_json,
      run_path,
      parent_tool_call_id,
    } = data
    const streaming = streamingByConversation.value[conversation_id]

    // Guard: ignore empty tool_call_id events (some providers stream partial tool deltas)
    if (!tool_call_id) {
      debug('tool event ignored (empty tool_call_id)', {
        conversation_id,
        request_id,
        type,
        tool_name,
      })
      return
    }

    if (!streaming) {
      debug('tool event ignored (no streaming)', {
        conversation_id,
        request_id,
        type,
        tool_call_id,
      })
      return
    }

    if (streaming.requestId !== request_id) {
      debug('tool event ignored (request mismatch)', {
        conversation_id,
        request_id,
        active: streaming.requestId,
        type,
        tool_call_id,
      })
      return
    }

    const subAgentName = extractSubAgentName(run_path)

    // Helper: add a new tool call to segments (or nest under parent sub-agent)
    const addToolCallToSegments = (toolCall: ToolCallInfo) => {
      if (subAgentName) {
        const parent = findSubAgentToolCall(streaming, subAgentName, parent_tool_call_id)
        if (parent) {
          if (!parent.childToolCalls) parent.childToolCalls = []
          parent.childToolCalls.push(toolCall)
          if (!parent.childSegments) parent.childSegments = []
          const lastChild = parent.childSegments[parent.childSegments.length - 1]
          if (lastChild && lastChild.type === 'tools') {
            lastChild.toolCalls.push(toolCall)
          } else {
            parent.childSegments.push({ type: 'tools', toolCalls: [toolCall] })
          }
          return
        }
      }
      const lastSeg = streaming.segments[streaming.segments.length - 1]
      if (lastSeg && lastSeg.type === 'tools') {
        lastSeg.toolCalls.push(toolCall)
      } else {
        streaming.segments.push({ type: 'tools', toolCalls: [toolCall] })
      }
    }

    if (streaming && streaming.requestId === request_id) {
      if (type === 'call') {
        // Tool call (dedupe by tool_call_id; some providers may emit repeated snapshots)
        const existing = streaming.toolCalls.find((tc) => tc.toolCallId === tool_call_id)
        if (existing) {
          existing.toolName = tool_name
          existing.argsJson = args_json
          if (!existing.agentName && subAgentName) existing.agentName = subAgentName
          // Don't downgrade completed to calling
          if (existing.status !== 'completed') {
            existing.status = 'calling'
          }
        } else {
          const newToolCall: ToolCallInfo = {
            toolCallId: tool_call_id,
            toolName: tool_name,
            argsJson: args_json,
            status: 'calling',
            agentName: subAgentName,
          }
          streaming.toolCalls.push(newToolCall)
          // Track in segments (same object reference so updates propagate)
          addToolCallToSegments(newToolCall)
        }
      } else if (type === 'result') {
        // Tool result (result may arrive before call event)
        const existing = streaming.toolCalls.find((tc) => tc.toolCallId === tool_call_id)
        if (existing) {
          // Update existing - same object reference in segments, so changes propagate
          existing.toolName = existing.toolName || tool_name
          existing.resultJson = result_json
          existing.status = 'completed'
          if (!existing.agentName && subAgentName) existing.agentName = subAgentName
        } else {
          // Try best-effort merge by (toolName + argsJson) to avoid duplicates like "calling" + "completed"
          const mergeCandidate = streaming.toolCalls.find(
            (tc) =>
              tc.status === 'calling' &&
              tc.toolName === tool_name &&
              // If result includes no args_json, allow merge; otherwise require same args
              (!args_json || tc.argsJson === args_json)
          )
          if (mergeCandidate) {
            // Update merge candidate - same object reference in segments
            mergeCandidate.toolCallId = tool_call_id || mergeCandidate.toolCallId
            mergeCandidate.resultJson = result_json
            mergeCandidate.status = 'completed'
            if (!mergeCandidate.agentName && subAgentName) mergeCandidate.agentName = subAgentName
          } else {
            const newToolCall: ToolCallInfo = {
              toolCallId: tool_call_id,
              toolName: tool_name,
              resultJson: result_json,
              status: 'completed',
              agentName: subAgentName,
            }
            streaming.toolCalls.push(newToolCall)
            // Track in segments
            addToolCallToSegments(newToolCall)
          }
        }
      }

      // Mirror tool calls into the message so UI can render them consistently
      upsertMessage(conversation_id, streaming.messageId, {
        tool_calls: JSON.stringify(streaming.toolCalls),
      } as any)
    }
  }

  const handleChatRetrieval = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, items } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (!streaming || streaming.requestId !== request_id) return
    if (!Array.isArray(items) || items.length === 0) return

    const retrievalItems: RetrievalItemInfo[] = items.map((item: any) => ({
      source: item.source as 'knowledge' | 'memory',
      content: String(item.content ?? ''),
      score: Number(item.score ?? 0),
    }))

    streaming.segments.push({ type: 'retrieval', items: retrievalItems })
  }

  const handleChatComplete = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      // Finalize message locally first to avoid visible "jump"
      upsertMessage(conversation_id, streaming.messageId, {
        content: streaming.content,
        thinking_content: streaming.thinkingContent,
        tool_calls: JSON.stringify(streaming.toolCalls),
        status: MessageStatus.SUCCESS,
      } as any)

      if (openClawConversations.value.has(conversation_id)) {
        persistStreamingSegments(segmentsByMessage.value, streaming.messageId, streaming.segments)
        setTimeout(() => {
          delete streamingByConversation.value[conversation_id]
          delete activeRequestByConversation.value[conversation_id]
        }, 0)
      } else {
        // Clear streaming state first
        delete streamingByConversation.value[conversation_id]
        delete activeRequestByConversation.value[conversation_id]

        // Refresh messages from backend - segments will be loaded from backend
        void loadMessages(conversation_id)
      }
    }
  }

  const handleChatStopped = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      upsertMessage(conversation_id, streaming.messageId, {
        content: streaming.content,
        thinking_content: streaming.thinkingContent,
        tool_calls: JSON.stringify(streaming.toolCalls),
        status: MessageStatus.CANCELLED,
      } as any)

      if (openClawConversations.value.has(conversation_id)) {
        persistStreamingSegments(segmentsByMessage.value, streaming.messageId, streaming.segments)
        setTimeout(() => {
          delete streamingByConversation.value[conversation_id]
          delete activeRequestByConversation.value[conversation_id]
        }, 0)
      } else {
        // Clear streaming state first
        delete streamingByConversation.value[conversation_id]
        delete activeRequestByConversation.value[conversation_id]

        // Refresh messages from backend - segments will be loaded from backend
        void loadMessages(conversation_id)
      }
    }
  }

  const handleChatError = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, error_key, error_data } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      streaming.status = MessageStatus.ERROR

      // Store error message
      errorByConversation.value[conversation_id] = error_key

      // Store error key per message for specific error display
      if (error_key) {
        errorKeyByMessage.value[streaming.messageId] = error_key
      }

      // Store error detail from error_data.Error if available
      if (error_data?.Error) {
        // Extract the most useful part of the error message
        const fullError = String(error_data.Error)
        // Try to extract the actual error message (after "err=")
        const errMatch = fullError.match(/err=(.+?)(?:\n|$)/)
        if (errMatch) {
          errorDetailByMessage.value[streaming.messageId] = errMatch[1].trim()
        } else {
          // Fallback: take the first line, strip common prefixes
          let briefError = fullError.split('\n')[0].trim()
          // Remove common error prefixes for cleaner display
          briefError = briefError
            .replace(/^\[NodeRunError\]\s*/, '')
            .replace(/^\[LocalFunc\]\s*/, '')
            .replace(/^failed to \w+ tool \w+:\s*/i, '')
          errorDetailByMessage.value[streaming.messageId] = briefError || fullError.split('\n')[0]
        }
      }

      upsertMessage(conversation_id, streaming.messageId, {
        content: streaming.content,
        thinking_content: streaming.thinkingContent,
        tool_calls: JSON.stringify(streaming.toolCalls),
        status: MessageStatus.ERROR,
      } as any)

      // Persist streaming segments locally since we don't call loadMessages for errors.
      persistStreamingSegments(segmentsByMessage.value, streaming.messageId, streaming.segments)

      // Clear streaming state after a tick to let the UI read from segments first
      setTimeout(() => {
        delete streamingByConversation.value[conversation_id]
        delete activeRequestByConversation.value[conversation_id]
      }, 0)
    }
  }

  // Helper to extract event data
  const extractEventData = (event: any) => {
    if (!event) return null
    // Wails events may wrap data in an array
    return Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
  }

  // Subscribe to chat events
  let unsubscribers: Array<() => void> = []
  let subscriptionRefCount = 0
  const debugEnabled =
    typeof window !== 'undefined' && window.localStorage?.getItem('debug:chat') === '1'

  const debug = (...args: any[]) => {
    if (debugEnabled) {
      console.debug('[chat]', ...args)
    }
  }

  const subscribe = () => {
    subscriptionRefCount += 1
    if (subscriptionRefCount > 1) {
      debug('subscribe skipped (already subscribed)', { subscriptionRefCount })
      return
    }

    debug('subscribe', { subscriptionRefCount })
    unsubscribers.push(
      Events.On(ChatEventType.USER_MESSAGE, (e: any) => {
        debug(ChatEventType.USER_MESSAGE, extractEventData(e))
        handleChatUserMessage(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.START, (e: any) => {
        debug(ChatEventType.START, extractEventData(e))
        handleChatStart(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.CHUNK, (e: any) => {
        console.log('[chat] >>> CHUNK event arrived at frontend')
        debug(ChatEventType.CHUNK, extractEventData(e))
        handleChatChunk(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.THINKING, (e: any) => {
        console.log('[chat] >>> THINKING event arrived at frontend', extractEventData(e))
        debug(ChatEventType.THINKING, extractEventData(e))
        handleChatThinking(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.TOOL, (e: any) => {
        debug(ChatEventType.TOOL, extractEventData(e))
        handleChatTool(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.RETRIEVAL, (e: any) => {
        debug(ChatEventType.RETRIEVAL, extractEventData(e))
        handleChatRetrieval(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.COMPLETE, (e: any) => {
        debug(ChatEventType.COMPLETE, extractEventData(e))
        handleChatComplete(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.STOPPED, (e: any) => {
        debug(ChatEventType.STOPPED, extractEventData(e))
        handleChatStopped(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.ERROR, (e: any) => {
        // Always log errors to console

        console.error('[chat] chat:error', extractEventData(e))
        handleChatError(e)
      })
    )
  }

  const unsubscribe = () => {
    subscriptionRefCount = Math.max(0, subscriptionRefCount - 1)
    if (subscriptionRefCount > 0) {
      debug('unsubscribe skipped (still in use)', { subscriptionRefCount })
      return
    }

    debug('unsubscribe', { subscriptionRefCount })
    unsubscribers.forEach((unsub) => unsub?.())
    unsubscribers = []
  }

  return {
    // State
    messagesByConversation,
    streamingByConversation,
    loadingByConversation,
    errorByConversation,
    errorKeyByMessage,
    errorDetailByMessage,
    segmentsByMessage,

    // Getters
    getMessages,
    getStreaming,
    isGenerating,

    // Actions
    loadMessages,
    sendMessage,
    sendOpenClawMessage,
    editAndResend,
    editAndResendOpenClaw,
    stopGeneration,
    clearMessages,
    appendLocalMessage,
    markOpenClawConversation,

    // Event subscription
    subscribe,
    unsubscribe,
  }
})
