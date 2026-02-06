<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pencil, Copy, Check, AlertCircle, ChevronDown, ChevronUp, SendHorizontal, Type } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { MessageStatus, MessageRole, type ToolCallInfo, type MessageSegment } from '@/stores'
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
  segments?: MessageSegment[] // Ordered segments for interleaved display
  errorKey?: string // Specific error key for more informative error messages
  errorDetail?: string // Actual error message detail for user display
  // Snap mode props
  mode?: 'main' | 'snap'
  hasAttachedTarget?: boolean
  showAiSendButton?: boolean
  showAiEditButton?: boolean
}>()

const emit = defineEmits<{
  edit: [messageId: number, newContent: string]
  snapSendAndTrigger: [content: string]
  snapSendToEdit: [content: string]
  snapCopy: [content: string]
}>()

const { t } = useI18n()

const isEditing = ref(false)
const copied = ref(false)
const showErrorDetail = ref(false)

// Determine content to display (streaming or final) — used only for non-segment fallback & copy
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

// Determine tool calls (flat list, used for non-segment fallback)
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
const isSnapMode = computed(() => props.mode === 'snap')

const showThinking = computed(() => isAssistant.value && thinkingContent.value)
const showStatus = computed(
  () =>
    props.message.status === MessageStatus.ERROR || props.message.status === MessageStatus.CANCELLED
)

// Compute display segments: from props (streaming/persisted) or fallback from message data
const displaySegments = computed((): MessageSegment[] => {
  // Priority: provided segments (streaming or persisted)
  if (props.segments && props.segments.length > 0) {
    return props.segments
  }
  // Fallback: construct from message data (non-interleaved)
  if (!isAssistant.value) return []
  const segs: MessageSegment[] = []
  const content = displayContent.value
  if (content) {
    segs.push({ type: 'content', content })
  }
  if (toolCalls.value.length > 0) {
    segs.push({ type: 'tools', toolCalls: toolCalls.value })
  }
  return segs
})

// Check if a given index is the last content segment (for cursor display)
const isLastContentSegment = (idx: number): boolean => {
  for (let i = displaySegments.value.length - 1; i >= 0; i--) {
    if (displaySegments.value[i].type === 'content') return i === idx
  }
  return false
}

// Error message key mapping
const errorMessageKey = computed(() => {
  if (props.message.status !== MessageStatus.ERROR) return ''
  switch (props.errorKey) {
    case 'error.max_iterations_exceeded':
      return 'assistant.chat.errorMaxIterations'
    case 'error.chat_stream_failed':
      return 'assistant.chat.errorStream'
    default:
      return 'assistant.chat.error'
  }
})

