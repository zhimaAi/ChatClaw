const STREAMING_STATUS = 'streaming'

export interface StreamingBootstrapState {
  messageId: number
  requestId: string
  content: string
  thinkingContent: string
  toolCalls: any[]
  segments: any[]
  status: string
}

export function buildRecoveredStreamingState(payload: {
  message_id?: unknown
  request_id?: unknown
}): StreamingBootstrapState | null {
  const messageId = Number(payload.message_id)
  const requestId = String(payload.request_id ?? '').trim()

  if (!Number.isFinite(messageId) || messageId <= 0 || requestId === '') {
    return null
  }

  return {
    messageId,
    requestId,
    content: '',
    thinkingContent: '',
    toolCalls: [],
    segments: [],
    status: STREAMING_STATUS,
  }
}
