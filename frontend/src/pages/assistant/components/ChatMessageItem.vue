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
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'

const props = defineProps<{
  message: Message
  isStreaming?: boolean
  streamingContent?: string
  streamingThinking?: string
  streamingToolCalls?: ToolCallInfo[]
  toolResults?: Record<string, string> // Map of toolCallId -> result
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

// Backend tool call format (OpenAI-compatible): usually uses lower-case keys.
type BackendToolCall =
  | {
      id: string
      function: {
        name: string
        arguments: string
      }
    }
  | {
      ID: string
      Function: {
        Name: string
        Arguments: string
      }
    }

// Determine tool calls
const toolCalls = computed(() => {
  if (props.isStreaming && props.streamingToolCalls) {
    return props.streamingToolCalls
  }
  // Parse from message if available
  if (props.message.tool_calls && props.message.tool_calls !== '[]') {
    try {
      const parsed = JSON.parse(props.message.tool_calls)
      // Check if it's backend format (schema.ToolCall) or frontend format (ToolCallInfo)
      if (Array.isArray(parsed) && parsed.length > 0) {
        const first = parsed[0] as any
        // Backend format detection: {id, function:{name,arguments}} or legacy {ID, Function:{...}}
        const isBackendLower = typeof first?.id === 'string' && typeof first?.function === 'object'
        const isBackendUpper = 'ID' in first && 'Function' in first
        if (isBackendLower || isBackendUpper) {
          // Convert backend format to frontend format and merge tool results
          return (parsed as BackendToolCall[]).map((tc: any) => {
            const toolCallId = tc.id ?? tc.ID
            const fn = tc.function ?? tc.Function
            const toolName = fn?.name ?? fn?.Name ?? ''
            const argsJson = fn?.arguments ?? fn?.Arguments ?? ''
            return {
              toolCallId,
              toolName,
              argsJson,
              resultJson: props.toolResults?.[toolCallId],
              status: 'completed' as const,
            }
          })
        }
        // Otherwise it's already frontend format, but still merge results
        return (parsed as ToolCallInfo[]).map((tc) => ({
          ...tc,
          resultJson: tc.resultJson || props.toolResults?.[tc.toolCallId],
        }))
      }
      return []
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
    <div
      :class="
        cn('flex max-w-[85%] w-full flex-col gap-1.5', isUser ? 'items-end' : 'items-start')
      "
    >
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
            'relative text-sm',
            isUser
              ? 'rounded-2xl border border-border/50 bg-muted px-4 py-3 text-foreground'
              : 'bg-transparent px-0 py-0 text-foreground'
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
          <!-- User messages: plain text -->
          <p v-if="isUser" class="whitespace-pre-wrap wrap-break-word">{{ displayContent }}</p>

          <!-- Assistant messages: markdown rendered -->
          <template v-else-if="isAssistant">
            <MarkdownRenderer
              v-if="displayContent"
              :content="displayContent"
              class="wrap-break-word"
            />
          </template>

          <!-- Other roles: plain text -->
          <p v-else class="whitespace-pre-wrap wrap-break-word">{{ displayContent }}</p>
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
            isAssistant && '-mt-1',
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
