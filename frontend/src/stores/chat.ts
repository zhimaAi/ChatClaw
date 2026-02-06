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

// Message segment for interleaved content/tool-call display (ReAct paradigm)
export type MessageSegment =
  | { type: 'content'; content: string }
  | { type: 'tools'; toolCalls: ToolCallInfo[] }

// Streaming message state (for the currently generating message)
export interface StreamingMessageState {
  messageId: number
  requestId: string
  content: string
  thinkingContent: string
  toolCalls: ToolCallInfo[]
  segments: MessageSegment[] // Ordered segments for interleaved rendering
  status: string
}

// Auto-decrementing counter for local optimistic message IDs (always negative to avoid collision with backend IDs)
let localMessageCounter = 0

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
      const messages = await ChatService.GetMessages(conversationId)
      messagesByConversation.value[conversationId] = messages ?? []

      // Parse and restore segments from backend for each assistant message
      for (const msg of messages ?? []) {
        if (msg.role === 'assistant' && msg.segments) {
          try {
            const rawSegments = JSON.parse(msg.segments) as Array<{
              type: string
              content?: string
              tool_call_ids?: string[]
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
                if (seg.type === 'content') {
                  return { type: 'content' as const, content: seg.content || '' }
                } else if (seg.type === 'tools') {
                  const toolCalls: ToolCallInfo[] = (seg.tool_call_ids || [])
                    .map((id) => toolCallMap.get(id))
                    .filter((tc): tc is ToolCallInfo => tc !== undefined)
                  return { type: 'tools' as const, toolCalls }
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

  // Send a new message
  const sendMessage = async (conversationId: number, content: string, tabId: string) => {
    if (conversationId <= 0 || !content.trim()) return null

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

  const handleChatChunk = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      const chunk = delta || ''
      streaming.content += chunk

      // Track segments: append to last content segment or start a new one
      if (chunk) {
        const lastSeg = streaming.segments[streaming.segments.length - 1]
        if (lastSeg && lastSeg.type === 'content') {
          lastSeg.content += chunk
        } else {
          streaming.segments.push({ type: 'content', content: chunk })
        }
      }

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

    // Helper: add a new tool call to segments
    const addToolCallToSegments = (toolCall: ToolCallInfo) => {
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
          } else {
            const newToolCall: ToolCallInfo = {
              toolCallId: tool_call_id,
              toolName: tool_name,
              resultJson: result_json,
              status: 'completed',
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

      // Clear streaming state first
      delete streamingByConversation.value[conversation_id]
      delete activeRequestByConversation.value[conversation_id]

      // Refresh messages from backend - segments will be loaded from backend
      void loadMessages(conversation_id)
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

      // Clear streaming state first
      delete streamingByConversation.value[conversation_id]
      delete activeRequestByConversation.value[conversation_id]

      // Refresh messages from backend - segments will be loaded from backend
      void loadMessages(conversation_id)
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

      // For errors, persist streaming segments locally since we don't call loadMessages.
      // Deep-clone to detach from reactive streaming state.
      segmentsByMessage.value[streaming.messageId] = streaming.segments.map((seg) => {
        if (seg.type === 'content') {
          return { type: 'content' as const, content: seg.content }
        }
        return { type: 'tools' as const, toolCalls: seg.toolCalls.map((tc) => ({ ...tc })) }
      })

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
    editAndResend,
    stopGeneration,
    clearMessages,

    // Event subscription
    subscribe,
    unsubscribe,
  }
})
