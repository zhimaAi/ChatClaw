<script setup lang="ts">
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldCheck, Monitor, FolderOpen, X } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { AgentsService, type FileEntry } from '@bindings/chatclaw/internal/services/agents'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { Events } from '@wailsio/runtime'
import FileTreeNode from './FileTreeNode.vue'

const FS_MUTATING_TOOLS = new Set([
  'write_file', 'edit_file', 'patch_file', 'execute', 'execute_background',
])

const props = defineProps<{
  open: boolean
  agent: Agent | null
  conversationId: number | null | undefined
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  openWorkspaceSettings: []
}>()

const { t } = useI18n()
const MAX_TREE_DEPTH = 3

const workspaceDir = ref('')
const fileTree = ref<FileEntry[]>([])
const loading = ref(false)
const expandedDirs = ref<Set<string>>(new Set())

const sandboxMode = computed(() => props.agent?.sandbox_mode || 'codex')
const isSandbox = computed(() => sandboxMode.value === 'codex')
const hasConversation = computed(() => !!props.conversationId)

const defaultWorkDir = ref('')
const displayWorkDir = computed(() => props.agent?.work_dir || defaultWorkDir.value)

const refreshFileTree = async () => {
  if (!props.agent || !props.conversationId) return
  try {
    const files = await AgentsService.ListWorkspaceFiles(props.agent.id, props.conversationId)
    fileTree.value = files || []
  } catch {
    // Silently ignore refresh errors
  }
}

let refreshTimer: ReturnType<typeof setTimeout> | null = null

const debouncedRefresh = () => {
  if (!props.open) return
  if (refreshTimer) clearTimeout(refreshTimer)
  refreshTimer = setTimeout(() => {
    refreshTimer = null
    void refreshFileTree()
  }, 800)
}

const loadWorkspaceData = async () => {
  if (!props.agent || !props.conversationId) return
  loading.value = true
  try {
    const dir = await AgentsService.GetWorkspaceDir(props.agent.id, props.conversationId)
    workspaceDir.value = dir
    const files = await AgentsService.ListWorkspaceFiles(props.agent.id, props.conversationId)
    fileTree.value = files || []
  } catch (error) {
    console.error('Failed to load workspace data:', error)
    fileTree.value = []
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.open, props.agent?.id, props.conversationId],
  ([open]) => {
    if (open && props.agent && props.conversationId) {
      void loadWorkspaceData()
    }
  },
  { immediate: true }
)

const toggleDir = (path: string) => {
  const newSet = new Set(expandedDirs.value)
  if (newSet.has(path)) {
    newSet.delete(path)
  } else {
    newSet.add(path)
  }
  expandedDirs.value = newSet
}

const handleOpenFolder = async () => {
  if (!hasConversation.value) {
    emit('openWorkspaceSettings')
    return
  }
  if (!workspaceDir.value) return
  try {
    await BrowserService.OpenDirectory(workspaceDir.value)
  } catch (error) {
    console.error('Failed to open directory:', error)
  }
}

const handleEnvironmentClick = () => {
  emit('openWorkspaceSettings')
}

const handleClose = () => {
  emit('update:open', false)
}

let unsubTool: (() => void) | null = null
let unsubComplete: (() => void) | null = null

onMounted(() => {
  void AgentsService.GetDefaultWorkDir().then((dir) => {
    defaultWorkDir.value = dir
  })

  unsubTool = Events.On('chat:tool', (event: any) => {
    const data = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    if (!data || !props.conversationId) return
    if (data.conversation_id !== props.conversationId) return
    if (data.type === 'result' && FS_MUTATING_TOOLS.has(data.tool_name)) {
      debouncedRefresh()
    }
  })

  unsubComplete = Events.On('chat:complete', (event: any) => {
    const data = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    if (!data || !props.conversationId) return
    if (data.conversation_id !== props.conversationId) return
    debouncedRefresh()
  })
})

onUnmounted(() => {
  unsubTool?.()
  unsubTool = null
  unsubComplete?.()
  unsubComplete = null
  if (refreshTimer) {
    clearTimeout(refreshTimer)
    refreshTimer = null
  }
})
</script>

