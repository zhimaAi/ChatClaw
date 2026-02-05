<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, Wrench, Loader2, Check, X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import type { ToolCallInfo } from '@/stores'

const props = defineProps<{
  toolCalls: ToolCallInfo[]
  isStreaming?: boolean
}>()

const { t } = useI18n()

const expandedCalls = ref<Set<string>>(new Set())

const toggleExpand = (toolCallId: string) => {
  if (expandedCalls.value.has(toolCallId)) {
    expandedCalls.value.delete(toolCallId)
  } else {
    expandedCalls.value.add(toolCallId)
  }
}

const isExpanded = (toolCallId: string) => expandedCalls.value.has(toolCallId)

const formatJson = (json?: string) => {
  if (!json) return ''
  try {
    return JSON.stringify(JSON.parse(json), null, 2)
  } catch {
    return json
  }
}

const getToolDisplayName = (toolName: string) => {
  // Map tool IDs to display names
  const nameMap: Record<string, string> = {
    calculator: t('tools.calculator.name'),
    duckduckgo_search: t('tools.duckduckgo.name'),
  }
  return nameMap[toolName] ?? toolName
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div
      v-for="toolCall in toolCalls"
      :key="toolCall.toolCallId"
      class="flex flex-col gap-1 rounded-lg border border-border/50 bg-muted/30 px-3 py-2 text-sm dark:bg-zinc-900/50"
    >
      <!-- Tool header -->
      <button
        class="flex items-center gap-2 text-left text-muted-foreground hover:text-foreground"
        @click="toggleExpand(toolCall.toolCallId)"
      >
        <Wrench class="size-4" />
        <span class="flex-1 text-xs font-medium">
          {{ getToolDisplayName(toolCall.toolName) }}
        </span>

        <!-- Status indicator -->
        <span class="flex items-center gap-1 text-xs">
          <template v-if="toolCall.status === 'calling'">
            <Loader2 class="size-3 animate-spin" />
            <span class="opacity-70">{{ t('assistant.chat.toolCalling') }}</span>
          </template>
          <template v-else-if="toolCall.status === 'completed'">
            <Check class="size-3 text-green-500" />
            <span class="opacity-70">{{ t('assistant.chat.toolCompleted') }}</span>
          </template>
          <template v-else-if="toolCall.status === 'error'">
            <X class="size-3 text-destructive" />
            <span class="opacity-70">{{ t('assistant.chat.toolError') }}</span>
          </template>
        </span>

        <ChevronDown
          :class="cn('size-4 transition-transform', isExpanded(toolCall.toolCallId) && 'rotate-180')"
        />
      </button>

      <!-- Tool details -->
      <div v-if="isExpanded(toolCall.toolCallId)" class="mt-1 space-y-2 text-xs">
        <!-- Arguments -->
        <div v-if="toolCall.argsJson" class="space-y-1">
          <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolArgs') }}</div>
          <pre
            class="overflow-x-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
          ><code>{{ formatJson(toolCall.argsJson) }}</code></pre>
        </div>

        <!-- Result -->
        <div v-if="toolCall.resultJson" class="space-y-1">
          <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolResult') }}</div>
          <pre
            class="max-h-48 overflow-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
          ><code>{{ formatJson(toolCall.resultJson) }}</code></pre>
        </div>
      </div>
    </div>
  </div>
</template>
