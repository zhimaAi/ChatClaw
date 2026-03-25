<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import AssistantPage from '@/pages/assistant/AssistantPage.vue'
import { ChatEventType, useChatStore } from '@/stores/chat'
import { ChatService } from '@bindings/chatclaw/internal/services/chat'
import {
  extractHistoryRunStreamingSnapshot,
  HISTORY_RUN_IMMEDIATE_RELOAD_EVENT_NAMES,
  HISTORY_RUN_RELOAD_DELAY_MS,
  shouldReloadHistoryConversation,
} from './eventSync'

const QUERY_KEY_CONVERSATION_ID = 'conversationId'
const QUERY_KEY_AGENT_ID = 'agentId'
const INVALID_NUMBER = Number.NaN
const FORWARDED_CHAT_EVENT_TYPE = 'history-run-chat-event'
const FORWARDED_CHAT_STATE_TYPE = 'history-run-chat-state'

function parseNumericQueryParam(key: string): number {
  const value = new URLSearchParams(window.location.search).get(key)?.trim()
  if (!value) return INVALID_NUMBER
  const parsed = Number(value)
  if (!Number.isFinite(parsed) || parsed <= 0) return INVALID_NUMBER
  return parsed
}

const { t } = useI18n()

const initialConversationId = computed(() => parseNumericQueryParam(QUERY_KEY_CONVERSATION_ID))
const initialAgentId = computed(() => {
  const parsed = parseNumericQueryParam(QUERY_KEY_AGENT_ID)
  return Number.isFinite(parsed) ? parsed : null
})
const hasValidConversationId = computed(() => Number.isFinite(initialConversationId.value))
const iframeTabId = computed(() =>
  hasValidConversationId.value ? `history-run-${initialConversationId.value}` : 'history-run-invalid'
)
const chatStore = useChatStore()
let pendingReloadTimer: ReturnType<typeof setTimeout> | null = null

function clearPendingReloadTimer() {
  if (pendingReloadTimer == null) return
  window.clearTimeout(pendingReloadTimer)
  pendingReloadTimer = null
}

function reloadCurrentConversationMessages() {
  if (!hasValidConversationId.value) return
  void chatStore.loadMessages(initialConversationId.value)
}

function scheduleHistoryConversationReload(eventName: string, payload: unknown) {
  if (!hasValidConversationId.value) return
  if (!shouldReloadHistoryConversation(eventName, payload, initialConversationId.value)) return

  if (HISTORY_RUN_IMMEDIATE_RELOAD_EVENT_NAMES.has(eventName)) {
    clearPendingReloadTimer()
    reloadCurrentConversationMessages()
    return
  }

  if (pendingReloadTimer != null) return
  pendingReloadTimer = setTimeout(() => {
    pendingReloadTimer = null
    reloadCurrentConversationMessages()
  }, HISTORY_RUN_RELOAD_DELAY_MS)
}

async function restoreStreamingStateFromPayload(eventName: string, payload: unknown) {
  if (!hasValidConversationId.value) return

  const snapshot = extractHistoryRunStreamingSnapshot(
    eventName,
    payload,
    initialConversationId.value
  )
  if (!snapshot) return

  const [content, exists] = await ChatService.GetGenerationContent(
    snapshot.conversationId,
    snapshot.requestId
  )
  if (!exists) return

  chatStore.restoreStreamingSnapshot(
    snapshot.conversationId,
    snapshot.requestId,
    snapshot.messageId,
    content || ''
  )
}

const handleForwardedChatEvent = (event: MessageEvent) => {
  if (event.origin !== window.location.origin) return
  const payload = event.data
  if (!payload || payload.type !== FORWARDED_CHAT_EVENT_TYPE) return
  if (typeof payload.eventName !== 'string') return

  const knownEventNames = new Set<string>(Object.values(ChatEventType))
  if (!knownEventNames.has(payload.eventName)) return

  chatStore.handleForwardedEvent(payload.eventName, payload.payload)
  scheduleHistoryConversationReload(payload.eventName, payload.payload)
  void restoreStreamingStateFromPayload(payload.eventName, payload.payload)
}

const handleForwardedChatState = (event: MessageEvent) => {
  if (event.origin !== window.location.origin) return
  const payload = event.data
  if (!payload || payload.type !== FORWARDED_CHAT_STATE_TYPE) return
  if (typeof payload.eventName !== 'string') return

  void restoreStreamingStateFromPayload(payload.eventName, payload.payload)
}

onMounted(() => {
  window.addEventListener('message', handleForwardedChatEvent)
  window.addEventListener('message', handleForwardedChatState)
})

onUnmounted(() => {
  window.removeEventListener('message', handleForwardedChatEvent)
  window.removeEventListener('message', handleForwardedChatState)
  clearPendingReloadTimer()
})
</script>

<template>
  <div class="flex h-full w-full overflow-hidden bg-background">
    <div
      v-if="!hasValidConversationId"
      class="flex h-full w-full items-center justify-center px-6 text-center text-sm text-muted-foreground"
    >
      {{ t('scheduledTasks.conversationEmpty') }}
    </div>
    <AssistantPage
      v-else
      :key="iframeTabId"
      :tab-id="iframeTabId"
      mode="history-iframe"
      :initial-conversation-id="initialConversationId"
      :initial-agent-id="initialAgentId"
    />
  </div>
</template>
