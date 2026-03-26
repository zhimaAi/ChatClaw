// Mirror the chat event names locally so this helper stays importable in plain node tests.
export const HISTORY_RUN_CHAT_EVENT_TYPE = {
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

// Keep a short delay so bursty chunk events collapse into a single history refresh.
export const HISTORY_RUN_RELOAD_DELAY_MS = 120

// Terminal events should flush immediately so the history iframe does not lag at the end.
export const HISTORY_RUN_IMMEDIATE_RELOAD_EVENT_NAMES = new Set<string>([
  HISTORY_RUN_CHAT_EVENT_TYPE.COMPLETE,
  HISTORY_RUN_CHAT_EVENT_TYPE.STOPPED,
  HISTORY_RUN_CHAT_EVENT_TYPE.ERROR,
])

// These chat events can change the rendered transcript for a running scheduled-task conversation.
export const HISTORY_RUN_RELOAD_EVENT_NAMES = new Set<string>([
  HISTORY_RUN_CHAT_EVENT_TYPE.USER_MESSAGE,
  HISTORY_RUN_CHAT_EVENT_TYPE.START,
  HISTORY_RUN_CHAT_EVENT_TYPE.CHUNK,
  HISTORY_RUN_CHAT_EVENT_TYPE.THINKING,
  HISTORY_RUN_CHAT_EVENT_TYPE.TOOL,
  HISTORY_RUN_CHAT_EVENT_TYPE.RETRIEVAL,
  HISTORY_RUN_CHAT_EVENT_TYPE.COMPLETE,
  HISTORY_RUN_CHAT_EVENT_TYPE.STOPPED,
  HISTORY_RUN_CHAT_EVENT_TYPE.ERROR,
])

export function extractConversationId(payload: unknown): number | null {
  const conversationId = Number((payload as { conversation_id?: unknown } | null)?.conversation_id)
  if (!Number.isFinite(conversationId) || conversationId <= 0) {
    return null
  }
  return conversationId
}

export function shouldReloadHistoryConversation(
  eventName: string,
  payload: unknown,
  conversationId: number
): boolean {
  if (!Number.isFinite(conversationId) || conversationId <= 0) {
    return false
  }
  if (!HISTORY_RUN_RELOAD_EVENT_NAMES.has(eventName)) {
    return false
  }
  return extractConversationId(payload) === conversationId
}

export interface HistoryRunStreamingSnapshot {
  conversationId: number
  requestId: string
  messageId: number
}

export function extractHistoryRunStreamingSnapshot(
  eventName: string,
  payload: unknown,
  conversationId: number
): HistoryRunStreamingSnapshot | null {
  if (!shouldReloadHistoryConversation(eventName, payload, conversationId)) {
    return null
  }

  const messageId = Number((payload as { message_id?: unknown } | null)?.message_id)
  const requestId = String((payload as { request_id?: unknown } | null)?.request_id ?? '').trim()

  if (!Number.isFinite(messageId) || messageId <= 0 || requestId === '') {
    return null
  }

  return {
    conversationId,
    requestId,
    messageId,
  }
}
