<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores'
import type { Message } from '@bindings/willchat/internal/services/chat'
import ChatMessageItem from './ChatMessageItem.vue'

const props = defineProps<{
  conversationId: number
  tabId: string
  mode?: 'main' | 'snap'
  hasAttachedTarget?: boolean
  showAiSendButton?: boolean
  showAiEditButton?: boolean
}>()

const emit = defineEmits<{
  editMessage: [messageId: number, newContent: string]
  snapSendAndTrigger: [content: string]
  snapSendToEdit: [content: string]
  snapCopy: [content: string]
}>()

const { t } = useI18n()
const chatStore = useChatStore()

const scrollContainerRef = ref<HTMLElement | null>(null)
const shouldAutoScroll = ref(true)
const isFirstMessage = ref(true) // Track if this is the first message for special handling

// Get messages for this conversation
const allMessages = computed(() => chatStore.getMessages(props.conversationId).value)

// Build a map of tool results by tool_call_id (from tool messages)
const toolResultsMap = computed(() => {
  const map: Record<string, { content: string; toolName: string }> = {}
  for (const msg of allMessages.value) {
    if (msg.role === 'tool' && msg.tool_call_id) {
      map[msg.tool_call_id] = {
        content: msg.content,
        toolName: msg.tool_call_name || '',
      }
    }
  }
  return map
})

// Filter out tool messages (they'll be displayed inline with assistant messages)
const messages = computed(() => {
  return allMessages.value.filter((msg) => msg.role !== 'tool')
})

// Get tool results for a specific message's tool calls
const getToolResultsForMessage = (msg: Message): Record<string, string> => {
  if (msg.role !== 'assistant' || !msg.tool_calls || msg.tool_calls === '[]') {
    return {}
  }

  try {
    const parsed = JSON.parse(msg.tool_calls) as Array<{
      ID?: string
      id?: string
      toolCallId?: string
    }>
    const results: Record<string, string> = {}

    for (const tc of parsed) {
      const toolCallId = tc.ID || tc.id || tc.toolCallId
      if (toolCallId && toolResultsMap.value[toolCallId]) {
        results[toolCallId] = toolResultsMap.value[toolCallId].content
      }
    }

    return results
  } catch {
    return {}
  }
}

// Get streaming state for this conversation
const streaming = computed(() => chatStore.getStreaming(props.conversationId).value)

// Check if generating
const isGenerating = computed(() => chatStore.isGenerating(props.conversationId).value)

// Scroll to bottom with retries for first message
const scrollToBottom = (isFirstMsg = false) => {
  if (!scrollContainerRef.value || !shouldAutoScroll.value) return

  const performScroll = () => {
    if (scrollContainerRef.value) {
      scrollContainerRef.value.scrollTop = scrollContainerRef.value.scrollHeight
    }
  }

  if (isFirstMsg) {
    // For first message, use aggressive retries to ensure content is fully rendered and positioned
    nextTick(() => {
      requestAnimationFrame(() => {
        performScroll()
        // Multiple retries with increasing delays to catch all rendering phases
        setTimeout(performScroll, 50)
        setTimeout(performScroll, 150)
        setTimeout(performScroll, 300)
      })
    })
  } else {
    // For subsequent messages, fewer retries
    nextTick(() => {
      requestAnimationFrame(() => {
        performScroll()
        setTimeout(performScroll, 50)
      })
    })
  }
}

// Handle scroll to detect if user has scrolled up
const handleScroll = () => {
  if (!scrollContainerRef.value) return
  const { scrollTop, scrollHeight, clientHeight } = scrollContainerRef.value
  // Auto-scroll if near bottom (within 100px)
  shouldAutoScroll.value = scrollHeight - scrollTop - clientHeight < 100
}

// Handle message edit
const handleEdit = (messageId: number, newContent: string) => {
  emit('editMessage', messageId, newContent)
}

// Watch for new messages and scroll
watch(
  () => messages.value.length,
  (newLen, oldLen) => {
    // Detect first message in a new conversation
    const isFirst = oldLen === 0 && newLen === 1
    if (isFirst) {
      isFirstMessage.value = true
    }
    scrollToBottom(isFirst)
  }
)

