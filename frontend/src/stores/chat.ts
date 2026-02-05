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

  // Send a new message
  const sendMessage = async (conversationId: number, content: string, tabId: string) => {
    if (conversationId <= 0 || !content.trim()) return null

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
  }

  const handleChatChunk = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      streaming.content += delta || ''
    }
  }

  const handleChatThinking = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, delta } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      streaming.thinkingContent += delta || ''
    }
  }

  const handleChatTool = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id, type, tool_call_id, tool_name, args_json, result_json } =
      data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      if (type === 'call') {
        // New tool call
        streaming.toolCalls.push({
          toolCallId: tool_call_id,
          toolName: tool_name,
          argsJson: args_json,
          status: 'calling',
        })
      } else if (type === 'result') {
        // Tool result
        const toolCall = streaming.toolCalls.find((tc) => tc.toolCallId === tool_call_id)
        if (toolCall) {
          toolCall.resultJson = result_json
          toolCall.status = 'completed'
        }
      }
    }
  }

  const handleChatComplete = (event: any) => {
    const data = extractEventData(event)
    if (!data) return

    const { conversation_id, request_id } = data
    const streaming = streamingByConversation.value[conversation_id]

    if (streaming && streaming.requestId === request_id) {
      // Refresh messages from backend to get the final state
      loadMessages(conversation_id)

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
      // Refresh messages from backend to get the cancelled state
      loadMessages(conversation_id)

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

      // Refresh messages from backend
      loadMessages(conversation_id)

      // Clear streaming state after a delay
      setTimeout(() => {
        delete streamingByConversation.value[conversation_id]
        delete activeRequestByConversation.value[conversation_id]
      }, 100)
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

  const subscribe = () => {
    unsubscribers.push(Events.On(ChatEventType.START, handleChatStart))
    unsubscribers.push(Events.On(ChatEventType.CHUNK, handleChatChunk))
    unsubscribers.push(Events.On(ChatEventType.THINKING, handleChatThinking))
    unsubscribers.push(Events.On(ChatEventType.TOOL, handleChatTool))
    unsubscribers.push(Events.On(ChatEventType.COMPLETE, handleChatComplete))
    unsubscribers.push(Events.On(ChatEventType.STOPPED, handleChatStopped))
    unsubscribers.push(Events.On(ChatEventType.ERROR, handleChatError))
  }

  const unsubscribe = () => {
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
