<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
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
  X,
  RefreshCw,
  MoreHorizontal,
  Check,
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import logoBigmodel from '@/assets/icons/mcp-market/bigmodel.png'
import logoModelscope from '@/assets/icons/mcp-market/modelscope.png'
import logoHigress from '@/assets/icons/mcp-market/higress.png'
import logoMcpSo from '@/assets/icons/mcp-market/mcp-so.png'
import logoSmithery from '@/assets/icons/mcp-market/smithery.png'
import logoGlama from '@/assets/icons/mcp-market/glama.png'
import logoPulsemcp from '@/assets/icons/mcp-market/pulsemcp.png'
import logoComposio from '@/assets/icons/mcp-market/composio.png'
import logoGithub from '@/assets/icons/mcp-market/github.png'

import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { MCPService } from '@bindings/chatclaw/internal/services/mcp'
import type { MCPServer } from '@bindings/chatclaw/internal/services/mcp'
import { AssistantMCPService } from '@bindings/chatclaw/internal/services/assistantmcp'
import type { AssistantMCP } from '@bindings/chatclaw/internal/services/assistantmcp'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { useThemeLogo } from '@/composables/useLogo'
import { Events } from '@wailsio/runtime'

const { t } = useI18n()
const { logoSrc } = useThemeLogo()

// ==================== Top-level tabs ====================
type TopTab = 'servers' | 'settings'
const activeTopTab = ref<TopTab>('servers')

// ==================== MCP tab: sub-tabs ====================
type SubTab = 'installed' | 'market' | 'assistantMcp'
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

// ==================== Validate all enabled servers ====================
const validating = ref(false)

async function handleValidateAll() {
  validating.value = true
  try {
    const disabledIDs: string[] = await MCPService.ValidateEnabledServers()
    if (disabledIDs.length > 0) {
      for (const srv of servers.value) {
        if (disabledIDs.includes(srv.id)) {
          srv.enabled = false
        }
      }
      toast.default(t('settings.mcp.validateDisabled', { count: disabledIDs.length }))
    } else {
      toast.success(t('settings.mcp.validateAllPassed'))
    }
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    validating.value = false
  }
}

// ==================== Add / Edit dialog ====================
const dialogOpen = ref(false)
const dialogMode = ref<'add' | 'edit'>('add')
const dialogForm = ref({
  id: '',
  name: '',
  description: '',
  transport: 'stdio' as 'stdio' | 'streamableHttp',
  command: '',
  argsText: '',
  envPairs: [] as Array<{ key: string; value: string }>,
  url: '',
  headerPairs: [] as Array<{ key: string; value: string }>,
  timeout: 30,
})
const dialogSaving = ref(false)
const dialogTesting = ref(false)

function openAddDialog() {
  dialogMode.value = 'add'
  dialogForm.value = {
    id: '',
    name: '',
    description: '',
    transport: 'stdio',
    command: '',
    argsText: '',
    envPairs: [],
    url: '',
    headerPairs: [],
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
  } catch {
    /* keep empty */
  }

  let envPairs: Array<{ key: string; value: string }> = []
  try {
    const obj = JSON.parse(server.env || '{}')
    envPairs = Object.entries(obj).map(([k, v]) => ({ key: k, value: String(v) }))
  } catch {
    /* keep empty */
  }

  let headerPairs: Array<{ key: string; value: string }> = []
  try {
    const obj = JSON.parse(server.headers || '{}')
    headerPairs = Object.entries(obj).map(([k, v]) => ({ key: k, value: String(v) }))
  } catch {
    /* keep empty */
  }

  dialogForm.value = {
    id: server.id,
    name: server.name,
    description: server.description || '',
    transport: server.transport as 'stdio' | 'streamableHttp',
    command: server.command,
    argsText,
    envPairs,
    url: server.url,
    headerPairs,
    timeout: server.timeout > 0 ? server.timeout : 30,
  }
  dialogOpen.value = true
}

function parseLinesToArray(text: string): string {
  const lines = text
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
  return JSON.stringify(lines)
}

function pairsToJsonObject(pairs: Array<{ key: string; value: string }>): string {
  const obj: Record<string, string> = {}
  pairs.forEach(({ key, value }) => {
    const k = key.trim()
    if (k) obj[k] = value
  })
  return JSON.stringify(obj)
}

function addPair(pairs: Array<{ key: string; value: string }>) {
  pairs.push({ key: '', value: '' })
}

function removePair(pairs: Array<{ key: string; value: string }>, index: number) {
  pairs.splice(index, 1)
}

