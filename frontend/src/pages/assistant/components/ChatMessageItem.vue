<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pencil, Copy, Check, Loader2 } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { MessageStatus, MessageRole, type ToolCallInfo } from '@/stores'
import type { Message } from '@bindings/willchat/internal/services/chat'
import ThinkingBlock from './ThinkingBlock.vue'
import ToolCallBlock from './ToolCallBlock.vue'
import MessageEditor from './MessageEditor.vue'

const props = defineProps<{
  message: Message
  isStreaming?: boolean
  streamingContent?: string
  streamingThinking?: string
  streamingToolCalls?: ToolCallInfo[]
}>()

const emit = defineEmits<{
  edit: [messageId: number, newContent: string]
}>()

const { t } = useI18n()

const isEditing = ref(false)
const copied = ref(false)

// Determine content to display (streaming or final)
const displayContent = computed(() => {
  if (props.isStreaming && props.streamingContent !== undefined) {
    return props.streamingContent
  }
  return props.message.content
})

// Determine thinking content
const thinkingContent = computed(() => {
  if (props.isStreaming && props.streamingThinking !== undefined) {
    return props.streamingThinking
  }
  return props.message.thinking_content ?? ''
})

// Determine tool calls
const toolCalls = computed(() => {
  if (props.isStreaming && props.streamingToolCalls) {
    return props.streamingToolCalls
  }
  // Parse from message if available
  if (props.message.tool_calls && props.message.tool_calls !== '[]') {
    try {
      return JSON.parse(props.message.tool_calls) as ToolCallInfo[]
    } catch {
      return []
    }
  }
  return []
})

const isUser = computed(() => props.message.role === MessageRole.USER)
const isAssistant = computed(() => props.message.role === MessageRole.ASSISTANT)
const isTool = computed(() => props.message.role === MessageRole.TOOL)

const showThinking = computed(() => isAssistant.value && thinkingContent.value)
const showToolCalls = computed(() => isAssistant.value && toolCalls.value.length > 0)
const showStatus = computed(
  () =>
    props.message.status === MessageStatus.ERROR ||
    props.message.status === MessageStatus.CANCELLED
)

const handleCopy = async () => {
  try {
    await navigator.clipboard.writeText(displayContent.value)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    toast.error(t('assistant.chat.copyFailed'))
  }
}

const handleEdit = () => {
  isEditing.value = true
}

const handleSaveEdit = (newContent: string) => {
  isEditing.value = false
  emit('edit', props.message.id, newContent)
}

const handleCancelEdit = () => {
  isEditing.value = false
}
</script>

<template>
  <div
    :class="
      cn('group flex w-full gap-3', isUser ? 'justify-end' : 'justify-start', isTool && 'hidden')
    "
  >
    <!-- Message content container -->
    <div :class="cn('flex max-w-[85%] flex-col gap-2', isUser && 'items-end')">
      <!-- Thinking block (for assistant messages) -->
      <ThinkingBlock v-if="showThinking" :content="thinkingContent" :is-streaming="isStreaming" />

      <!-- Tool calls block (for assistant messages) -->
      <ToolCallBlock
        v-if="showToolCalls"
        :tool-calls="toolCalls"
        :is-streaming="isStreaming"
      />

      <!-- Message bubble -->
      <div
        :class="
          cn(
            'relative rounded-2xl px-4 py-3 text-sm',
            isUser
              ? 'bg-primary text-primary-foreground'
              : 'bg-muted text-foreground dark:bg-zinc-800'
          )
        "
      >
        <!-- Edit mode -->
        <MessageEditor
          v-if="isEditing && isUser"
          :initial-content="message.content"
          @save="handleSaveEdit"
          @cancel="handleCancelEdit"
        />

        <!-- Normal display mode -->
        <template v-else>
          <p class="whitespace-pre-wrap wrap-break-word">{{ displayContent }}</p>

          <!-- Streaming indicator -->
          <span v-if="isStreaming && isAssistant" class="ml-1 inline-block">
            <span class="animate-pulse">▌</span>
          </span>
        </template>

        <!-- Status indicator -->
        <div v-if="showStatus" class="mt-2 flex items-center gap-1 text-xs opacity-70">
          <template v-if="message.status === MessageStatus.ERROR">
            <span class="text-destructive">{{ t('assistant.chat.error') }}</span>
          </template>
          <template v-else-if="message.status === MessageStatus.CANCELLED">
            <span>{{ t('assistant.chat.cancelled') }}</span>
          </template>
        </div>
      </div>

      <!-- Action buttons (visible on hover) -->
      <div
        v-if="!isEditing && !isStreaming"
        :class="
          cn(
            'flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100',
            isUser ? 'justify-end' : 'justify-start'
          )
        "
      >
        <!-- Copy button -->
        <Button
          size="icon"
          variant="ghost"
          class="size-6"
          :title="t('assistant.chat.copy')"
          @click="handleCopy"
        >
          <Check v-if="copied" class="size-3.5 text-green-500" />
          <Copy v-else class="size-3.5 text-muted-foreground" />
        </Button>

        <!-- Edit button (only for user messages) -->
        <Button
          v-if="isUser"
          size="icon"
          variant="ghost"
          class="size-6"
          :title="t('assistant.chat.edit')"
          @click="handleEdit"
        >
          <Pencil class="size-3.5 text-muted-foreground" />
        </Button>
      </div>
    </div>
  </div>
</template>
