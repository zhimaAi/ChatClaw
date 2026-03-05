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

  dialogSaving.value = true
  try {
    if (dialogMode.value === 'add') {
      await MCPService.AddServer({
        name: form.name.trim(),
        transport: form.transport,
        command: form.command.trim(),
        args: parseLinesToArray(form.argsText),
        env: parseLinesToObject(form.envText),
        url: form.url.trim(),
        headers: parseLinesToObject(form.headersText),
        timeout: form.timeout,
      })
      toast.success(t('settings.mcp.addSuccess'))
    } else {
      await MCPService.UpdateServer({
        id: form.id,
        name: form.name.trim(),
        transport: form.transport,
        command: form.command.trim(),
        args: parseLinesToArray(form.argsText),
        env: parseLinesToObject(form.envText),
        url: form.url.trim(),
        headers: parseLinesToObject(form.headersText),
        timeout: form.timeout,
      })
      toast.success(t('settings.mcp.updateSuccess'))
    }
    dialogOpen.value = false
    void loadServers()
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
            class="cursor-not-allowed rounded-md bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground/50"
            disabled
          >
            {{ t('settings.mcp.tabMarket') }}
          </button>
        </div>
        <button
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
        <div v-else class="flex flex-col gap-2">
          <div
            v-for="server in servers"
            :key="server.id"
            class="group rounded-lg border border-border p-3 transition-colors hover:bg-accent/30 dark:border-white/10"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-foreground">{{ server.name }}</span>
                  <Badge variant="secondary" class="bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                    <Terminal v-if="server.transport === 'stdio'" class="mr-0.5 size-2.5" />
                    <Globe v-else class="mr-0.5 size-2.5" />
                    {{ server.transport === 'stdio' ? 'stdio' : 'HTTP' }}
                  </Badge>
                </div>
                <p class="mt-1 truncate text-xs text-muted-foreground">
                  {{ serverSummary(server) }}
                </p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <Switch
                  :model-value="server.enabled"
                  class="scale-90"
                  @update:model-value="() => handleToggleServer(server)"
                />
                <button
                  class="inline-flex cursor-pointer items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  @click="openEditDialog(server)"
                >
                  <Pencil class="size-3.5" />
                </button>
                <button
                  class="inline-flex cursor-pointer items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive"
                  @click="confirmDelete(server)"
                >
                  <Trash2 class="size-3.5" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Market tab (disabled placeholder) -->
      <div v-if="activeSubTab === 'market'" class="flex flex-1 items-center justify-center">
        <span class="text-sm text-muted-foreground">{{ t('settings.mcp.marketComingSoon') }}</span>
      </div>
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
      <DialogContent class="sm:max-w-md">
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
            :disabled="!canSave || dialogSaving"
            @click="handleDialogSave"
          >
            <Loader2 v-if="dialogSaving" class="mr-1.5 inline size-3.5 animate-spin" />
            {{ dialogMode === 'add' ? t('settings.mcp.addServer') : t('common.save') }}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- ==================== Delete Confirm ==================== -->
    <AlertDialog :open="!!deleteTarget" @update:open="(v) => !v && (deleteTarget = null)">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('common.delete') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.mcp.deleteConfirm') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="handleDelete">{{ t('common.confirm') }}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
