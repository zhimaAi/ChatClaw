<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { StopCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useChatStore, type StreamingMessageState } from '@/stores'
import type { Message } from '@bindings/willchat/internal/services/chat'
import ChatMessageItem from './ChatMessageItem.vue'

const props = defineProps<{
  conversationId: number
  tabId: string
}>()

const emit = defineEmits<{
  editMessage: [messageId: number, newContent: string]
}>()

const { t } = useI18n()
const chatStore = useChatStore()

const scrollContainerRef = ref<HTMLElement | null>(null)
const shouldAutoScroll = ref(true)

// Get messages for this conversation
const messages = computed(() => chatStore.getMessages(props.conversationId).value)

// Get streaming state for this conversation
const streaming = computed(() => chatStore.getStreaming(props.conversationId).value)

// Check if generating
const isGenerating = computed(() => chatStore.isGenerating(props.conversationId).value)

// Scroll to bottom
const scrollToBottom = () => {
  if (scrollContainerRef.value && shouldAutoScroll.value) {
    nextTick(() => {
      if (scrollContainerRef.value) {
        scrollContainerRef.value.scrollTop = scrollContainerRef.value.scrollHeight
      }
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

// Handle stop generation
const handleStop = () => {
  chatStore.stopGeneration(props.conversationId)
}

// Handle message edit
const handleEdit = (messageId: number, newContent: string) => {
  emit('editMessage', messageId, newContent)
}

// Watch for new messages and scroll
watch(
  () => messages.value.length,
  () => {
    scrollToBottom()
  }
)

// Watch for streaming content changes and scroll
watch(
  () => streaming.value?.content,
  () => {
    scrollToBottom()
  }
)

// Load messages when conversation changes
watch(
  () => props.conversationId,
  (newId) => {
    if (newId > 0) {
      chatStore.loadMessages(newId)
    }
  },
  { immediate: true }
)

// Note: Chat event subscription is handled at AssistantPage level
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Messages container -->
    <div
      ref="scrollContainerRef"
      class="flex-1 overflow-auto px-6 py-4"
      @scroll="handleScroll"
    >
      <div class="mx-auto flex max-w-[800px] flex-col gap-4">
        <!-- Existing messages -->
        <ChatMessageItem
          v-for="msg in messages"
          :key="msg.id"
          :message="msg"
          @edit="handleEdit"
        />

        <!-- Streaming message (if any) -->
        <ChatMessageItem
          v-if="streaming"
          :message="{
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
          }"
          :is-streaming="true"
          :streaming-content="streaming.content"
          :streaming-thinking="streaming.thinkingContent"
          :streaming-tool-calls="streaming.toolCalls"
        />
      </div>
    </div>

    <!-- Stop button (shown when generating) -->
    <div v-if="isGenerating" class="flex justify-center pb-2">
      <Button
        variant="outline"
        size="sm"
        class="gap-2"
        @click="handleStop"
      >
        <StopCircle class="size-4" />
        {{ t('assistant.chat.stop') }}
      </Button>
    </div>
  </div>
</template>
