<script setup lang="ts">
import { ref, watch, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldCheck, Monitor, FolderOpen, X, Terminal, Globe, Plus, Search, Check } from 'lucide-vue-next'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { AgentsService, type FileEntry, UpdateAgentInput } from '@bindings/chatclaw/internal/services/agents'
import { MCPService } from '@bindings/chatclaw/internal/services/mcp'
import type { MCPServer } from '@bindings/chatclaw/internal/services/mcp'
import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { Events } from '@wailsio/runtime'
import { useNavigationStore } from '@/stores/navigation'
import { useSettingsStore } from '@/stores/settings'
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
const navigationStore = useNavigationStore()
const settingsStore = useSettingsStore()
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
    if (open && props.agent) {
      void loadMCPServers()
      if (props.conversationId) {
        void loadWorkspaceData()
      }
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

// ==================== MCP Tools ====================
const globalMCPEnabled = ref(false)
const globalMCPServers = ref<MCPServer[]>([])
const mcpEnabled = computed(() => props.agent?.mcp_enabled ?? false)
const agentMCPServerIDs = computed<string[]>(() => {
  const raw = props.agent?.mcp_server_ids
  if (!raw || raw === '[]') return []
  try { return JSON.parse(raw) } catch { return [] }
})
const agentMCPServerEnabledIDs = computed<string[]>(() => {
  const raw = props.agent?.mcp_server_enabled_ids
  if (!raw || raw === '[]') return []
  try { return JSON.parse(raw) } catch { return [] }
})

// Agent's selected MCP servers (for display in workspace list)
const agentMCPServers = computed(() =>
  globalMCPServers.value.filter((s) => agentMCPServerIDs.value.includes(s.id))
)

// Modal: filtered list by search
const mcpModalSearch = ref('')
const mcpModalFiltered = computed(() => {
  const list = globalMCPServers.value
  const q = mcpModalSearch.value.trim().toLowerCase()
  if (!q) return list
  return list.filter(
    (s) =>
      s.name.toLowerCase().includes(q) ||
      (s.description || '').toLowerCase().includes(q)
  )
})

function isInAgent(id: string): boolean {
  return agentMCPServerIDs.value.includes(id)
}

async function loadMCPServers() {
  try {
    const [all, settings] = await Promise.all([
      MCPService.ListServers(),
      SettingsService.List(Category.CategoryMCP),
    ])
    globalMCPServers.value = (all || []).filter((s) => s.enabled)
    const enabledSetting = settings.find((s) => s.key === 'mcp_enabled')
    globalMCPEnabled.value = enabledSetting?.value === 'true'
  } catch {
    globalMCPServers.value = []
  }
}

async function handleMCPEnabledChange(val: boolean) {
  if (!props.agent) return
  try {
    await AgentsService.UpdateAgent(props.agent.id, new UpdateAgentInput({ mcp_enabled: val }))
    props.agent.mcp_enabled = val
  } catch (error) {
    console.error('Failed to update mcp_enabled:', error)
  }
}

// Modal: local draft of selected IDs (not saved until confirm)
const draftSelectedIDs = ref<Set<string>>(new Set())

function isDraftSelected(id: string): boolean {
  return draftSelectedIDs.value.has(id)
}

function handleDraftToggle(serverId: string, selected: boolean) {
  const next = new Set(draftSelectedIDs.value)
  if (selected) {
    next.add(serverId)
  } else {
    next.delete(serverId)
  }
  draftSelectedIDs.value = next
}

async function handleModalConfirm() {
  if (!props.agent) return
  const newIds = [...draftSelectedIDs.value]
  const currentEnabled = agentMCPServerEnabledIDs.value
  // Keep enabled IDs that are still selected; newly added ones are enabled by default
  const previousIDs = new Set(agentMCPServerIDs.value)
  const newEnabledIds = newIds.filter((id) =>
    previousIDs.has(id) ? currentEnabled.includes(id) : true
  )
  const json = JSON.stringify(newIds)
  const enabledJson = JSON.stringify(newEnabledIds)
  try {
    await AgentsService.UpdateAgent(props.agent.id, new UpdateAgentInput({ mcp_server_ids: json, mcp_server_enabled_ids: enabledJson }))
    props.agent.mcp_server_ids = json
    props.agent.mcp_server_enabled_ids = enabledJson
    addDialogOpen.value = false
  } catch (error) {
    console.error('Failed to update MCP servers:', error)
  }
}

function handleModalCancel() {
  addDialogOpen.value = false
}

async function handleRemoveServer(serverId: string) {
  if (!props.agent) return
  const newIds = agentMCPServerIDs.value.filter((id) => id !== serverId)
  const newEnabledIds = agentMCPServerEnabledIDs.value.filter((id) => id !== serverId)
  const idsJson = JSON.stringify(newIds)
  const enabledJson = JSON.stringify(newEnabledIds)
  try {
    await AgentsService.UpdateAgent(props.agent.id, new UpdateAgentInput({ mcp_server_ids: idsJson, mcp_server_enabled_ids: enabledJson }))
    props.agent.mcp_server_ids = idsJson
    props.agent.mcp_server_enabled_ids = enabledJson
  } catch (error) {
    console.error('Failed to remove MCP server:', error)
  }
}

// Whether all global MCPs are draft-selected
const allDraftSelected = computed(() =>
  globalMCPServers.value.length > 0 &&
  globalMCPServers.value.every((s) => draftSelectedIDs.value.has(s.id))
)

function handleToggleAll() {
  if (allDraftSelected.value) {
    draftSelectedIDs.value = new Set()
  } else {
    draftSelectedIDs.value = new Set(globalMCPServers.value.map((s) => s.id))
  }
}

function navigateToAddMCPServer() {
  addDialogOpen.value = false
  settingsStore.setActiveMenu('mcp')
  navigationStore.navigateToModule('settings')
  setTimeout(() => {
    Events.Emit('mcp:open-add-dialog')
  }, 100)
}

// ==================== Add MCP Picker Dialog ====================
const addDialogOpen = ref(false)

function openAddMCPDialog() {
  mcpModalSearch.value = ''
  draftSelectedIDs.value = new Set(agentMCPServerIDs.value)
  addDialogOpen.value = true
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

      <!-- MCP Tools section -->
      <div class="mt-4">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            {{ t('assistant.workspaceDrawer.mcpTools') }}
          </span>
          <div class="flex items-center gap-1">
            <TooltipProvider :delay-duration="300">
              <Tooltip>
                <TooltipTrigger as-child>
                  <Button
                    size="icon"
                    variant="ghost"
                    class="size-5"
                    :disabled="!globalMCPEnabled"
                    @click="openAddMCPDialog"
                  >
                    <Plus class="size-3 text-muted-foreground" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="left">
                  {{ t('assistant.workspaceDrawer.mcpAddFromGlobal') }}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <Switch
              :model-value="mcpEnabled"
              :disabled="!globalMCPEnabled"
              class="scale-75"
              @update:model-value="handleMCPEnabledChange"
            />
          </div>
        </div>

        <template v-if="!globalMCPEnabled">
          <div class="flex items-center justify-center rounded-lg border border-dashed border-border py-4">
            <span class="text-[11px] text-muted-foreground">
              {{ t('assistant.workspaceDrawer.mcpGlobalDisabled') }}
            </span>
          </div>
        </template>
        <template v-else-if="mcpEnabled">
          <div v-if="agentMCPServers.length === 0" class="flex items-center justify-center rounded-lg border border-dashed border-border py-4">
            <span class="text-[11px] text-muted-foreground">
              {{ t('assistant.workspaceDrawer.mcpEmptyHint') }}
            </span>
          </div>
          <div v-else class="flex flex-col gap-1">
            <div
              v-for="server in agentMCPServers"
              :key="server.id"
              class="flex items-center justify-between gap-2 rounded-md px-2 py-1.5 transition-colors hover:bg-muted/50"
            >
              <div class="flex min-w-0 items-center gap-1.5">
                <Terminal v-if="server.transport === 'stdio'" class="size-3 shrink-0 text-muted-foreground" />
                <Globe v-else class="size-3 shrink-0 text-muted-foreground" />
                <span class="truncate text-xs text-foreground">{{ server.name }}</span>
              </div>
              <button
                type="button"
                class="shrink-0 rounded p-0.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                @click.stop="handleRemoveServer(server.id)"
              >
                <X class="size-3.5" />
              </button>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Add MCP Picker Dialog: select from global MCP list with Switch -->
    <Dialog v-model:open="addDialogOpen">
      <DialogContent size="xl">
        <DialogHeader>
          <div class="flex items-center justify-between pr-8">
            <DialogTitle>{{ t('assistant.workspaceDrawer.mcpAddFromGlobal') }}</DialogTitle>
            <Button size="sm" @click="navigateToAddMCPServer">
              {{ t('assistant.workspaceDrawer.mcpGoToSettings') }}
            </Button>
          </div>
          <DialogDescription class="sr-only">{{ t('assistant.workspaceDrawer.mcpAddFromGlobal') }}</DialogDescription>
        </DialogHeader>

        <div class="flex flex-col gap-3 py-2">
          <div class="relative">
            <Search class="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              v-model="mcpModalSearch"
              :placeholder="t('assistant.workspaceDrawer.mcpSearchPlaceholder')"
              class="pl-8"
            />
          </div>
          <div class="flex items-center justify-between">
            <p class="text-xs text-muted-foreground">
              {{ t('assistant.workspaceDrawer.mcpPickerHint') }}
            </p>
            <button
              v-if="globalMCPServers.length > 0"
              type="button"
              class="shrink-0 cursor-pointer text-xs text-muted-foreground transition-colors hover:text-foreground"
              @click="handleToggleAll"
            >
              {{ allDraftSelected ? t('assistant.workspaceDrawer.mcpDeselectAll') : t('assistant.workspaceDrawer.mcpSelectAll') }}
            </button>
          </div>
          <div v-if="mcpModalFiltered.length === 0" class="flex items-center justify-center rounded-lg border border-dashed border-border py-8">
            <span class="text-xs text-muted-foreground">
              {{ mcpModalSearch.trim() ? t('assistant.workspaceDrawer.mcpNoSearchResults') : t('assistant.workspaceDrawer.mcpNoAvailableToAdd') }}
            </span>
          </div>
          <div v-else class="max-h-[400px] overflow-y-auto">
            <div class="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-3">
              <div
                v-for="server in mcpModalFiltered"
                :key="server.id"
                :class="cn(
                  'relative flex cursor-pointer flex-col rounded-lg border border-border p-3.5 transition-colors dark:border-white/10',
                  isDraftSelected(server.id)
                    ? 'bg-primary/5'
                    : 'hover:bg-accent/30'
                )"
                @click="handleDraftToggle(server.id, !isDraftSelected(server.id))"
              >
                <div
                  v-if="isDraftSelected(server.id)"
                  class="absolute right-2 top-2 flex size-5 items-center justify-center rounded-full bg-primary"
                >
                  <Check class="size-3 text-primary-foreground" />
                </div>
                <div class="flex items-center gap-2 pr-6">
                  <span class="truncate text-sm font-medium text-foreground">{{ server.name }}</span>
                  <span class="inline-flex shrink-0 items-center rounded bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                    <Terminal v-if="server.transport === 'stdio'" class="mr-0.5 size-2.5" />
                    <Globe v-else class="mr-0.5 size-2.5" />
                    {{ server.transport === 'stdio' ? 'stdio' : 'HTTP' }}
                  </span>
                </div>
                <p v-if="server.description" class="mt-1.5 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground">
                  {{ server.description }}
                </p>
              </div>
            </div>
          </div>
        </div>

        <div class="flex items-center justify-end gap-2 border-t border-border pt-4">
          <Button variant="outline" size="sm" @click="handleModalCancel">
            {{ t('common.cancel') }}
          </Button>
          <Button size="sm" @click="handleModalConfirm">
            {{ t('common.confirm') }}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  </div>
</template>