async function handleDialogSave() {
  const form = dialogForm.value
  if (!form.name.trim() || !form.description.trim()) return

  const payload = {
    name: form.name.trim(),
    description: form.description.trim(),
    transport: form.transport,
    command: form.command.trim(),
    args: parseLinesToArray(form.argsText),
    env: pairsToJsonObject(form.envPairs),
    url: form.url.trim(),
    headers: pairsToJsonObject(form.headerPairs),
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
        t(dialogMode.value === 'add' ? 'settings.mcp.addFailed' : 'settings.mcp.updateFailed')
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
const mcpEnabled = ref(true)

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
}

async function handleMCPEnabledChange(val: boolean) {
  const prev = mcpEnabled.value
  mcpEnabled.value = val
  try {
    await SettingsService.SetValue('mcp_enabled', String(val))
    if (val) {
      await AssistantMCPService.StartEnabledServers()
    } else {
      await AssistantMCPService.StopAllServers()
    }
    void loadAssistantMcps()
  } catch (error) {
    console.error('Failed to update mcp_enabled setting:', error)
    mcpEnabled.value = prev
  }
}

// ==================== Computed ====================
const dialogTitle = computed(() =>
  dialogMode.value === 'add' ? t('settings.mcp.addServerTitle') : t('settings.mcp.editServer')
)

const canSave = computed(() => {
  const f = dialogForm.value
  if (!f.name.trim()) return false
  if (!f.description.trim()) return false
  if (f.transport === 'stdio' && !f.command.trim()) return false
  if (f.transport === 'streamableHttp' && !f.url.trim()) return false
  return true
})

function serverSummary(server: MCPServer): string {
  return server.description
}

// ==================== Detail view ====================
interface MCPToolInfo {
  name: string
  description: string
}
interface MCPPromptInfo {
  name: string
  description: string
}
interface MCPResourceInfo {
  name: string
  uri: string
  description: string
  mimeType: string
}
interface InspectResult {
  tools: MCPToolInfo[]
  prompts: MCPPromptInfo[]
  resources: MCPResourceInfo[]
}

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
    detailResult.value = (await MCPService.InspectServer(server.id)) as InspectResult
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
  {
    name: 'BigModel MCP Market',
    desc: '精选 MCP，极速接入。',
    url: 'https://bigmodel.cn/marketplace/index/mcp',
    logo: logoBigmodel,
  },
  {
    name: 'modelscope.cn',
    desc: '魔塔社区 MCP 服务器。',
    url: 'https://www.modelscope.cn/mcp',
    logo: logoModelscope,
  },
  {
    name: 'mcp.higress.ai',
    desc: 'Higress MCP 服务器。',
    url: 'https://mcp.higress.ai/',
    logo: logoHigress,
  },
  { name: 'mcp.so', desc: 'MCP 服务器发现平台。', url: 'https://mcp.so/', logo: logoMcpSo },
  {
    name: 'smithery.ai',
    desc: 'Smithery MCP 工具。',
    url: 'https://smithery.ai/',
    logo: logoSmithery,
  },
  {
    name: 'glama.ai',
    desc: 'Glama MCP 服务器目录。',
    url: 'https://glama.ai/mcp/servers',
    logo: logoGlama,
  },
  {
    name: 'pulsemcp.com',
    desc: 'Pulse MCP 服务器。',
    url: 'https://www.pulsemcp.com/',
    logo: logoPulsemcp,
  },
  {
    name: 'mcp.composio.dev',
    desc: 'Composio MCP 开发工具。',
    url: 'https://mcp.composio.dev/',
    logo: logoComposio,
  },
  {
    name: 'Model Context Protocol Servers',
    desc: '官方 MCP 服务器集合。',
    url: 'https://github.com/modelcontextprotocol/servers',
    logo: logoGithub,
  },
  {
    name: 'awesome MCP Servers',
    desc: '精选的 MCP 服务器列表。',
    url: 'https://github.com/punkpeye/awesome-mcp-servers',
    logo: logoGithub,
  },
]

async function openMarketLink(url: string) {
  try {
    await BrowserService.OpenURL(url)
  } catch (error) {
    console.error('Failed to open URL:', error)
  }
}

// ==================== Assistant MCP ====================
const assistantMcps = ref<AssistantMCP[]>([])
const assistantMcpsLoading = ref(false)

async function loadAssistantMcps() {
  assistantMcpsLoading.value = true
  try {
    assistantMcps.value = await AssistantMCPService.List()
  } catch (error) {
    console.error('Failed to load assistant MCPs:', error)
  } finally {
    assistantMcpsLoading.value = false
  }
}

