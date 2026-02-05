import { ref, computed } from 'vue'
import { defineStore } from 'pinia'
import { Events } from '@wailsio/runtime'
import {
  ChatService,
  type Message,
  SendMessageInput,
  EditAndResendInput,
} from '@bindings/willchat/internal/services/chat'

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
  COMPLETE: 'chat:complete',
  STOPPED: 'chat:stopped',
  ERROR: 'chat:error',
} as const

// Tool call info for display
export interface ToolCallInfo {
  toolCallId: string
  toolName: string
  argsJson?: string
  resultJson?: string
  status: 'calling' | 'completed' | 'error'
}

// Streaming message state (for the currently generating message)
export interface StreamingMessageState {
  messageId: number
  requestId: string
  content: string
  thinkingContent: string
  toolCalls: ToolCallInfo[]
  status: string
}

export const useChatStore = defineStore('chat', () => {
  // Messages by conversation ID
  const messagesByConversation = ref<Record<number, Message[]>>({})

  // Streaming state by conversation ID (only one per conversation)
  const streamingByConversation = ref<Record<number, StreamingMessageState>>({})

  // Active request ID by conversation (to match events)
  const activeRequestByConversation = ref<Record<number, string>>({})

  // Loading state by conversation
  const loadingByConversation = ref<Record<number, boolean>>({})

  // Error state by conversation
  const errorByConversation = ref<Record<number, string | null>>({})

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
      const messages = await ChatService.GetMessages(conversationId)
      messagesByConversation.value[conversationId] = messages ?? []
    } catch (error: unknown) {
      errorByConversation.value[conversationId] =
        error instanceof Error ? error.message : 'Failed to load messages'
      messagesByConversation.value[conversationId] = []
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
    messagesByConversation.value[conversationId] = [...messagesByConversation.value[conversationId], msg]
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
    messagesByConversation.value[conversationId] = [...current, { ...(patch as any), id: messageId } as Message]
  }

  const removeMessage = (conversationId: number, messageId: number) => {
    ensureConversationMessages(conversationId)
    messagesByConversation.value[conversationId] = messagesByConversation.value[conversationId].filter(
      (m) => m.id !== messageId
    )
  }

  // Send a new message
  const sendMessage = async (conversationId: number, content: string, tabId: string) => {
    if (conversationId <= 0 || !content.trim()) return null

    // Optimistically append user message (backend inserts user msg, but doesn't emit an event for it)
    const localUserMessageId = -Date.now()
    appendMessage(conversationId, {
      id: localUserMessageId,
      conversation_id: conversationId,
      role: MessageRole.USER,
      content: content.trim(),
      status: MessageStatus.SUCCESS,
      thinking_content: '',
      tool_calls: '[]',
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

  // Edit and resend a message
  const editAndResend = async (
    conversationId: number,
    messageId: number,
    newContent: string,
    tabId: string
  ) => {
    if (conversationId <= 0 || messageId <= 0 || !newContent.trim()) return null

    // Optimistic UX: immediately update the edited message and truncate following messages
    ensureConversationMessages(conversationId)
    const current = messagesByConversation.value[conversationId]
    const idx = current.findIndex((m) => m.id === messageId)
    if (idx >= 0) {
      const next = [...current]
      next[idx] = { ...next[idx], content: newContent.trim() } as Message
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

  // Clear messages for a conversation
  const clearMessages = (conversationId: number) => {
    messagesByConversation.value[conversationId] = []
    delete streamingByConversation.value[conversationId]
    delete activeRequestByConversation.value[conversationId]
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

  const handleChatChunk = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      streaming.content += delta || ''
      upsertMessage(conversation_id, streaming.messageId, {
        content: streaming.content,
      })
    }
  }

  const handleChatThinking = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      streaming.thinkingContent += delta || ''
      upsertMessage(conversation_id, streaming.messageId, {
        thinking_content: streaming.thinkingContent,
      } as any)
    }
  }

  const handleChatTool = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, type, tool_call_id, tool_name, args_json, result_json } =
      data
    const streaming = streamingByConversation.value[conversation_id]

    // Guard: ignore empty tool_call_id events (some providers stream partial tool deltas)
    if (!tool_call_id) {
      debug('tool event ignored (empty tool_call_id)', { conversation_id, request_id, type, tool_name })
      return
    }

    if (!streaming) {
      debug('tool event ignored (no streaming)', { conversation_id, request_id, type, tool_call_id })
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

    if (streaming && streaming.requestId === request_id) {
      if (type === 'call') {
        // Tool call (dedupe by tool_call_id; some providers may emit repeated snapshots)
        const existing = streaming.toolCalls.find((tc) => tc.toolCallId === tool_call_id)
        if (existing) {
          existing.toolName = tool_name
          existing.argsJson = args_json
          // Don't downgrade completed to calling
          if (existing.status !== 'completed') {
            existing.status = 'calling'
          }
        } else {
          streaming.toolCalls.push({
            toolCallId: tool_call_id,
            toolName: tool_name,
            argsJson: args_json,
            status: 'calling',
          })
        }
      } else if (type === 'result') {
        // Tool result (result may arrive before call event)
        const existing = streaming.toolCalls.find((tc) => tc.toolCallId === tool_call_id)
        if (existing) {
          existing.toolName = existing.toolName || tool_name
          existing.resultJson = result_json
          existing.status = 'completed'
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
            mergeCandidate.toolCallId = tool_call_id || mergeCandidate.toolCallId
            mergeCandidate.resultJson = result_json
            mergeCandidate.status = 'completed'
          } else {
            streaming.toolCalls.push({
              toolCallId: tool_call_id,
              toolName: tool_name,
              resultJson: result_json,
              status: 'completed',
            })
          }
        }
      }

      // Mirror tool calls into the message so UI can render them consistently
      upsertMessage(conversation_id, streaming.messageId, {
        tool_calls: JSON.stringify(streaming.toolCalls),
      } as any)
    }
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

      // Refresh messages from backend to get the final persisted state (tokens, tool messages, etc.)
      void loadMessages(conversation_id)

      // Clear streaming state
      delete streamingByConversation.value[conversation_id]
      delete activeRequestByConversation.value[conversation_id]
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

      // Refresh messages from backend to get the cancelled persisted state
      void loadMessages(conversation_id)

      // Clear streaming state
      delete streamingByConversation.value[conversation_id]
      delete activeRequestByConversation.value[conversation_id]
    }
  }

  const handleChatError = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, error_key } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      streaming.status = MessageStatus.ERROR

      // Store error message
      errorByConversation.value[conversation_id] = error_key

      upsertMessage(conversation_id, streaming.messageId, {
        content: streaming.content,
        thinking_content: streaming.thinkingContent,
        tool_calls: JSON.stringify(streaming.toolCalls),
        status: MessageStatus.ERROR,
      } as any)

      // Refresh messages from backend
      void loadMessages(conversation_id)

      // Clear streaming state
      delete streamingByConversation.value[conversation_id]
      delete activeRequestByConversation.value[conversation_id]
    }
  }

  // Helper to extract event data
  const extractEventData = (event: any) => {
    if (!event) return null
    // Wails events may wrap data in an array
    return Array.isArray(event?.data) ? event.data[0] : event?.data ?? event
  }

  // Subscribe to chat events
  let unsubscribers: Array<() => void> = []
  let subscriptionRefCount = 0
  const debugEnabled =
    typeof window !== 'undefined' && window.localStorage?.getItem('debug:chat') === '1'

  const debug = (...args: any[]) => {
    if (debugEnabled) {
      // eslint-disable-next-line no-console
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
      Events.On(ChatEventType.START, (e: any) => {
        debug(ChatEventType.START, extractEventData(e))
        handleChatStart(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.CHUNK, (e: any) => {
        debug(ChatEventType.CHUNK, extractEventData(e))
        handleChatChunk(e)
      })
    )
    unsubscribers.push(
      Events.On(ChatEventType.THINKING, (e: any) => {
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
        // eslint-disable-next-line no-console
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

    // Getters
    getMessages,
    getStreaming,
    isGenerating,

    // Actions
    loadMessages,
    sendMessage,
    editAndResend,
    stopGeneration,
    clearMessages,

    // Event subscription
    subscribe,
    unsubscribe,
  }
})
