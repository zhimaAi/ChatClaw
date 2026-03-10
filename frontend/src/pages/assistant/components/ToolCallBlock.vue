<script setup lang="ts">
import { ref, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, Wrench, Loader2, Check, X, Bot, Brain } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import type { ToolCallInfo, MessageSegment } from '@/stores'

const props = defineProps<{
  toolCalls: ToolCallInfo[]
  isStreaming?: boolean
  nested?: boolean
}>()

const { t } = useI18n()

const SUB_AGENT_TOOLS = new Set(['general_purpose', 'bash'])
const isSubAgentTool = (toolName: string) => SUB_AGENT_TOOLS.has(toolName)

const expandedCalls = ref<Set<string>>(new Set())
const expandedAgents = ref<Set<string>>(new Set())

const toggleExpand = (toolCallId: string) => {
  if (expandedCalls.value.has(toolCallId)) {
    expandedCalls.value.delete(toolCallId)
  } else {
    expandedCalls.value.add(toolCallId)
  }
}

const isExpanded = (toolCallId: string) => expandedCalls.value.has(toolCallId)

const toggleAgent = (toolCallId: string) => {
  if (expandedAgents.value.has(toolCallId)) {
    expandedAgents.value.delete(toolCallId)
  } else {
    expandedAgents.value.add(toolCallId)
  }
}

const isAgentExpanded = (toolCallId: string) => expandedAgents.value.has(toolCallId)

const childCount = (toolCall: ToolCallInfo) => toolCall.childToolCalls?.length ?? 0
const childCompleted = (toolCall: ToolCallInfo) =>
  toolCall.childToolCalls?.filter((c) => c.status === 'completed').length ?? 0

const formatJson = (json?: string) => {
  if (!json) return ''
  try {
    return JSON.stringify(JSON.parse(json), null, 2)
  } catch {
    return json
  }
}