async function handleToggleAssistantMcp(item: AssistantMCP) {
  const newEnabled = !item.enabled
  try {
    if (newEnabled) {
      await AssistantMCPService.Enable(item.id)
    } else {
      await AssistantMCPService.Disable(item.id)
    }
    item.enabled = newEnabled
    void loadAssistantMcps()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

// Add/Edit assistant MCP dialog
const amcpDialogOpen = ref(false)
const amcpDialogMode = ref<'add' | 'edit'>('add')
const amcpDialogForm = ref({ id: '', name: '', description: '', port: 0 })
const amcpDialogSaving = ref(false)

function openAddAssistantMcpDialog() {
  amcpDialogMode.value = 'add'
  amcpDialogForm.value = { id: '', name: '', description: '', port: 0 }
  amcpDialogOpen.value = true
}

function openEditAssistantMcpDialog(item: AssistantMCP) {
  amcpDialogMode.value = 'edit'
  amcpDialogForm.value = {
    id: item.id,
    name: item.name,
    description: item.description,
    port: item.port,
  }
  amcpDialogOpen.value = true
}

const amcpCanSave = computed(() => {
  const len = amcpDialogForm.value.name.trim().length
  return len > 0 && len <= 20
})

async function handleSaveAssistantMcp() {
  const form = amcpDialogForm.value
  if (!form.name.trim()) return

  amcpDialogSaving.value = true
  try {
    if (amcpDialogMode.value === 'add') {
      await AssistantMCPService.Create({
        name: form.name.trim(),
        description: form.description.trim(),
      })
      toast.success(t('settings.mcp.assistantMcpCreateSuccess'))
    } else {
      const updated = await AssistantMCPService.Update({
        id: form.id,
        name: form.name.trim(),
        description: form.description.trim(),
      })
      toast.success(t('settings.mcp.assistantMcpUpdateSuccess'))
      if (updated && amcpDetail.value?.id === form.id) {
        amcpDetail.value = updated
      }
      const idx = assistantMcps.value.findIndex((a) => a.id === form.id)
      if (idx >= 0 && updated) assistantMcps.value[idx] = updated
    }
    amcpDialogOpen.value = false
    void loadAssistantMcps()
  } catch (error) {
    toast.error(
      getErrorMessage(error) ||
        t(
          amcpDialogMode.value === 'add'
            ? 'settings.mcp.assistantMcpCreateFailed'
            : 'settings.mcp.assistantMcpUpdateFailed'
        )
    )
  } finally {
    amcpDialogSaving.value = false
  }
}

// Delete assistant MCP
const amcpDeleteTarget = ref<AssistantMCP | null>(null)

function confirmDeleteAssistantMcp(item: AssistantMCP) {
  amcpDeleteTarget.value = item
}

async function handleDeleteAssistantMcp() {
  const item = amcpDeleteTarget.value
  if (!item) return
  amcpDeleteTarget.value = null
  try {
    await AssistantMCPService.Delete(item.id)
    assistantMcps.value = assistantMcps.value.filter((a) => a.id !== item.id)
    if (amcpDetail.value?.id === item.id) {
      goBackFromAmcpDetail()
    }
    toast.success(t('settings.mcp.assistantMcpDeleteSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.assistantMcpDeleteFailed'))
  }
}

// Tool entry type matching backend ToolEntry
interface ToolEntry {
  agentId: number
  toolName: string
  toolDescription: string
}

function parseTools(item: AssistantMCP): ToolEntry[] {
  try {
    const parsed = JSON.parse(item.tools || '[]')
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

// Link agents dialog
const linkAgentsDialogOpen = ref(false)
const linkAgentsTarget = ref<AssistantMCP | null>(null)
const allAgents = ref<Agent[]>([])
const selectedAgentIds = ref<Set<number>>(new Set())
const linkAgentsSaving = ref(false)

async function openLinkAgentsDialog(item: AssistantMCP) {
  linkAgentsTarget.value = item
  linkAgentsDialogOpen.value = true

  try {
    const agents = await AgentsService.ListAgents()
    allAgents.value = agents
    agentMap.value = new Map(agents.map((a) => [a.id, a]))
  } catch (error) {
    console.error('Failed to load agents:', error)
    allAgents.value = []
  }

  const existingTools = parseTools(item)
  selectedAgentIds.value = new Set(existingTools.map((t) => t.agentId))
}

function toggleAgentSelection(agentId: number) {
  const newSet = new Set(selectedAgentIds.value)
  if (newSet.has(agentId)) {
    newSet.delete(agentId)
  } else {
    newSet.add(agentId)
  }
  selectedAgentIds.value = newSet
}

async function handleSaveLinkAgents() {
  const target = linkAgentsTarget.value
  if (!target) return

  linkAgentsSaving.value = true
  try {
    const ids = Array.from(selectedAgentIds.value)
    const updated = await AssistantMCPService.AddTools({ id: target.id, agentIds: ids })
    if (updated) {
      const idx = assistantMcps.value.findIndex((a) => a.id === target.id)
      if (idx >= 0) assistantMcps.value[idx] = updated
      if (amcpDetail.value?.id === target.id) amcpDetail.value = updated
    }
    linkAgentsDialogOpen.value = false
    toast.success(t('settings.mcp.assistantMcpUpdateSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.assistantMcpUpdateFailed'))
  } finally {
    linkAgentsSaving.value = false
  }
}

// Detail view for assistant MCP
const amcpDetail = ref<AssistantMCP | null>(null)
const editingTool = ref<ToolEntry | null>(null)
const editToolForm = ref({ toolName: '', toolDescription: '' })
const editToolSaving = ref(false)
const removingToolAgentId = ref<number | null>(null)
const connectionInfo = ref<{ url: string; authorization: string } | null>(null)
const agentMap = ref<Map<number, Agent>>(new Map())

async function showAmcpDetail(item: AssistantMCP) {
  amcpDetail.value = item
  connectionInfo.value = null
  try {
    connectionInfo.value = await AssistantMCPService.GetConnectionInfo(item.id)
  } catch {
    /* ignore */
  }
  try {
    const agents = await AgentsService.ListAgents()
    agentMap.value = new Map(agents.map((a) => [a.id, a]))
  } catch {
    /* ignore */
  }
}

function goBackFromAmcpDetail() {
  amcpDetail.value = null
  editingTool.value = null
  removingToolAgentId.value = null
  connectionInfo.value = null
  activeSubTab.value = 'assistantMcp'
}

function startEditTool(tool: ToolEntry) {
  editingTool.value = tool
  editToolForm.value = { toolName: tool.toolName, toolDescription: tool.toolDescription }
}

function cancelEditTool() {
  editingTool.value = null
}

async function handleSaveEditTool() {
  if (!amcpDetail.value || !editingTool.value) return
  editToolSaving.value = true
  try {
    const updated = await AssistantMCPService.UpdateTool({
      id: amcpDetail.value.id,
      agentId: editingTool.value.agentId,
      toolName: editToolForm.value.toolName.trim(),
      toolDescription: editToolForm.value.toolDescription.trim(),
    })
    if (updated) {
      amcpDetail.value = updated
      const idx = assistantMcps.value.findIndex((a) => a.id === updated.id)
      if (idx >= 0) assistantMcps.value[idx] = updated
    }
    editingTool.value = null
    toast.success(t('settings.mcp.assistantMcpUpdateSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.assistantMcpUpdateFailed'))
  } finally {
    editToolSaving.value = false
  }
}

async function handleRemoveTool(tool: ToolEntry) {
  if (!amcpDetail.value || removingToolAgentId.value !== null) return
  removingToolAgentId.value = tool.agentId
  try {
    const updated = await AssistantMCPService.RemoveTool({
      id: amcpDetail.value.id,
      agentId: tool.agentId,
    })
    if (updated) {
      amcpDetail.value = updated
      const idx = assistantMcps.value.findIndex((a) => a.id === updated.id)
      if (idx >= 0) assistantMcps.value[idx] = updated
    }
    if (editingTool.value?.agentId === tool.agentId) {
      editingTool.value = null
    }
    toast.success(t('settings.mcp.assistantMcpDeleteSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.mcp.assistantMcpUpdateFailed'))
  } finally {
    removingToolAgentId.value = null
  }
}

// ==================== Init ====================
let unsubOpenAdd: (() => void) | null = null

onMounted(() => {
  void loadServers()
  void loadSettings()
  void loadAssistantMcps()

  unsubOpenAdd = Events.On('mcp:open-add-dialog', () => {
    activeTopTab.value = 'servers'
    activeSubTab.value = 'installed'
    openAddDialog()
  })
})

onUnmounted(() => {
  unsubOpenAdd?.()
  unsubOpenAdd = null
})
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-hidden bg-background text-foreground">
    <!-- Top tab bar: MCP | Settings -->
    <div class="flex items-center border-b border-border px-4 py-2">
      <div class="inline-flex rounded-lg bg-muted p-1">
        <button
          v-for="tab in [
            { key: 'servers' as TopTab, label: t('settings.mcp.tabServers') },
            { key: 'settings' as TopTab, label: t('settings.mcp.tabSettings') },
          ]"
          :key="tab.key"
          type="button"
          :class="
            cn(
              'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
              activeTopTab === tab.key
                ? 'bg-background text-foreground shadow-sm'
                : 'text-foreground'
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
          <div
            class="flex shrink-0 items-start justify-between gap-4 border-b border-border px-4 py-3"
          >
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="text-base font-semibold text-foreground">{{ detailServer.name }}</span>
                <Badge
                  variant="secondary"
                  class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
                >
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
              v-for="tab in [
                {
                  key: 'tools' as DetailTab,
                  label: t('settings.mcp.tabTools'),
                  icon: Wrench,
                  count: detailResult?.tools?.length ?? 0,
                },
                {
                  key: 'prompts' as DetailTab,
                  label: t('settings.mcp.tabPrompts'),
                  icon: MessageSquare,
                  count: detailResult?.prompts?.length ?? 0,
                },
                {
                  key: 'resources' as DetailTab,
                  label: t('settings.mcp.tabResources'),
                  icon: Database,
                  count: detailResult?.resources?.length ?? 0,
                },
              ]"
              :key="tab.key"
              :class="
                cn(
                  'inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  detailTab === tab.key
                    ? 'bg-foreground text-background'
                    : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground'
                )
              "
              @click="detailTab = tab.key"
            >
              <component :is="tab.icon" class="size-3" />
              {{ tab.label }}
              <Badge
                v-if="detailResult && tab.count > 0"
                variant="secondary"
                :class="
                  cn(
                    'ml-0.5 px-1.5 py-0 text-[10px]',
                    detailTab === tab.key
                      ? 'bg-background/25 text-background'
                      : 'bg-foreground/10 text-muted-foreground'
                  )
                "
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
                <div
                  v-if="detailResult.tools.length === 0"
                  class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
                >
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
                    <p
                      v-if="tool.description"
                      class="mt-1 pl-5.5 text-xs leading-relaxed text-muted-foreground"
                    >
                      {{ tool.description }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Prompts -->
              <div v-if="detailTab === 'prompts'">
                <div
                  v-if="detailResult.prompts.length === 0"
                  class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
                >
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
                    <p
                      v-if="prompt.description"
                      class="mt-1 pl-5.5 text-xs leading-relaxed text-muted-foreground"
                    >
                      {{ prompt.description }}
                    </p>
                  </div>
                </div>
              </div>

              <!-- Resources -->
              <div v-if="detailTab === 'resources'">
                <div
                  v-if="detailResult.resources.length === 0"
                  class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
                >
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
                      <Badge
                        v-if="res.mimeType"
                        variant="secondary"
                        class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
                      >
                        {{ res.mimeType }}
                      </Badge>
                    </div>
                    <p class="mt-1 pl-5.5 text-xs text-muted-foreground/70">{{ res.uri }}</p>
                    <p
                      v-if="res.description"
                      class="mt-0.5 pl-5.5 text-xs leading-relaxed text-muted-foreground"
                    >
                      {{ res.description }}
                    </p>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </template>

      <!-- ==================== Assistant MCP Detail View ==================== -->
      <template v-else-if="amcpDetail">
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div class="flex shrink-0 items-center border-b border-border px-4 py-2">
            <button
              class="inline-flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              @click="goBackFromAmcpDetail"
            >
              <ChevronLeft class="size-4" />
              {{ t('settings.mcp.tabAssistantMcp') }}
            </button>
          </div>
          <div
            class="flex shrink-0 items-start justify-between gap-4 border-b border-border px-4 py-3"
          >
            <div class="min-w-0 flex-1">
              <span class="text-base font-semibold text-foreground">{{ amcpDetail.name }}</span>
              <p class="mt-1 text-xs leading-relaxed text-muted-foreground">
                {{ amcpDetail.description }}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <button
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="openEditAssistantMcpDialog(amcpDetail)"
              >
                <Pencil class="size-3.5" />
                {{ t('settings.mcp.assistantMcpEdit') }}
              </button>
              <button
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="openLinkAgentsDialog(amcpDetail)"
              >
                <Plus class="size-3.5" />
                {{ t('settings.mcp.assistantMcpAddTool') }}
              </button>
            </div>
          </div>
          <div class="shrink-0 border-b border-border px-4 py-3">
            <div class="rounded-md border border-border bg-muted/30 p-3">
              <div class="flex flex-col gap-3 text-xs">
                <div class="flex flex-col gap-1">
                  <span class="text-muted-foreground">{{ t('settings.mcp.assistantMcpUrl') }}</span>
                  <code
                    class="rounded bg-background px-2 py-1 font-mono text-foreground select-text"
                    >{{ connectionInfo?.url || `http://127.0.0.1:${amcpDetail.port}/mcp` }}</code
                  >
                </div>
                <div class="flex flex-col gap-1">
                  <span class="text-muted-foreground">{{
                    t('settings.mcp.assistantMcpAuth')
                  }}</span>
                  <code
                    class="break-all rounded bg-background px-2 py-1 font-mono text-foreground select-text"
                    >{{
                      connectionInfo?.authorization || `Authorization: Bearer ${amcpDetail.token}`
                    }}</code
                  >
                </div>
              </div>
            </div>
          </div>
          <div class="flex-1 overflow-auto px-4 py-3">
            <div
              v-if="parseTools(amcpDetail).length === 0"
              class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
            >
              <Wrench class="size-8 opacity-40" />
              <span class="text-sm">{{ t('settings.mcp.noTools') }}</span>
            </div>
            <div v-else class="flex flex-col gap-1">
              <div
                v-for="tool in parseTools(amcpDetail)"
                :key="tool.agentId"
                class="rounded-md border border-border p-3 dark:border-white/10"
              >
                <template v-if="editingTool && editingTool.agentId === tool.agentId">
                  <div class="flex flex-col gap-2">
                    <div class="flex flex-col gap-1">
                      <Label class="text-xs">{{ t('settings.mcp.assistantMcpToolName') }}</Label>
                      <Input
                        v-model="editToolForm.toolName"
                        class="font-mono text-xs"
                        :placeholder="t('settings.mcp.assistantMcpToolNamePlaceholder')"
                      />
                    </div>
                    <div class="flex flex-col gap-1">
                      <Label class="text-xs">{{ t('settings.mcp.assistantMcpToolDesc') }}</Label>
                      <p
                        class="rounded-md border border-input bg-muted/50 px-3 py-2 text-xs text-muted-foreground"
                      >
                        {{ editToolForm.toolDescription || '—' }}
                      </p>
                    </div>
                    <div class="flex items-center justify-end gap-2">
                      <button
                        class="rounded-md px-3 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                        @click="cancelEditTool"
                      >
                        {{ t('common.cancel') }}
                      </button>
                      <button
                        class="rounded-md bg-foreground px-3 py-1 text-xs font-medium text-background transition-opacity hover:opacity-80 disabled:opacity-50"
                        :disabled="editToolSaving || !editToolForm.toolName.trim()"
                        @click="handleSaveEditTool"
                      >
                        <Loader2 v-if="editToolSaving" class="mr-1 inline size-3 animate-spin" />
                        {{ t('common.save') }}
                      </button>
                    </div>
                  </div>
                </template>
                <template v-else>
                  <div class="flex items-center justify-between gap-2">
                    <div class="flex items-center gap-2">
                      <div
                        class="flex size-5 shrink-0 items-center justify-center overflow-hidden rounded border border-border bg-white dark:border-white/15 dark:bg-white/5"
                      >
                        <img
                          v-if="agentMap.get(tool.agentId)?.icon"
                          :src="agentMap.get(tool.agentId)!.icon"
                          class="size-4 object-contain"
                        />
                        <img v-else :src="logoSrc" class="size-4 opacity-90" alt="ChatClaw logo" />
                      </div>
                      <span class="font-mono text-sm font-medium text-foreground">{{
                        tool.toolName
                      }}</span>
                    </div>
                    <div class="flex items-center gap-1">
                      <button
                        class="inline-flex cursor-pointer items-center justify-center rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                        @click="startEditTool(tool)"
                      >
                        <Pencil class="size-3" />
                      </button>
                      <button
                        :disabled="removingToolAgentId === tool.agentId"
                        class="inline-flex cursor-pointer items-center justify-center rounded-md p-1 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                        @click="handleRemoveTool(tool)"
                      >
                        <Loader2
                          v-if="removingToolAgentId === tool.agentId"
                          class="size-3 animate-spin"
                        />
                        <Trash2 v-else class="size-3" />
                      </button>
                    </div>
                  </div>
                  <p
                    v-if="tool.toolDescription"
                    class="mt-1 pl-7 text-xs leading-relaxed text-muted-foreground"
                  >
                    {{ tool.toolDescription }}
                  </p>
                </template>
              </div>
            </div>
          </div>
        </div>
      </template>

      <!-- ==================== List View ==================== -->
      <template v-else>
        <!-- Sub tab bar: Installed | Market + Add button -->
        <div class="flex items-center justify-between px-4 py-2">
          <div class="flex gap-1">
            <button
              v-for="subTab in [
                { key: 'installed' as SubTab, label: t('settings.mcp.tabInstalled') },
                { key: 'assistantMcp' as SubTab, label: t('settings.mcp.tabAssistantMcp') },
                { key: 'market' as SubTab, label: t('settings.mcp.tabMarket') },
              ]"
              :key="subTab.key"
              :class="
                cn(
                  'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                  activeSubTab === subTab.key
                    ? 'bg-foreground text-background'
                    : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground'
                )
              "
              @click="activeSubTab = subTab.key"
            >
              {{ subTab.label }}
            </button>
          </div>
          <div class="flex min-h-[28px] items-center gap-1">
            <template v-if="activeSubTab === 'installed'">
              <button
                class="inline-flex cursor-pointer items-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="validating"
                :title="t('settings.mcp.validateAll')"
                @click="handleValidateAll"
              >
                <RefreshCw class="size-3.5" :class="validating && 'animate-spin'" />
              </button>
              <button
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="openAddDialog"
              >
                <Plus class="size-3.5" />
                {{ t('settings.mcp.addServer') }}
              </button>
            </template>
            <template v-else-if="activeSubTab === 'assistantMcp'">
              <button
                class="inline-flex cursor-pointer items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="openAddAssistantMcpDialog"
              >
                <Plus class="size-3.5" />
                {{ t('settings.mcp.assistantMcpAdd') }}
              </button>
            </template>
          </div>
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
                <Badge
                  variant="secondary"
                  class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
                >
                  <Terminal v-if="server.transport === 'stdio'" class="mr-0.5 size-2.5" />
                  <Globe v-else class="mr-0.5 size-2.5" />
                  {{ server.transport === 'stdio' ? 'stdio' : 'HTTP' }}
                </Badge>
              </div>
              <p
                class="mt-1.5 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground"
              >
                {{ serverSummary(server) }}
              </p>
              <div class="mt-auto flex items-center justify-between gap-2 pt-3">
                <span class="text-[10px] text-muted-foreground">{{ server.timeout }}s timeout</span>
                <div class="flex items-center gap-2" @click.stop>
                  <Switch
                    :model-value="server.enabled"
                    :disabled="!mcpEnabled"
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
              class="group flex cursor-pointer items-center gap-3 rounded-lg border border-border p-3.5 transition-colors hover:bg-accent/30 dark:border-white/10"
              @click="openMarketLink(item.url)"
            >
              <img
                :src="item.logo"
                :alt="item.name"
                class="size-8 shrink-0 rounded-md object-contain"
              />
              <div class="min-w-0 flex-1">
                <span class="text-sm font-medium text-foreground">{{ item.name }}</span>
                <p class="mt-0.5 text-xs text-muted-foreground">{{ item.desc }}</p>
              </div>
              <ExternalLink
                class="size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground"
              />
            </div>
          </div>
        </div>

        <!-- Assistant MCP tab -->
        <template v-if="activeSubTab === 'assistantMcp'">
          <div class="flex-1 overflow-auto px-4 pb-4">
            <div
              v-if="assistantMcpsLoading && assistantMcps.length === 0"
              class="flex items-center justify-center py-12"
            >
              <Loader2 class="size-5 animate-spin text-muted-foreground" />
            </div>
            <div
              v-else-if="assistantMcps.length === 0"
              class="flex flex-col items-center justify-center gap-3 py-12 text-muted-foreground"
            >
              <Package class="size-8 opacity-40" />
              <span class="text-sm">{{ t('settings.mcp.assistantMcpNoItems') }}</span>
              <button
                class="mt-1 inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border bg-background px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-accent"
                @click="openAddAssistantMcpDialog"
              >
                <Plus class="size-3.5" />
                {{ t('settings.mcp.assistantMcpAdd') }}
              </button>
            </div>
            <div v-else class="space-y-3">
              <p
                v-if="!mcpEnabled && assistantMcps.length > 0"
                class="text-xs text-muted-foreground"
              >
                {{ t('assistant.workspaceDrawer.mcpGlobalDisabled') }}
              </p>
              <div class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
                <div
                  v-for="amcp in assistantMcps"
                  :key="amcp.id"
                  class="group flex cursor-pointer flex-col rounded-lg border border-border p-3.5 transition-colors hover:bg-accent/30 dark:border-white/10"
                  @click="showAmcpDetail(amcp)"
                >
                  <div class="flex items-center justify-between gap-2">
                    <span class="truncate text-sm font-medium text-foreground">{{
                      amcp.name
                    }}</span>
                    <div @click.stop>
                      <Switch
                        :model-value="amcp.enabled"
                        :disabled="!mcpEnabled"
                        class="scale-90"
                        @update:model-value="() => handleToggleAssistantMcp(amcp)"
                      />
                    </div>
                  </div>
                  <p
                    class="mt-1.5 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground"
                  >
                    {{ amcp.description }}
                  </p>
                  <div class="mt-auto flex items-center justify-between gap-2 pt-3">
                    <span
                      v-if="parseTools(amcp).length > 0"
                      class="text-[10px] text-muted-foreground"
                    >
                      {{
                        t('settings.mcp.assistantMcpToolCount', { count: parseTools(amcp).length })
                      }}
                    </span>
                    <span v-else />
                    <div @click.stop>
                      <DropdownMenu>
                        <DropdownMenuTrigger as-child>
                          <button
                            class="inline-flex cursor-pointer items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                          >
                            <MoreHorizontal class="size-3.5" />
                          </button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" class="w-36">
                          <DropdownMenuItem @click="openLinkAgentsDialog(amcp)">
                            <Plus class="mr-2 size-3.5" />
                            {{ t('settings.mcp.assistantMcpAddTool') }}
                          </DropdownMenuItem>
                          <DropdownMenuItem @click="openEditAssistantMcpDialog(amcp)">
                            <Pencil class="mr-2 size-3.5" />
                            {{ t('settings.mcp.assistantMcpEdit') }}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            class="text-destructive focus:text-destructive"
                            @click="confirmDeleteAssistantMcp(amcp)"
                          >
                            <Trash2 class="mr-2 size-3.5" />
                            {{ t('settings.mcp.assistantMcpDelete') }}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </template>
    </template>

    <!-- ==================== Settings Tab ==================== -->
    <template v-if="activeTopTab === 'settings'">
      <div class="flex flex-1 flex-col items-center overflow-auto py-8">
        <SettingsCard :title="t('settings.mcp.title')">
          <SettingsItem>
            <template #label>
              <div class="flex flex-col gap-1">
                <span class="text-sm font-medium text-foreground">{{
                  t('settings.mcp.enable')
                }}</span>
                <span class="text-xs text-muted-foreground">{{
                  t('settings.mcp.enableHint')
                }}</span>
              </div>
            </template>
            <Switch :model-value="mcpEnabled" @update:model-value="handleMCPEnabledChange" />
          </SettingsItem>
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

          <!-- Description -->
          <div class="flex flex-col gap-1.5">
            <div class="flex items-center justify-between">
              <Label class="text-sm">{{ t('settings.mcp.description') }}</Label>
              <span class="text-[10px] text-muted-foreground"
                >{{ dialogForm.description.length }}/300</span
              >
            </div>
            <textarea
              v-model="dialogForm.description"
              :placeholder="t('settings.mcp.descriptionPlaceholder')"
              :maxlength="300"
              rows="2"
              class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 resize-none"
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
                <SelectItem value="streamableHttp">{{
                  t('settings.mcp.transportHttp')
                }}</SelectItem>
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
              <div class="flex items-center justify-between">
                <Label class="text-sm">{{ t('settings.mcp.envVars') }}</Label>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                  @click="addPair(dialogForm.envPairs)"
                >
                  <Plus class="size-3" />
                  {{ t('settings.mcp.addRow') }}
                </button>
              </div>
              <div
                v-if="dialogForm.envPairs.length === 0"
                class="text-xs text-muted-foreground py-1"
              >
                {{ t('settings.mcp.envVarsPlaceholder') }}
              </div>
              <div
                v-for="(pair, idx) in dialogForm.envPairs"
                :key="idx"
                class="flex items-center gap-2"
              >
                <Input v-model="pair.key" placeholder="KEY" class="flex-1 font-mono text-xs" />
                <span class="text-muted-foreground text-xs">=</span>
                <Input v-model="pair.value" placeholder="VALUE" class="flex-1 font-mono text-xs" />
                <button
                  type="button"
                  class="shrink-0 rounded p-0.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                  @click="removePair(dialogForm.envPairs, idx)"
                >
                  <X class="size-3.5" />
                </button>
              </div>
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
              <div class="flex items-center justify-between">
                <Label class="text-sm">{{ t('settings.mcp.httpHeaders') }}</Label>
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-xs text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                  @click="addPair(dialogForm.headerPairs)"
                >
                  <Plus class="size-3" />
                  {{ t('settings.mcp.addRow') }}
                </button>
              </div>
              <div
                v-if="dialogForm.headerPairs.length === 0"
                class="text-xs text-muted-foreground py-1"
              >
                {{ t('settings.mcp.httpHeadersPlaceholder') }}
              </div>
              <div
                v-for="(pair, idx) in dialogForm.headerPairs"
                :key="idx"
                class="flex items-center gap-2"
              >
                <Input
                  v-model="pair.key"
                  placeholder="Header-Name"
                  class="flex-1 font-mono text-xs"
                />
                <span class="text-muted-foreground text-xs">:</span>
                <Input v-model="pair.value" placeholder="Value" class="flex-1 font-mono text-xs" />
                <button
                  type="button"
                  class="shrink-0 rounded p-0.5 text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
                  @click="removePair(dialogForm.headerPairs, idx)"
                >
                  <X class="size-3.5" />
                </button>
              </div>
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
            <Loader2
              v-if="dialogTesting || dialogSaving"
              class="mr-1.5 inline size-3.5 animate-spin"
            />
            <template v-if="dialogTesting">{{ t('settings.mcp.testing') }}</template>
            <template v-else>{{
              dialogMode === 'add' ? t('settings.mcp.addServer') : t('common.save')
            }}</template>
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
          <AlertDialogCancel @click="deleteTarget = null">{{
            t('common.cancel')
          }}</AlertDialogCancel>
          <AlertDialogAction @click="handleDelete">{{ t('common.confirm') }}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- ==================== Assistant MCP Add/Edit Dialog ==================== -->
    <Dialog v-model:open="amcpDialogOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {{
              amcpDialogMode === 'add'
                ? t('settings.mcp.assistantMcpAddTitle')
                : t('settings.mcp.assistantMcpEditTitle')
            }}
          </DialogTitle>
          <DialogDescription class="sr-only">
            {{
              amcpDialogMode === 'add'
                ? t('settings.mcp.assistantMcpAddTitle')
                : t('settings.mcp.assistantMcpEditTitle')
            }}
          </DialogDescription>
        </DialogHeader>

        <div class="flex flex-col gap-4 py-2">
          <div class="flex flex-col gap-1.5">
            <Label class="text-sm">{{ t('settings.mcp.assistantMcpName') }}</Label>
            <Input
              v-model="amcpDialogForm.name"
              :placeholder="t('settings.mcp.assistantMcpNamePlaceholder')"
              :maxlength="20"
            />
          </div>

          <div class="flex flex-col gap-1.5">
            <Label class="text-sm">{{ t('settings.mcp.assistantMcpDescription') }}</Label>
            <textarea
              v-model="amcpDialogForm.description"
              :placeholder="t('settings.mcp.assistantMcpDescriptionPlaceholder')"
              :maxlength="300"
              rows="2"
              class="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 resize-none"
            />
          </div>

          <div class="rounded-md border border-border bg-muted/30 p-3">
            <div class="flex flex-col gap-2 text-xs">
              <div class="flex flex-col gap-1">
                <span class="text-muted-foreground">{{ t('settings.mcp.assistantMcpUrl') }}</span>
                <code
                  v-if="amcpDialogMode === 'edit'"
                  class="rounded bg-background px-2 py-1 font-mono text-foreground"
                  >http://127.0.0.1:{{ amcpDialogForm.port }}/mcp</code
                >
                <span v-else class="text-muted-foreground/70">{{
                  t('settings.mcp.assistantMcpAutoPort')
                }}</span>
              </div>
              <div class="flex items-center justify-between">
                <span class="text-muted-foreground">{{ t('settings.mcp.assistantMcpAuth') }}</span>
                <span class="text-foreground">Bearer Token</span>
              </div>
            </div>
          </div>
        </div>

        <DialogFooter>
          <button
            class="cursor-pointer rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-80 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!amcpCanSave || amcpDialogSaving"
            @click="handleSaveAssistantMcp"
          >
            <Loader2 v-if="amcpDialogSaving" class="mr-1.5 inline size-3.5 animate-spin" />
            {{ amcpDialogMode === 'add' ? t('settings.mcp.assistantMcpAdd') : t('common.save') }}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- ==================== Assistant MCP Delete Confirm ==================== -->
    <AlertDialog :open="!!amcpDeleteTarget">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('common.delete') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.mcp.assistantMcpDeleteConfirm') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel @click="amcpDeleteTarget = null">{{
            t('common.cancel')
          }}</AlertDialogCancel>
          <AlertDialogAction @click="handleDeleteAssistantMcp">{{
            t('common.confirm')
          }}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- ==================== Link Agents Dialog ==================== -->
    <Dialog v-model:open="linkAgentsDialogOpen">
      <DialogContent size="xl">
        <DialogHeader>
          <DialogTitle>{{ t('settings.mcp.assistantMcpLinkAgentsTitle') }}</DialogTitle>
          <DialogDescription>{{ t('settings.mcp.assistantMcpLinkAgentsDesc') }}</DialogDescription>
        </DialogHeader>

        <div class="max-h-[420px] overflow-auto py-2">
          <div
            v-if="allAgents.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-8 text-muted-foreground"
          >
            <Package class="size-6 opacity-40" />
            <span class="text-sm">{{ t('settings.mcp.assistantMcpNoAgents') }}</span>
          </div>
          <div v-else class="grid grid-cols-2 gap-2">
            <div
              v-for="agent in allAgents"
              :key="agent.id"
              :class="
                cn(
                  'flex cursor-pointer items-center gap-2.5 rounded-lg border p-3 transition-colors hover:bg-accent/50',
                  selectedAgentIds.has(agent.id)
                    ? 'border-foreground/40 bg-accent/30'
                    : 'border-border'
                )
              "
              @click="toggleAgentSelection(agent.id)"
            >
              <div
                class="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded-md border border-border bg-white dark:border-white/15 dark:bg-white/5"
              >
                <img
                  v-if="agent.icon"
                  :src="agent.icon"
                  :alt="agent.name"
                  class="size-6 object-contain"
                />
                <img v-else :src="logoSrc" class="size-6 opacity-90" alt="ChatClaw logo" />
              </div>
              <span class="flex-1 truncate text-sm text-foreground">{{ agent.name }}</span>
              <div
                :class="
                  cn(
                    'flex size-4 shrink-0 items-center justify-center rounded border transition-colors',
                    selectedAgentIds.has(agent.id)
                      ? 'border-foreground bg-foreground text-background'
                      : 'border-border'
                  )
                "
              >
                <Check v-if="selectedAgentIds.has(agent.id)" class="size-3" />
              </div>
            </div>
          </div>
        </div>

        <DialogFooter>
          <button
            class="cursor-pointer rounded-md bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-80 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="linkAgentsSaving"
            @click="handleSaveLinkAgents"
          >
            <Loader2 v-if="linkAgentsSaving" class="mr-1.5 inline size-3.5 animate-spin" />
            {{ t('common.save') }}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </div>
</template>