// Watch for streaming content changes and scroll
watch(
  () => streaming.value?.content,
  () => {
    // First message needs special handling
    scrollToBottom(isFirstMessage.value)
    // After first chunk, no longer treat as first message
    if (isFirstMessage.value && streaming.value?.content) {
      // Give it one more retry after first chunk to ensure stability
      setTimeout(() => {
        isFirstMessage.value = false
      }, 150)
    }
  }
)

// Watch for tool call updates and scroll
watch(
  () =>
    streaming.value?.toolCalls
      ?.map((tc) => `${tc.toolCallId}:${tc.status}:${tc.resultJson ? 1 : 0}`)
      .join('|'),
  () => {
    scrollToBottom()
  }
)

// When a new streaming response starts, force scroll to bottom
watch(
  () => streaming.value?.messageId,
  (newId, oldId) => {
    if (newId && newId !== oldId) {
      shouldAutoScroll.value = true
      scrollToBottom()
    }
  }
)

// Load messages when conversation changes
watch(
  () => props.conversationId,
  async (newId, oldId) => {
    if (newId > 0) {
      await chatStore.loadMessages(newId)
      // When opening a conversation, jump to bottom by default.
      shouldAutoScroll.value = true
      // Reset first message flag when switching conversations
      isFirstMessage.value = newId !== oldId
      scrollToBottom()
    }
  },
  { immediate: true }
)

// Note: Chat event subscription is handled at AssistantPage level
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-col overflow-hidden">
    <!-- Messages container -->
    <div
      ref="scrollContainerRef"
      class="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-6 py-4"
      @scroll="handleScroll"
    >
      <div class="mx-auto flex min-w-0 max-w-[800px] flex-col gap-1">
        <!-- Existing messages -->
        <ChatMessageItem
          v-for="msg in messages"
          :key="msg.id"
          :message="msg"
          :tool-results="getToolResultsForMessage(msg)"
          :is-streaming="!!(streaming && msg.id === streaming.messageId)"
          :streaming-content="
            streaming && msg.id === streaming.messageId ? streaming.content : undefined
          "
          :streaming-thinking="
            streaming && msg.id === streaming.messageId ? streaming.thinkingContent : undefined
          "
          :streaming-tool-calls="
            streaming && msg.id === streaming.messageId ? streaming.toolCalls : undefined
          "
          :segments="
            streaming && msg.id === streaming.messageId
              ? streaming.segments
              : chatStore.segmentsByMessage[msg.id]
          "
          :error-key="chatStore.errorKeyByMessage[msg.id]"
          :error-detail="chatStore.errorDetailByMessage[msg.id]"
          :mode="mode"
          :has-attached-target="hasAttachedTarget"
          :show-ai-send-button="showAiSendButton"
          :show-ai-edit-button="showAiEditButton"
          @edit="handleEdit"
          @snap-send-and-trigger="(content) => emit('snapSendAndTrigger', content)"
          @snap-send-to-edit="(content) => emit('snapSendToEdit', content)"
          @snap-copy="(content) => emit('snapCopy', content)"
        />

        <!-- Streaming message fallback (should not happen, but keep UI resilient) -->
        <ChatMessageItem
          v-if="streaming && !messages.some((m) => m.id === streaming.messageId)"
          :message="({
            id: streaming.messageId,
            conversation_id: conversationId,
            role: 'assistant',
            content: streaming.content,
            status: streaming.status,
            thinking_content: streaming.thinkingContent,
            input_tokens: 0,
            output_tokens: 0,
            created_at: null,
            updated_at: null,
          } as Message)"
          :is-streaming="true"
          :streaming-content="streaming.content"
          :streaming-thinking="streaming.thinkingContent"
          :streaming-tool-calls="streaming.toolCalls"
          :segments="streaming.segments"
          :mode="mode"
          :has-attached-target="hasAttachedTarget"
          :show-ai-send-button="showAiSendButton"
          :show-ai-edit-button="showAiEditButton"
          @snap-send-and-trigger="(content) => emit('snapSendAndTrigger', content)"
          @snap-send-to-edit="(content) => emit('snapSendToEdit', content)"
          @snap-copy="(content) => emit('snapCopy', content)"
        />

        <!-- Bottom spacer: keep distance from input box when auto-scrolling -->
        <!-- Use smaller spacing to prevent first message from being pushed out of view -->
        <div aria-hidden="true" class="h-8 shrink-0" />
      </div>
    </div>
  </div>
</template>
