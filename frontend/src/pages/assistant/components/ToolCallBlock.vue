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

const safeParse = (json?: string): any | null => {
  if (!json) return null
  try {
    return JSON.parse(json)
  } catch {
    return null
  }
}

type DuckDuckGoResult = {
  message?: string
  results?: Array<{
    title?: string
    url?: string
    summary?: string
  }>
}

const getDuckDuckGoResult = (json?: string): DuckDuckGoResult | null => {
  const obj = safeParse(json)
  if (!obj || typeof obj !== 'object') return null
  if (!Array.isArray((obj as any).results)) return null
  return obj as DuckDuckGoResult
}

const getQueryFromArgs = (argsJson?: string): string => {
  const obj = safeParse(argsJson)
  const q = obj?.query
  return typeof q === 'string' ? q : ''
}
</script>

<template>
  <div class="w-full max-w-[560px] flex flex-col gap-2">
    <div
      v-for="toolCall in toolCalls"
      :key="toolCall.toolCallId"
      class="w-full min-w-0 flex flex-col gap-1 rounded-lg border border-border/50 bg-muted/30 px-3 py-2 text-sm dark:bg-zinc-900/50"
    >
      <!-- Tool header -->
      <button
        class="flex w-full min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground"
        @click="toggleExpand(toolCall.toolCallId)"
      >
        <Wrench class="size-4" />
        <span class="min-w-0 flex-1 truncate text-xs font-medium">
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
        <!-- Friendly result view for DuckDuckGo -->
        <template
          v-if="toolCall.toolName === 'duckduckgo_search' && getDuckDuckGoResult(toolCall.resultJson)"
        >
          <div class="space-y-1">
            <div class="font-medium text-muted-foreground">
              {{ t('assistant.chat.toolResult') }}
            </div>
            <div class="text-muted-foreground/80">
              <span v-if="getQueryFromArgs(toolCall.argsJson)">
                查询：{{ getQueryFromArgs(toolCall.argsJson) }}
              </span>
            </div>
          </div>

          <div class="space-y-2">
            <div
              v-for="(r, idx) in getDuckDuckGoResult(toolCall.resultJson)?.results"
              :key="(r?.url || '') + idx"
              class="rounded-md border border-border/50 bg-background/30 p-2"
            >
              <a
                v-if="r?.url"
                :href="r.url"
                target="_blank"
                rel="noopener noreferrer"
                class="block truncate font-medium text-foreground underline decoration-border/60 hover:decoration-border"
              >
                {{ r?.title || r.url }}
              </a>
              <div v-else class="block truncate font-medium text-foreground">
                {{ r?.title || '-' }}
              </div>
              <p v-if="r?.summary" class="mt-1 line-clamp-3 text-muted-foreground">
                {{ r.summary }}
              </p>
            </div>
          </div>

          <details class="rounded border border-border/50 bg-background/20 p-2">
            <summary class="cursor-pointer select-none text-muted-foreground">
              原始 JSON
            </summary>
            <div class="mt-2 space-y-2">
              <div v-if="toolCall.argsJson" class="space-y-1">
                <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolArgs') }}</div>
                <pre
                  class="w-full max-w-full overflow-x-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
                ><code>{{ formatJson(toolCall.argsJson) }}</code></pre>
              </div>
              <div v-if="toolCall.resultJson" class="space-y-1">
                <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolResult') }}</div>
                <pre
                  class="w-full max-w-full max-h-48 overflow-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
                ><code>{{ formatJson(toolCall.resultJson) }}</code></pre>
              </div>
            </div>
          </details>
        </template>

        <!-- Arguments -->
        <div v-else-if="toolCall.argsJson" class="space-y-1">
          <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolArgs') }}</div>
          <pre
            class="w-full max-w-full overflow-x-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
          ><code>{{ formatJson(toolCall.argsJson) }}</code></pre>
        </div>

        <!-- Result -->
        <div v-if="toolCall.resultJson && toolCall.toolName !== 'duckduckgo_search'" class="space-y-1">
          <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolResult') }}</div>
          <pre
            class="w-full max-w-full max-h-48 overflow-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
          ><code>{{ formatJson(toolCall.resultJson) }}</code></pre>
        </div>
      </div>
    </div>
  </div>
</template>