const getToolDisplayName = (toolName: string) => {
  const nameMap: Record<string, string> = {
    calculator: t('tools.calculator.name'),
    duckduckgo_search: t('tools.duckduckgo.name'),
    library_retriever: t('tools.libraryRetriever.name'),
    memory_retriever: t('tools.memoryRetriever.name'),
    execute: t('tools.execute.name'),
    execute_background: t('tools.executeBackground.name'),
    http_request: t('tools.httpRequest.name'),
    sequential_thinking: t('tools.sequentialThinking.name'),
    sequentialthinking: t('tools.sequentialThinking.name'),
    wikipedia_search: t('tools.wikipedia.name'),
    browser_use: t('tools.browserUse.name'),
    ls: t('tools.ls.name'),
    read_file: t('tools.readFile.name'),
    write_file: t('tools.writeFile.name'),
    edit_file: t('tools.editFile.name'),
    patch_file: t('tools.patchFile.name'),
    glob: t('tools.glob.name'),
    grep: t('tools.grep.name'),
    skill_search: t('tools.skillSearch.name'),
    skill_list: t('tools.skillList.name'),
    skill_install: t('tools.skillInstall.name'),
    skill_uninstall: t('tools.skillUninstall.name'),
    skill_enable: t('tools.skillEnable.name'),
    skill_disable: t('tools.skillDisable.name'),
    skill_open_folder: t('tools.skillOpenFolder.name'),
    TaskCreate: t('tools.taskCreate.name'),
    TaskGet: t('tools.taskGet.name'),
    TaskUpdate: t('tools.taskUpdate.name'),
    TaskList: t('tools.taskList.name'),
    skill: t('tools.skill.name'),
    write_todos: t('tools.writeTodos.name'),
    task: t('tools.task.name'),
    general_purpose: t('assistant.chat.subAgentGeneralPurpose'),
    bash: t('assistant.chat.subAgentBash'),
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

const MAX_SUB_AGENT_TITLE_LEN = 40

const getSubAgentTitle = (toolCall: ToolCallInfo): string => {
  const prefix = t('assistant.chat.subAgentPrefix')
  const parsed = safeParse(toolCall.argsJson)
  const request = parsed?.request
  if (typeof request === 'string' && request.trim()) {
    const firstLine = request.trim().split('\n')[0]
    if (firstLine.length > MAX_SUB_AGENT_TITLE_LEN) {
      return `${prefix} ${firstLine.slice(0, MAX_SUB_AGENT_TITLE_LEN)}…`
    }
    return `${prefix} ${firstLine}`
  }
  return getToolDisplayName(toolCall.toolName)
}

// --- File-write tool streaming preview ---

const FILE_WRITE_TOOLS = new Set(['write_file', 'edit_file', 'patch_file'])
const isFileWriteTool = (toolName: string) => FILE_WRITE_TOOLS.has(toolName)

function extractCodeFromArgs(toolName: string, argsJson?: string): string {
  if (!argsJson) return ''
  const parsed = safeParse(argsJson)
  if (parsed) {
    if (toolName === 'write_file') return parsed.content ?? ''
    if (toolName === 'edit_file') return parsed.new_string ?? ''
    if (toolName === 'patch_file' && Array.isArray(parsed.operations)) {
      return parsed.operations
        .map((op: any) => op.content ?? '')
        .filter(Boolean)
        .join('\n')
    }
    return ''
  }
  const key = toolName === 'edit_file' ? 'new_string' : 'content'
  return extractValueFromPartialJson(argsJson, key)
}

function extractValueFromPartialJson(json: string, key: string): string {
  const re = new RegExp(`"${key}"\\s*:\\s*"`)
  const m = re.exec(json)
  if (!m) return ''
  let result = ''
  const escapes: Record<string, string> = {
    n: '\n',
    t: '\t',
    '"': '"',
    '\\': '\\',
    '/': '/',
    r: '\r',
  }
  for (let i = m.index + m[0].length; i < json.length; i++) {
    const ch = json[i]
    if (ch === '"') break
    if (ch === '\\' && i + 1 < json.length) {
      const next = json[i + 1]
      if (escapes[next]) {
        result += escapes[next]
        i++
        continue
      }
      if (next === 'u' && i + 5 < json.length) {
        const hex = json.slice(i + 2, i + 6)
        if (/^[0-9a-fA-F]{4}$/.test(hex)) {
          result += String.fromCharCode(parseInt(hex, 16))
          i += 5
          continue
        }
      }
    }
    result += ch
  }
  return result
}

const codePreviewRefs = ref<Record<string, any>>({})

watch(
  () => props.toolCalls.map((tc) => tc.argsJson),
  () => {
    nextTick(() => {
      for (const tc of props.toolCalls) {
        if (tc.status === 'calling' && isFileWriteTool(tc.toolName)) {
          const el = codePreviewRefs.value[tc.toolCallId]
          if (el) el.scrollTop = el.scrollHeight
        }
      }
    })
  },
  { deep: true }
)

function escapeHtml(str: string): string {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// --- DuckDuckGo ---

type DuckDuckGoResult = {
  message?: string
  results?: Array<{ title?: string; url?: string; summary?: string }>
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

// --- Task (sub-agent) ---

const getTaskArgs = (argsJson?: string): { description: string; agentType: string } | null => {
  const obj = safeParse(argsJson)
  if (!obj || typeof obj !== 'object') return null
  return { description: obj.description ?? '', agentType: obj.subagent_type ?? '' }
}

const isTaskTool = (toolName: string) => toolName === 'task'

const getTaskResult = (resultJson?: string): string => {
  if (!resultJson) return ''
  const parsed = safeParse(resultJson)
  if (typeof parsed === 'string') return parsed
  return resultJson
}
</script>

<template>
  <div :class="cn('min-w-0 w-full flex flex-col gap-2', !nested && 'max-w-[560px]')">
    <div
      v-for="toolCall in toolCalls"
      :key="toolCall.toolCallId"
      :class="cn(
        'w-full min-w-0 flex flex-col gap-1 rounded-lg border text-sm',
        isSubAgentTool(toolCall.toolName)
          ? 'border-border/60 bg-muted/15 dark:bg-zinc-900/30'
          : 'border-border/50 bg-muted/30 px-3 py-2 dark:bg-zinc-900/50'
      )"
    >
      <!-- Sub-agent tool -->
      <template v-if="isSubAgentTool(toolCall.toolName)">
        <button
          class="flex w-full items-center gap-2 px-3 py-2 text-left text-muted-foreground hover:text-foreground transition-colors"
          @click="toggleAgent(toolCall.toolCallId)"
        >
          <Bot class="size-4 shrink-0 text-muted-foreground/70" />
          <span class="flex-1 truncate font-medium text-xs">
            {{ getSubAgentTitle(toolCall) }}
          </span>
          <span class="flex items-center gap-1.5 text-xs tabular-nums">
            <template v-if="toolCall.status === 'calling'">
              <Loader2 class="size-3 animate-spin" />
              <span v-if="childCount(toolCall) > 0" class="opacity-70">{{ childCompleted(toolCall) }}/{{ childCount(toolCall) }}</span>
              <span v-else class="opacity-70">{{ t('assistant.chat.toolCalling') }}</span>
            </template>
            <template v-else>
              <Check class="size-3 text-muted-foreground" />
              <span class="opacity-70">{{ t('assistant.chat.toolCompleted') }}</span>
            </template>
          </span>
          <ChevronDown
            :class="cn('size-4 shrink-0 transition-transform', isAgentExpanded(toolCall.toolCallId) && 'rotate-180')"
          />
        </button>
        <div v-if="isAgentExpanded(toolCall.toolCallId)" class="flex flex-col gap-1.5 px-2 pb-2">
          <template v-for="(seg, segIdx) in toolCall.childSegments" :key="segIdx">
            <div
              v-if="seg.type === 'thinking' && seg.content"
              class="flex items-start gap-1.5 rounded-md bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground/80 dark:bg-zinc-800/30"
            >
              <Brain class="size-3 mt-0.5 shrink-0 opacity-60" />
              <p class="min-w-0 whitespace-pre-wrap wrap-break-word line-clamp-4">{{ seg.content }}</p>
            </div>
            <ToolCallBlock
              v-if="seg.type === 'tools' && seg.toolCalls.length"
              :tool-calls="seg.toolCalls"
              :is-streaming="isStreaming"
              nested
            />
            <div
              v-if="seg.type === 'content' && seg.content"
              class="rounded-md border border-border/30 bg-background/40 px-2.5 py-2 text-xs leading-relaxed text-foreground/85 whitespace-pre-wrap wrap-break-word dark:bg-zinc-950/30"
            >{{ seg.content }}<span v-if="toolCall.status === 'calling' && segIdx === toolCall.childSegments!.length - 1" class="streaming-cursor" aria-hidden="true"></span></div>
          </template>
        </div>
      </template>

      <!-- File-write tool -->
      <template v-else-if="isFileWriteTool(toolCall.toolName)">
        <button
          class="flex w-full min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground"
          @click="toggleExpand(toolCall.toolCallId)"
        >
          <Wrench class="size-4 shrink-0" />
          <span class="min-w-0 flex-1 truncate text-xs font-medium">
            {{ getToolDisplayName(toolCall.toolName) }}
          </span>
          <span class="flex items-center gap-1 text-xs">
            <template v-if="toolCall.status === 'calling'">
              <Loader2 class="size-3 animate-spin" />
              <span class="opacity-70">{{ t('assistant.chat.toolCalling') }}</span>
            </template>
            <template v-else-if="toolCall.status === 'completed'">
              <Check class="size-3 text-muted-foreground" />
              <span class="opacity-70">{{ t('assistant.chat.toolCompleted') }}</span>
            </template>
            <template v-else-if="toolCall.status === 'error'">
              <X class="size-3 text-destructive" />
              <span class="opacity-70">{{ t('assistant.chat.toolError') }}</span>
            </template>
          </span>
          <ChevronDown
            :class="
              cn(
                'size-4 shrink-0 transition-transform',
                (toolCall.status === 'calling' || isExpanded(toolCall.toolCallId)) && 'rotate-180'
              )
            "
          />
        </button>

        <!-- Streaming code preview (auto-open during calling) -->
        <div
          v-if="
            (toolCall.status === 'calling' || isExpanded(toolCall.toolCallId)) &&
            extractCodeFromArgs(toolCall.toolName, toolCall.argsJson)
          "
          :ref="
            (el: any) => {
              if (el) codePreviewRefs[toolCall.toolCallId] = el
            }
          "
          class="mt-1 max-h-64 overflow-auto rounded-md border border-border/30"
          style="background: var(--code-block-bg)"
        >
          <pre
            class="p-3 text-xs leading-relaxed whitespace-pre-wrap break-all text-foreground/90"
          ><code
            v-html="escapeHtml(extractCodeFromArgs(toolCall.toolName, toolCall.argsJson)) + (toolCall.status === 'calling' ? '<span class=\'streaming-cursor\' aria-hidden=\'true\'></span>' : '')"
          ></code></pre>
        </div>

        <!-- Result (when expanded) -->
        <div
          v-if="isExpanded(toolCall.toolCallId) && toolCall.resultJson"
          class="mt-1 space-y-1 text-xs"
        >
          <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolResult') }}</div>
          <pre
            class="w-full max-w-full max-h-24 overflow-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
          ><code>{{ toolCall.resultJson }}</code></pre>
        </div>
      </template>

      <!-- Non-file-write tools -->
      <template v-else>
        <button
          class="flex w-full min-w-0 items-center gap-2 text-left text-muted-foreground hover:text-foreground"
          @click="toggleExpand(toolCall.toolCallId)"
        >
          <Wrench class="size-4" />
          <span class="min-w-0 flex-1 truncate text-xs font-medium">
            {{ getToolDisplayName(toolCall.toolName) }}
          </span>
          <span class="flex items-center gap-1 text-xs">
            <template v-if="toolCall.status === 'calling'">
              <Loader2 class="size-3 animate-spin" />
              <span class="opacity-70">{{ t('assistant.chat.toolCalling') }}</span>
            </template>
            <template v-else-if="toolCall.status === 'completed'">
              <Check class="size-3 text-muted-foreground" />
              <span class="opacity-70">{{ t('assistant.chat.toolCompleted') }}</span>
            </template>
            <template v-else-if="toolCall.status === 'error'">
              <X class="size-3 text-destructive" />
              <span class="opacity-70">{{ t('assistant.chat.toolError') }}</span>
            </template>
          </span>
          <ChevronDown
            :class="
              cn('size-4 transition-transform', isExpanded(toolCall.toolCallId) && 'rotate-180')
            "
          />
        </button>

        <div v-if="isExpanded(toolCall.toolCallId)" class="mt-1 space-y-2 text-xs">
          <!-- DuckDuckGo -->
          <template
            v-if="
              toolCall.toolName === 'duckduckgo_search' && getDuckDuckGoResult(toolCall.resultJson)
            "
          >
            <div class="space-y-1">
              <div class="font-medium text-muted-foreground">
                {{ t('assistant.chat.toolResult') }}
              </div>
              <div class="text-muted-foreground/80">
                <span v-if="getQueryFromArgs(toolCall.argsJson)">
                  {{ t('assistant.chat.toolQuery') }}{{ getQueryFromArgs(toolCall.argsJson) }}
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
                  >{{ r?.title || r.url }}</a
                >
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
                {{ t('assistant.chat.rawJson') }}
              </summary>
              <div class="mt-2 space-y-2">
                <div v-if="toolCall.argsJson" class="space-y-1">
                  <div class="font-medium text-muted-foreground">
                    {{ t('assistant.chat.toolArgs') }}
                  </div>
                  <pre
                    class="w-full max-w-full overflow-x-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
                  ><code>{{ formatJson(toolCall.argsJson) }}</code></pre>
                </div>
                <div v-if="toolCall.resultJson" class="space-y-1">
                  <div class="font-medium text-muted-foreground">
                    {{ t('assistant.chat.toolResult') }}
                  </div>
                  <pre
                    class="w-full max-w-full max-h-48 overflow-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
                  ><code>{{ formatJson(toolCall.resultJson) }}</code></pre>
                </div>
              </div>
            </details>
          </template>

          <!-- Task (sub-agent) -->
          <template v-else-if="isTaskTool(toolCall.toolName)">
            <div v-if="getTaskArgs(toolCall.argsJson)" class="space-y-1.5">
              <div
                v-if="getTaskArgs(toolCall.argsJson)?.agentType"
                class="flex items-center gap-1.5"
              >
                <span class="font-medium text-muted-foreground"
                  >{{ t('assistant.chat.taskAgentType') }}:</span
                >
                <span
                  class="rounded bg-background/50 px-1.5 py-0.5 text-foreground/80 dark:bg-zinc-950/50"
                  >{{ getTaskArgs(toolCall.argsJson)?.agentType }}</span
                >
              </div>
              <div v-if="getTaskArgs(toolCall.argsJson)?.description">
                <div class="font-medium text-muted-foreground">
                  {{ t('assistant.chat.taskDescription') }}
                </div>
                <div class="mt-0.5 whitespace-pre-wrap text-foreground/80">
                  {{ getTaskArgs(toolCall.argsJson)?.description }}
                </div>
              </div>
            </div>
            <div v-if="toolCall.resultJson" class="space-y-1">
              <div class="font-medium text-muted-foreground">
                {{ t('assistant.chat.taskAgentResult') }}
              </div>
              <div
                class="max-h-80 overflow-auto rounded border border-border/50 bg-background/30 p-2.5 text-xs leading-relaxed text-foreground/90 whitespace-pre-wrap dark:bg-zinc-950/30"
              >
                {{ getTaskResult(toolCall.resultJson) }}
              </div>
            </div>
          </template>

          <!-- Generic: Arguments -->
          <div v-else-if="toolCall.argsJson" class="space-y-1">
            <div class="font-medium text-muted-foreground">{{ t('assistant.chat.toolArgs') }}</div>
            <pre
              class="w-full max-w-full overflow-x-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
            ><code>{{ formatJson(toolCall.argsJson) }}</code></pre>
          </div>

          <!-- Generic: Result -->
          <div
            v-if="
              toolCall.resultJson &&
              toolCall.toolName !== 'duckduckgo_search' &&
              !isTaskTool(toolCall.toolName) &&
              !isSubAgentTool(toolCall.toolName)
            "
            class="space-y-1"
          >
            <div class="font-medium text-muted-foreground">
              {{ t('assistant.chat.toolResult') }}
            </div>
            <pre
              class="w-full max-w-full max-h-48 overflow-auto rounded bg-background/50 p-2 text-xs dark:bg-zinc-950/50"
            ><code>{{ formatJson(toolCall.resultJson) }}</code></pre>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