const handleCopy = async () => {
  try {
    await navigator.clipboard.writeText(displayContent.value)
    copied.value = true
    window.setTimeout(() => {
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
      cn('group flex min-w-0 w-full gap-3 overflow-hidden', isUser ? 'justify-end' : 'justify-start', isTool && 'hidden')
    "
  >
    <!-- Message content container -->
    <div
      :class="cn('flex min-w-0 max-w-[85%] w-full flex-col gap-1.5', isUser ? 'items-end' : 'items-start')"
    >
      <!-- Thinking block (for assistant messages) -->
      <ThinkingBlock v-if="showThinking" :content="thinkingContent" :is-streaming="isStreaming" />

      <!-- Assistant messages: interleaved segments (content ↔ tool calls) -->
      <template v-if="isAssistant">
        <template v-for="(segment, idx) in displaySegments" :key="idx">
          <!-- Content segment -->
          <MarkdownRenderer
            v-if="segment.type === 'content' && segment.content"
            :content="segment.content"
            :is-streaming="!!isStreaming && isLastContentSegment(idx)"
            class="min-w-0 wrap-break-word"
          />
          <!-- Tools segment -->
          <ToolCallBlock
            v-if="segment.type === 'tools'"
            :tool-calls="segment.toolCalls"
            :is-streaming="isStreaming"
          />
        </template>

        <!-- Streaming cursor when no content segments yet (e.g. agent starts with tool calls) -->
        <MarkdownRenderer
          v-if="isStreaming && displaySegments.length === 0"
          content=""
          :is-streaming="true"
          class="min-w-0 wrap-break-word"
        />

        <!-- Status indicator (after all segments) -->
        <div v-if="showStatus" class="mt-2 text-xs">
          <template v-if="message.status === MessageStatus.ERROR">
            <div
              class="inline-flex items-center gap-1.5 rounded-md border border-border/50 bg-muted/50 px-2 py-1 text-muted-foreground"
            >
              <AlertCircle class="size-3.5 shrink-0" />
              <span>{{ t(errorMessageKey) }}</span>
              <button
                v-if="errorDetail"
                class="ml-1 flex items-center gap-0.5 text-muted-foreground/70 hover:text-muted-foreground transition-colors"
                @click="showErrorDetail = !showErrorDetail"
              >
                <span class="text-[10px]">{{ showErrorDetail ? t('common.hide') : t('common.detail') }}</span>
                <ChevronDown v-if="!showErrorDetail" class="size-3" />
                <ChevronUp v-else class="size-3" />
              </button>
            </div>
            <!-- Error detail (collapsible) -->
            <div
              v-if="errorDetail && showErrorDetail"
              class="mt-1.5 rounded-md border border-border/30 bg-muted/30 px-2.5 py-1.5 font-mono text-[11px] text-muted-foreground/80"
            >
              {{ errorDetail }}
            </div>
          </template>
          <template v-else-if="message.status === MessageStatus.CANCELLED">
            <div
              class="inline-flex items-center gap-1.5 rounded-md border border-border/50 bg-muted/50 px-2 py-1 text-muted-foreground"
            >
              <span>{{ t('assistant.chat.cancelled') }}</span>
            </div>
          </template>
        </div>
      </template>

      <!-- User message bubble -->
      <div
        v-else-if="isUser"
        class="relative min-w-0 max-w-full text-sm rounded-2xl border border-border/50 bg-muted px-4 py-3 text-foreground"
      >
        <!-- Edit mode -->
        <MessageEditor
          v-if="isEditing"
          :initial-content="message.content"
          @save="handleSaveEdit"
          @cancel="handleCancelEdit"
        />
        <!-- Normal display mode -->
        <p v-else class="whitespace-pre-wrap wrap-break-word">{{ displayContent }}</p>
      </div>

      <!-- Other roles: plain text -->
      <div v-else class="relative text-sm bg-transparent px-0 py-0 text-foreground">
        <p class="whitespace-pre-wrap wrap-break-word">{{ displayContent }}</p>
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
        <!-- Snap mode: Send and trigger button (assistant messages only) -->
        <Button
          v-if="isSnapMode && isAssistant && showAiSendButton && hasAttachedTarget"
          size="icon"
          variant="ghost"
          class="size-6"
          :title="t('winsnap.actions.sendAndTrigger')"
          @click="emit('snapSendAndTrigger', displayContent)"
        >
          <SendHorizontal class="size-3.5 text-muted-foreground" />
        </Button>

        <!-- Snap mode: Send to edit button (assistant messages only) -->
        <Button
          v-if="isSnapMode && isAssistant && showAiEditButton && hasAttachedTarget"
          size="icon"
          variant="ghost"
          class="size-6"
          :title="t('winsnap.actions.sendToEdit')"
          @click="emit('snapSendToEdit', displayContent)"
        >
          <Type class="size-3.5 text-muted-foreground" />
        </Button>

        <!-- Copy button -->
        <Button
          size="icon"
          variant="ghost"
          class="size-6"
          :title="t('assistant.chat.copy')"
          @click="isSnapMode && isAssistant ? emit('snapCopy', displayContent) : handleCopy()"
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
