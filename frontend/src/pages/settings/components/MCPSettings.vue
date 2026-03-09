<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  FolderOpen,
  Plus,
  Pencil,
  Trash2,
  Loader2,
  Package,
  Terminal,
  Globe,
  ChevronLeft,
  Wrench,
  MessageSquare,
  Database,
  ExternalLink,
} from 'lucide-vue-next'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { MCPService } from '@bindings/chatclaw/internal/services/mcp'
import type { MCPServer } from '@bindings/chatclaw/internal/services/mcp'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'

const { t } = useI18n()

// ==================== Top-level tabs ====================
type TopTab = 'servers' | 'settings'
const activeTopTab = ref<TopTab>('servers')

// ==================== MCP tab: sub-tabs ====================
type SubTab = 'installed' | 'market'
const activeSubTab = ref<SubTab>('installed')

// ==================== Server list ====================
const servers = ref<MCPServer[]>([])
const serversLoading = ref(false)

async function loadServers() {
  serversLoading.value = true
  try {
    servers.value = await MCPService.ListServers()
  } catch (error) {
    console.error('Failed to load MCP servers:', error)
  } finally {
    serversLoading.value = false
  }
}

async function handleToggleServer(server: MCPServer) {
  const newEnabled = !server.enabled
  try {
    if (newEnabled) {
      await MCPService.EnableServer(server.id)
    } else {
      await MCPService.DisableServer(server.id)
    }
    server.enabled = newEnabled
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

// ==================== Add / Edit dialog ====================
const dialogOpen = ref(false)
const dialogMode = ref<'add' | 'edit'>('add')
const dialogForm = ref({
  id: '',
  name: '',
  transport: 'stdio' as 'stdio' | 'streamableHttp',
  command: '',
  argsText: '',
  envText: '',
  url: '',
  headersText: '',
  timeout: 30,
})
const dialogSaving = ref(false)
const dialogTesting = ref(false)

function openAddDialog() {
  dialogMode.value = 'add'
  dialogForm.value = {
    id: '',
    name: '',
    transport: 'stdio',
    command: '',
    argsText: '',
    envText: '',
    url: '',
    headersText: '',
    timeout: 30,
  }
  dialogOpen.value = true
}

function openEditDialog(server: MCPServer) {
  dialogMode.value = 'edit'

  let argsText = ''
  try {
    const arr = JSON.parse(server.args || '[]')
    if (Array.isArray(arr)) argsText = arr.join('\n')
  } catch { /* keep empty */ }

  let envText = ''
  try {
    const obj = JSON.parse(server.env || '{}')
    envText = Object.entries(obj).map(([k, v]) => `${k}=${v}`).join('\n')
  } catch { /* keep empty */ }

  let headersText = ''
  try {
    const obj = JSON.parse(server.headers || '{}')
    headersText = Object.entries(obj).map(([k, v]) => `${k}=${v}`).join('\n')
  } catch { /* keep empty */ }

  dialogForm.value = {
    id: server.id,
    name: server.name,
    transport: server.transport as 'stdio' | 'streamableHttp',
    command: server.command,
    argsText,
    envText,
    url: server.url,
    headersText,
    timeout: server.timeout > 0 ? server.timeout : 30,
  }
  dialogOpen.value = true
}

function parseLinesToArray(text: string): string {
  const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
  return JSON.stringify(lines)
}

function parseLinesToObject(text: string): string {
  const obj: Record<string, string> = {}
  text.split('\n').forEach((line) => {
    const trimmed = line.trim()
    if (!trimmed) return
    const idx = trimmed.indexOf('=')
    if (idx > 0) {
      obj[trimmed.slice(0, idx).trim()] = trimmed.slice(idx + 1).trim()
    }
  })
  return JSON.stringify(obj)
}

async function handleDialogSave() {
  const form = dialogForm.value
  if (!form.name.trim()) return

  const payload = {
    name: form.name.trim(),
    transport: form.transport,
    command: form.command.trim(),
    args: parseLinesToArray(form.argsText),
    env: parseLinesToObject(form.envText),
    url: form.url.trim(),
    headers: parseLinesToObject(form.headersText),
    timeout: form.timeout,
  }

  dialogTesting.value = true
  try {
    await MCPService.TestServer(payload)
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.testFailed'))
    dialogTesting.value = false
    return
  }
  dialogTesting.value = false

  dialogSaving.value = true
  try {
    if (dialogMode.value === 'add') {
      await MCPService.AddServer(payload)
      toast.success(t('settings.mcp.addSuccess'))
    } else {
      await MCPService.UpdateServer({ id: form.id, ...payload })
      toast.success(t('settings.mcp.updateSuccess'))
    }
    dialogOpen.value = false
    void loadServers()
    if (detailServer.value && dialogMode.value === 'edit' && form.id === detailServer.value.id) {
      void showDetail({ ...detailServer.value, ...payload, id: form.id } as MCPServer)
    }
  } catch (error) {
    toast.error(
      getErrorMessage(error) ||
        t(dialogMode.value === 'add' ? 'settings.mcp.addFailed' : 'settings.mcp.updateFailed'),
    )
  } finally {
    dialogSaving.value = false
  }
}

// ==================== Delete ====================
const deleteTarget = ref<MCPServer | null>(null)

function confirmDelete(server: MCPServer) {
  deleteTarget.value = server
}

async function handleDelete() {
  const server = deleteTarget.value
  if (!server) return
  deleteTarget.value = null
  try {
    await MCPService.RemoveServer(server.id)
    servers.value = servers.value.filter((s) => s.id !== server.id)
    if (detailServer.value?.id === server.id) {
      detailServer.value = null
      detailResult.value = null
    }
    toast.success(t('settings.mcp.deleteSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.deleteFailed'))
  }
}

// ==================== Settings tab ====================
const mcpEnabled = ref(false)
const mcpDir = ref('')

async function loadSettings() {
  try {
    const settings = await SettingsService.List(Category.CategoryMCP)
    const enabledSetting = settings.find((s) => s.key === 'mcp_enabled')
    if (enabledSetting) {
      mcpEnabled.value = enabledSetting.value === 'true'
    }
  } catch (error) {
    console.error('Failed to load MCP settings:', error)
  }

  try {
    const dir = await SettingsService.GetMCPDir()
    mcpDir.value = dir
  } catch (error) {
    console.error('Failed to get MCP directory:', error)
  }
}

async function handleMCPEnabledChange(val: boolean) {
  const prev = mcpEnabled.value
  mcpEnabled.value = val
  try {
    await SettingsService.SetValue('mcp_enabled', String(val))
  } catch (error) {
    console.error('Failed to update mcp_enabled setting:', error)
    mcpEnabled.value = prev
  }
}

async function handleOpenMCPDir() {
  if (!mcpDir.value) return
  try {
    await BrowserService.OpenDirectory(mcpDir.value)
  } catch (error) {
    console.error('Failed to open MCP directory:', error)
  }
}

// ==================== Computed ====================
const dialogTitle = computed(() =>
  dialogMode.value === 'add' ? t('settings.mcp.addServerTitle') : t('settings.mcp.editServer'),
)

const canSave = computed(() => {
  const f = dialogForm.value
  if (!f.name.trim()) return false
  if (f.transport === 'stdio' && !f.command.trim()) return false
  if (f.transport === 'streamableHttp' && !f.url.trim()) return false
  return true
})

function serverSummary(server: MCPServer): string {
  if (server.transport === 'stdio') {
    let summary = server.command
    try {
      const args = JSON.parse(server.args || '[]')
      if (Array.isArray(args) && args.length > 0) {
        summary += ' ' + args.join(' ')
      }
    } catch { /* ignore */ }
    return summary
  }
  return server.url
}

// ==================== Detail view ====================
interface MCPToolInfo { name: string; description: string }
interface MCPPromptInfo { name: string; description: string }
interface MCPResourceInfo { name: string; uri: string; description: string; mimeType: string }
interface InspectResult { tools: MCPToolInfo[]; prompts: MCPPromptInfo[]; resources: MCPResourceInfo[] }

const detailServer = ref<MCPServer | null>(null)
const detailLoading = ref(false)
const detailResult = ref<InspectResult | null>(null)
type DetailTab = 'tools' | 'prompts' | 'resources'
const detailTab = ref<DetailTab>('tools')

async function showDetail(server: MCPServer) {
  detailServer.value = server
  detailTab.value = 'tools'
  detailLoading.value = true
  detailResult.value = null
  try {
    detailResult.value = await MCPService.InspectServer(server.id) as InspectResult
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.inspectFailed'))
  } finally {
    detailLoading.value = false
  }
}

function goBackFromDetail() {
  detailServer.value = null
  detailResult.value = null
}

function editFromDetail() {
  if (detailServer.value) openEditDialog(detailServer.value)
}

function deleteFromDetail() {
  if (detailServer.value) confirmDelete(detailServer.value)
}

// ==================== Market ====================
const mcpMarkets = [
  { name: 'BigModel MCP Market', desc: '精选 MCP，极速接入。', url: 'https://bigmodel.cn/marketplace/index/mcp' },
  { name: 'modelscope.cn', desc: '魔塔社区 MCP 服务器。', url: 'https://www.modelscope.cn/mcp' },
  { name: 'mcp.higress.ai', desc: 'Higress MCP 服务器。', url: 'https://mcp.higress.ai/' },
  { name: 'mcp.so', desc: 'MCP 服务器发现平台。', url: 'https://mcp.so/' },
  { name: 'smithery.ai', desc: 'Smithery MCP 工具。', url: 'https://smithery.ai/' },
  { name: 'glama.ai', desc: 'Glama MCP 服务器目录。', url: 'https://glama.ai/mcp/servers' },
  { name: 'pulsemcp.com', desc: 'Pulse MCP 服务器。', url: 'https://www.pulsemcp.com/' },
  { name: 'mcp.composio.dev', desc: 'Composio MCP 开发工具。', url: 'https://mcp.composio.dev/' },
  { name: 'Model Context Protocol Servers', desc: '官方 MCP 服务器集合。', url: 'https://github.com/modelcontextprotocol/servers' },
  { name: 'awesome MCP Servers', desc: '精选的 MCP 服务器列表。', url: 'https://github.com/punkpeye/awesome-mcp-servers' },
]

async function openMarketLink(url: string) {
  try {
    await BrowserService.OpenURL(url)
  } catch (error) {
    console.error('Failed to open URL:', error)
  }
}

// ==================== Init ====================
onMounted(() => {
  void loadServers()
  void loadSettings()
})
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-hidden bg-background text-foreground">
    <!-- Top tab bar: MCP | Settings -->
    <div class="flex items-center border-b border-border px-4 py-2">
      <div class="inline-flex rounded-lg bg-muted p-1">
        <button
          v-for="tab in ([
            { key: 'servers' as TopTab, label: t('settings.mcp.tabServers') },
            { key: 'settings' as TopTab, label: t('settings.mcp.tabSettings') },
          ])"
          :key="tab.key"
          type="button"
          :class="
            cn(
              'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
              activeTopTab === tab.key
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )
          "
          @click="activeTopTab = tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
    </div>

    <!-- ==================== MCP Tab ==================== -->
    <template v-if="activeTopTab === 'servers'">

      <!-- ==================== Detail View ==================== -->
      <template v-if="detailServer">
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
          <!-- Row 1: back button -->
          <div class="flex shrink-0 items-center border-b border-border px-4 py-2">
            <button
              class="inline-flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              @click="goBackFromDetail"
            >
              <ChevronLeft class="size-4" />
              {{ t('settings.mcp.backToList') }}
            </button>
          </div>

          <!-- Row 2: server info + actions -->
          <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border px-4 py-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="text-base font-semibold text-foreground">{{ detailServer.name }}</span>
                <Badge variant="secondary" class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                  <Terminal v-if="detailServer.transport === 'stdio'" class="mr-0.5 size-2.5" />
                  <Globe v-else class="mr-0.5 size-2.5" />
                  {{ detailServer.transport === 'stdio' ? 'stdio' : 'HTTP' }}
                </Badge>
              </div>
              <p class="mt-1 text-xs leading-relaxed text-muted-foreground">
                {{ serverSummary(detailServer) }}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <button
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="editFromDetail"
              >
                <Pencil class="size-3.5" />
                {{ t('settings.mcp.editServer') }}
              </button>
              <button
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="deleteFromDetail"
              >
                <Trash2 class="size-3.5" />
                {{ t('common.delete') }}
              </button>
            </div>
          </div>

          <!-- Row 3: capability tabs -->
          <div class="flex shrink-0 items-center gap-1 border-b border-border px-4 py-2">
            <button
              v-for="tab in ([
                { key: 'tools' as DetailTab, label: t('settings.mcp.tabTools'), icon: Wrench, count: detailResult?.tools?.length ?? 0 },
                { key: 'prompts' as DetailTab, label: t('settings.mcp.tabPrompts'), icon: MessageSquare, count: detailResult?.prompts?.length ?? 0 },
                { key: 'resources' as DetailTab, label: t('settings.mcp.tabResources'), icon: Database, count: detailResult?.resources?.length ?? 0 },
              ])"
              :key="tab.key"
              :class="
                cn(
                  'inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  detailTab === tab.key
                    ? 'bg-foreground text-background'
                    : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground',
                )
              "
              @click="detailTab = tab.key"
            >
              <component :is="tab.icon" class="size-3" />
              {{ tab.label }}
              <Badge
                v-if="detailResult && tab.count > 0"
                variant="secondary"
                class="ml-0.5 bg-background/20 px-1.5 py-0 text-[10px]"
              >
                {{ tab.count }}
              </Badge>
            </button>
          </div>

          <!-- Row 4: capability content -->
          <div class="flex-1 overflow-auto px-4 py-3">
            <div v-if="detailLoading" class="flex items-center justify-center py-12">
              <Loader2 class="size-5 animate-spin text-muted-foreground" />
            </div>

            <template v-else-if="detailResult">
              <!-- Tools -->
              <div v-if="detailTab === 'tools'">
                <div v-if="detailResult.tools.length === 0" class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground">
                  <Wrench class="size-8 opacity-40" />
                  <span class="text-sm">{{ t('settings.mcp.noTools') }}</span>
                </div>
                <div v-else class="flex flex-col gap-1">
                  <div
                    v-for="tool in detailResult.tools"
                    :key="tool.name"
                    class="rounded-md border border-border p-3 dark:border-white/10"
                  >
                    <div class="flex items-center gap-2">
                      <Wrench class="size-3.5 shrink-0 text-muted-foreground" />
                      <span class="text-sm font-medium text-foreground">{{ tool.name }}</span>
                    </div>
                    <p v-if="tool.description" class="mt-1 pl-5.5 text-xs leading-relaxed text-muted-foreground">
                      {{ tool.description }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Prompts -->
              <div v-if="detailTab === 'prompts'">
                <div v-if="detailResult.prompts.length === 0" class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground">
                  <MessageSquare class="size-8 opacity-40" />
                  <span class="text-sm">{{ t('settings.mcp.noPrompts') }}</span>
                </div>
                <div v-else class="flex flex-col gap-1">
                  <div
                    v-for="prompt in detailResult.prompts"
                    :key="prompt.name"
                    class="rounded-md border border-border p-3 dark:border-white/10"
                  >
                    <div class="flex items-center gap-2">
                      <MessageSquare class="size-3.5 shrink-0 text-muted-foreground" />
                      <span class="text-sm font-medium text-foreground">{{ prompt.name }}</span>
                    </div>
                    <p v-if="prompt.description" class="mt-1 pl-5.5 text-xs leading-relaxed text-muted-foreground">
                      {{ prompt.description }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Resources -->
              <div v-if="detailTab === 'resources'">
                <div v-if="detailResult.resources.length === 0" class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground">
                  <Database class="size-8 opacity-40" />
                  <span class="text-sm">{{ t('settings.mcp.noResources') }}</span>
                </div>
                <div v-else class="flex flex-col gap-1">
                  <div
                    v-for="res in detailResult.resources"
                    :key="res.uri"
                    class="rounded-md border border-border p-3 dark:border-white/10"
                  >
                    <div class="flex items-center gap-2">
                      <Database class="size-3.5 shrink-0 text-muted-foreground" />
                      <span class="text-sm font-medium text-foreground">{{ res.name }}</span>
                      <Badge v-if="res.mimeType" variant="secondary" class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                        {{ res.mimeType }}
                      </Badge>
                    </div>
                    <p class="mt-1 pl-5.5 text-xs text-muted-foreground/70">{{ res.uri }}</p>
                    <p v-if="res.description" class="mt-0.5 pl-5.5 text-xs leading-relaxed text-muted-foreground">
                      {{ res.description }}
                    </p>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </template>

      <!-- ==================== List View ==================== -->
      <template v-else>
        <!-- Sub tab bar: Installed | Market + Add button -->
        <div class="flex items-center justify-between px-4 py-2">
          <div class="flex gap-1">
            <button
              :class="
                cn(
                  'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  activeSubTab === 'installed'
                    ? 'bg-foreground text-background'
                    : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground',
                )
              "
              @click="activeSubTab = 'installed'"
            >
              {{ t('settings.mcp.tabInstalled') }}
            </button>
            <button
              :class="
                cn(
                  'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  activeSubTab === 'market'
                    ? 'bg-foreground text-background'
                    : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground',
                )
              "
              @click="activeSubTab = 'market'"
            >
              {{ t('settings.mcp.tabMarket') }}
            </button>
          </div>
          <button
            v-if="activeSubTab === 'installed'"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            @click="openAddDialog"
          >
            <Plus class="size-3.5" />
            {{ t('settings.mcp.addServer') }}
          </button>
        </div>

        <!-- Installed server list -->
        <div v-if="activeSubTab === 'installed'" class="flex-1 overflow-auto px-4 pb-4">
          <div
            v-if="serversLoading && servers.length === 0"
            class="flex items-center justify-center py-12"
          >
            <Loader2 class="size-5 animate-spin text-muted-foreground" />
          </div>
          <div
            v-else-if="servers.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
          >
            <Package class="size-8 opacity-40" />
            <span class="text-sm">{{ t('settings.mcp.noServers') }}</span>
            <span class="text-xs">{{ t('settings.mcp.noServersHint') }}</span>
          </div>
          <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
            <div
              v-for="server in servers"
              :key="server.id"
              class="group flex cursor-pointer flex-col rounded-lg border border-border p-3.5 transition-colors hover:bg-accent/30 dark:border-white/10"
              @click="showDetail(server)"
            >
              <div class="flex items-center gap-2">
                <span class="truncate text-sm font-medium text-foreground">{{ server.name }}</span>
                <Badge variant="secondary" class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                  <Terminal v-if="server.transport === 'stdio'" class="mr-0.5 size-2.5" />
                  <Globe v-else class="mr-0.5 size-2.5" />
                  {{ server.transport === 'stdio' ? 'stdio' : 'HTTP' }}
                </Badge>
              </div>
              <p class="mt-1.5 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground">
                {{ serverSummary(server) }}
              </p>
              <div class="mt-auto flex items-center justify-between gap-2 pt-3">
                <span class="text-[10px] text-muted-foreground">{{ server.timeout }}s timeout</span>
                <div class="flex items-center gap-2" @click.stop>
                  <Switch
                    :model-value="server.enabled"
                    class="scale-90"
                    @update:model-value="() => handleToggleServer(server)"
                  />
                  <button
                    class="inline-flex cursor-pointer items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                    @click.stop="confirmDelete(server)"
                  >
                    <Trash2 class="size-3.5" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Market tab -->
        <div v-if="activeSubTab === 'market'" class="flex-1 overflow-auto px-4 pb-4">
          <div class="flex flex-col gap-1.5">
            <div
              v-for="item in mcpMarkets"
              :key="item.url"
              class="group flex cursor-pointer items-center justify-between rounded-lg border border-border p-3.5 transition-colors hover:bg-accent/30 dark:border-white/10"
              @click="openMarketLink(item.url)"
            >
              <div class="min-w-0 flex-1">
                <span class="text-sm font-medium text-foreground">{{ item.name }}</span>
                <p class="mt-0.5 text-xs text-muted-foreground">{{ item.desc }}</p>
              </div>
              <ExternalLink class="size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" />
            </div>
          </div>
        </div>
      </template>
    </template>

    <!-- ==================== Settings Tab ==================== -->
    <template v-if="activeTopTab === 'settings'">
      <div class="flex flex-1 flex-col items-center overflow-auto py-8">
        <SettingsCard :title="t('settings.mcp.title')">
          <SettingsItem>
            <template #label>
              <div class="flex flex-col gap-1">
                <span class="text-sm font-medium text-foreground">{{ t('settings.mcp.enable') }}</span>
                <span class="text-xs text-muted-foreground">{{ t('settings.mcp.enableHint') }}</span>
              </div>
            </template>
            <Switch
              :model-value="mcpEnabled"
              @update:model-value="handleMCPEnabledChange"
            />
          </SettingsItem>

          <div class="flex flex-col gap-1 p-4">
            <span class="text-sm font-medium text-foreground">{{ t('settings.mcp.directory') }}</span>
            <span class="text-xs text-muted-foreground">{{ t('settings.mcp.directoryHint') }}</span>
            <div class="flex w-full items-center gap-2 pt-0.5">
              <Input
                :model-value="mcpDir"
                readonly
                class="flex-1 min-w-0 cursor-default bg-muted/30"
              />
              <button
                class="inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="handleOpenMCPDir"
              >
                <FolderOpen class="size-4" />
              </button>
            </div>
          </div>
        </SettingsCard>
      </div>
    </template>

    <!-- ==================== Add/Edit Dialog ==================== -->
    <Dialog v-model:open="dialogOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{{ dialogTitle }}</DialogTitle>
          <DialogDescription class="sr-only">{{ dialogTitle }}</DialogDescription>
        </DialogHeader>

        <div class="flex flex-col gap-4 py-2">
          <!-- Name -->
          <div class="flex flex-col gap-1.5">
            <Label class="text-sm">{{ t('settings.mcp.serverName') }}</Label>
            <Input
              v-model="dialogForm.name"
              :placeholder="t('settings.mcp.serverNamePlaceholder')"
            />
          </div>

          <!-- Transport type -->
          <div class="flex flex-col gap-1.5">
            <Label class="text-sm">{{ t('settings.mcp.transportType') }}</Label>
            <Select v-model="dialogForm.transport">
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="stdio">{{ t('settings.mcp.transportStdio') }}</SelectItem>
                <SelectItem value="streamableHttp">{{ t('settings.mcp.transportHttp') }}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <!-- stdio fields -->
          <template v-if="dialogForm.transport === 'stdio'">
            <div class="flex flex-col gap-1.5">
              <Label class="text-sm">{{ t('settings.mcp.command') }}</Label>
              <Input
                v-model="dialogForm.command"
                :placeholder="t('settings.mcp.commandPlaceholder')"
              />
            </div>
            <div class="flex flex-col gap-1.5">
              <Label class="text-sm">{{ t('settings.mcp.args') }}</Label>
              <textarea
                v-model="dialogForm.argsText"
                :placeholder="t('settings.mcp.argsPlaceholder')"
                rows="3"
                class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              />
            </div>
            <div class="flex flex-col gap-1.5">
              <Label class="text-sm">{{ t('settings.mcp.envVars') }}</Label>
              <textarea
                v-model="dialogForm.envText"
                :placeholder="t('settings.mcp.envVarsPlaceholder')"
                rows="3"
                class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              />
            </div>
          </template>

          <!-- streamableHttp fields -->
          <template v-if="dialogForm.transport === 'streamableHttp'">
            <div class="flex flex-col gap-1.5">
              <Label class="text-sm">{{ t('settings.mcp.serverUrl') }}</Label>
              <Input
                v-model="dialogForm.url"
                :placeholder="t('settings.mcp.serverUrlPlaceholder')"
              />
            </div>
            <div class="flex flex-col gap-1.5">
              <Label class="text-sm">{{ t('settings.mcp.httpHeaders') }}</Label>
              <textarea
                v-model="dialogForm.headersText"
                :placeholder="t('settings.mcp.httpHeadersPlaceholder')"
                rows="3"
                class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              />
            </div>
          </template>

          <!-- Timeout (common) -->
          <div class="flex flex-col gap-1.5">
            <Label class="text-sm">{{ t('settings.mcp.timeout') }}</Label>
            <div class="flex items-center gap-2">
              <Input
                v-model.number="dialogForm.timeout"
                type="number"
                :min="1"
                :max="300"
                class="w-24"
              />
              <span class="text-xs text-muted-foreground">{{ t('settings.mcp.timeoutUnit') }}</span>
            </div>
          </div>
        </div>

        <DialogFooter>
          <button
            class="cursor-pointer rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-80 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!canSave || dialogSaving || dialogTesting"
            @click="handleDialogSave"
          >
            <Loader2 v-if="dialogTesting || dialogSaving" class="mr-1.5 inline size-3.5 animate-spin" />
            <template v-if="dialogTesting">{{ t('settings.mcp.testing') }}</template>
            <template v-else>{{ dialogMode === 'add' ? t('settings.mcp.addServer') : t('common.save') }}</template>
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- ==================== Delete Confirm ==================== -->
    <AlertDialog :open="!!deleteTarget">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('common.delete') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.mcp.deleteConfirm') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel @click="deleteTarget = null">{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="handleDelete">{{ t('common.confirm') }}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