<template>
  <div
    :class="cn(
      'flex h-full shrink-0 flex-col border-l border-border bg-background transition-[width,opacity] duration-200 overflow-hidden',
      open ? 'w-[280px] opacity-100' : 'w-0 opacity-0 border-l-0'
    )"
  >
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-border px-3 py-2">
      <span class="text-sm font-medium text-foreground">
        {{ t('assistant.workspaceDrawer.title') }}
      </span>
      <Button
        size="icon"
        variant="ghost"
        class="size-6"
        @click="handleClose"
      >
        <X class="size-3.5 text-muted-foreground" />
      </Button>
    </div>

    <!-- Content -->
    <div class="flex-1 overflow-y-auto px-3 py-3">
      <!-- Environment section -->
      <div class="mb-4">
        <div class="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
          {{ t('assistant.workspaceDrawer.environment') }}
        </div>
        <div class="flex gap-2">
          <button
            class="flex flex-1 items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors"
            :class="isSandbox
              ? 'border-primary bg-primary/10 text-primary'
              : 'border-border text-muted-foreground hover:border-foreground/20 hover:text-foreground'"
            @click="handleEnvironmentClick"
          >
            <ShieldCheck class="size-4" />
            <span class="truncate">{{ t('assistant.workspaceDrawer.sandboxEnv') }}</span>
          </button>
          <button
            class="flex flex-1 items-center gap-2 rounded-lg border px-3 py-2 text-sm transition-colors"
            :class="!isSandbox
              ? 'border-primary bg-primary/10 text-primary'
              : 'border-border text-muted-foreground hover:border-foreground/20 hover:text-foreground'"
            @click="handleEnvironmentClick"
          >
            <Monitor class="size-4" />
            <span class="truncate">{{ t('assistant.workspaceDrawer.nativeEnv') }}</span>
          </button>
        </div>
      </div>

      <!-- Output files section -->
      <div>
        <div class="mb-2 flex items-center justify-between">
          <span class="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {{ t('assistant.workspaceDrawer.outputFiles') }}
          </span>
          <TooltipProvider :delay-duration="300">
            <Tooltip>
              <TooltipTrigger as-child>
                <Button
                  size="icon"
                  variant="ghost"
                  class="size-5"
                  @click="handleOpenFolder"
                >
                  <FolderOpen class="size-3 text-muted-foreground" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="left">
                {{ hasConversation ? t('assistant.workspaceDrawer.openFolder') : t('assistant.workspaceDrawer.configureWorkspace') }}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>

        <template v-if="hasConversation">
          <!-- Directory path -->
          <div
            v-if="workspaceDir"
            class="mb-2 truncate rounded-md bg-muted/50 px-2 py-1.5 font-mono text-[11px] text-muted-foreground"
            :title="workspaceDir"
          >
            {{ workspaceDir }}
          </div>

          <div class="mb-2 text-[11px] text-muted-foreground/80">
            {{ t('assistant.workspaceDrawer.depthLimitHint', { depth: MAX_TREE_DEPTH }) }}
          </div>

          <!-- File tree -->
          <div v-if="fileTree.length > 0" class="flex flex-col">
            <FileTreeNode
              v-for="entry in fileTree"
              :key="entry.path"
              :entry="entry"
              :depth="0"
              :expanded-dirs="expandedDirs"
              @toggle="toggleDir"
            />
          </div>

          <!-- Empty state -->
          <div
            v-else-if="!loading"
            class="flex items-center justify-center rounded-lg border border-dashed border-border py-6"
          >
            <span class="text-xs text-muted-foreground">
              {{ t('assistant.workspaceDrawer.noFiles') }}
            </span>
          </div>
        </template>

        <!-- No conversation: show default work dir with link to settings -->
        <button
          v-else
          class="group w-full cursor-pointer rounded-lg border border-dashed border-border px-3 py-3 text-left transition-colors hover:border-foreground/20 hover:bg-muted/50"
          @click="handleEnvironmentClick"
        >
          <div class="mb-1 text-[11px] text-muted-foreground">
            {{ t('assistant.workspaceDrawer.noConversationDir') }}
          </div>
          <div
            class="truncate font-mono text-[11px] text-muted-foreground"
            :title="displayWorkDir"
          >
            {{ displayWorkDir }}
          </div>
          <div class="mt-1.5 text-[11px] text-primary/70 group-hover:text-primary">
            {{ t('assistant.workspaceDrawer.noConversationAction') }}
          </div>
        </button>
      </div>
    </div>
  </div>
</template>
