<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  RefreshCw,
  Search,
  Loader2,
  FolderOpen,
  Folders,
  ChevronLeft,
  FileText,
  FileCode,
  Settings,
  Plus,
} from 'lucide-vue-next'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { OpenClawSkillsService } from '@bindings/chatclaw/internal/openclaw/skills'
import type {
  OpenClawSkill,
  SkillFileInfo,
  SkillInstallation,
} from '@bindings/chatclaw/internal/openclaw/skills/models'
import { OpenClawRuntimeService } from '@bindings/chatclaw/internal/openclaw/runtime'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { Events } from '@wailsio/runtime'
import { useNavigationStore, type NavModule } from '@/stores'

const props = defineProps<{
  tabId: string
}>()

const { t } = useI18n()
const navigationStore = useNavigationStore()

type SkillsFilter = 'all' | 'builtin' | 'installed'

const skills = ref<OpenClawSkill[]>([])
const loading = ref(false)
const refreshing = ref(false)
const searchQuery = ref('')
const filter = ref<SkillsFilter>('all')
/** Default install target: workspace-main/skills (matches openclaw skills install). */
const mainWorkspaceSkillsRoot = ref('')
/** Optional managed overrides: …/openclaw/skills */
const managedSkillsRoot = ref('')
const gatewayConnected = ref(true)
const addDialogOpen = ref(false)

const detailOpen = ref(false)
const activeSkill = ref<OpenClawSkill | null>(null)
const detailFiles = ref<SkillFileInfo[]>([])
const selectedFilePath = ref('')
const fileContent = ref('')
const fileLoading = ref(false)
let fileLoadVersion = 0

const BINARY_EXTENSIONS = new Set([
  'png',
  'jpg',
  'jpeg',
  'gif',
  'webp',
  'ico',
  'bmp',
  'svg',
  'woff',
  'woff2',
  'ttf',
  'otf',
  'eot',
  'zip',
  'pdf',
  'doc',
  'docx',
  'mp3',
  'mp4',
])

function isBinaryFile(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() || ''
  return BINARY_EXTENSIONS.has(ext)
}

function isMarkdownFile(path: string): boolean {
  return /\.md$/i.test(path)
}

function getCodeLanguage(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() || ''
  const langMap: Record<string, string> = {
    js: 'javascript',
    ts: 'typescript',
    py: 'python',
    go: 'go',
    json: 'json',
    yaml: 'yaml',
    yml: 'yaml',
    md: 'markdown',
  }
  return langMap[ext] || ''
}

function stripFrontmatter(markdown: string): string {
  const trimmed = markdown.trimStart()
  if (!trimmed.startsWith('---')) return markdown
  const rest = trimmed.slice(3)
  const end = rest.indexOf('\n---')
  if (end === -1) return markdown
  return rest.slice(end + 4).trimStart()
}

const filteredSkills = computed(() => {
  let list = skills.value
  if (filter.value === 'builtin') {
    list = list.filter((s) => !isWorkspaceSkill(s))
  } else if (filter.value === 'installed') {
    list = list.filter((s) => isWorkspaceSkill(s))
  }
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter((s) => {
      const inst = s.installations ?? []
      const instHay = inst
        .map((i) =>
          `${i.openclawAgentId} ${i.agentName} ${i.skillRoot} ${i.layer} ${i.location}`.toLowerCase()
        )
        .join(' ')
      return (
        s.slug.toLowerCase().includes(q) ||
        (s.name && s.name.toLowerCase().includes(q)) ||
        (s.description && s.description.toLowerCase().includes(q)) ||
        (s.agentId && s.agentId.toLowerCase().includes(q)) ||
        (s.agentName && s.agentName.toLowerCase().includes(q)) ||
        (s.skillRoot && s.skillRoot.toLowerCase().includes(q)) ||
        (s.permission && s.permission.toLowerCase().includes(q)) ||
        (s.scope && s.scope.toLowerCase().includes(q)) ||
        (s.dataSource && s.dataSource.toLowerCase().includes(q)) ||
        (s.ineligibleReason && s.ineligibleReason.toLowerCase().includes(q)) ||
        instHay.includes(q)
      )
    })
  }
  return list
})

function isWorkspaceSkill(s: OpenClawSkill): boolean {
  if (s.location === 'workspace') return true
  const inst = s.installations ?? []
  return inst.some((i) => i.location === 'workspace')
}

function pickWorkspaceInstallation(s: OpenClawSkill): SkillInstallation | null {
  const inst = s.installations ?? []
  return inst.find((i) => i.location === 'workspace') ?? null
}

function pickPrimaryInstallation(s: OpenClawSkill): SkillInstallation | null {
  const inst = s.installations ?? []
  if (inst.length === 0) return null
  return pickWorkspaceInstallation(s) ?? inst[0]
}

function skillSourceTag(s: OpenClawSkill): string {
  const ins = pickPrimaryInstallation(s)
  const layer = ins?.layer || s.dataSource || 'gateway'
  return `openclaw-${layer}`
}

const displayedSkills = computed(() => {
  const list = filteredSkills.value.slice()
  if (filter.value === 'all') {
    list.sort((a, b) => {
      const aw = isWorkspaceSkill(a)
      const bw = isWorkspaceSkill(b)
      if (aw !== bw) return aw ? -1 : 1
      const as = (a.slug || a.name || '').toLowerCase()
      const bs = (b.slug || b.name || '').toLowerCase()
      if (as < bs) return -1
      if (as > bs) return 1
      return 0
    })
  }
  return list
})

const renderedFileContent = computed(() => {
  if (!fileContent.value || !selectedFilePath.value) return ''
  if (isMarkdownFile(selectedFilePath.value)) {
    if (selectedFilePath.value === 'SKILL.md') {
      return stripFrontmatter(fileContent.value)
    }
    return fileContent.value
  }
  return fileContent.value
})

const isSelectedBinary = computed(() => isBinaryFile(selectedFilePath.value))
const isSelectedMarkdown = computed(() => isMarkdownFile(selectedFilePath.value))

const fileContentAsMarkdown = computed(() => {
  if (!renderedFileContent.value) return ''
  const lang = getCodeLanguage(selectedFilePath.value) || 'text'
  return '```' + lang + '\n' + renderedFileContent.value + '\n```'
})

function locationLabel(s: OpenClawSkill): string {
  return s.location === 'shared'
    ? t('settings.openclawSkills.locationShared')
    : t('settings.openclawSkills.locationWorkspace')
}

function agentBindingLabel(s: OpenClawSkill): string {
  if (s.location !== 'workspace') return ''
  const name = s.agentName?.trim()
  if (name) return `${name} (${s.agentId})`
  return s.agentId || ''
}

function installationLayerLabel(layer: string): string {
  switch (layer) {
    case 'gateway':
      return t('settings.openclawSkills.dataSourceGateway')
    case 'managed':
      return t('settings.openclawSkills.dataSourceManaged')
    case 'bundled':
      return t('settings.openclawSkills.dataSourceBundled')
    case 'extra':
      return t('settings.openclawSkills.dataSourceExtra')
    case 'workspace':
      return t('settings.openclawSkills.dataSourceWorkspace')
    default:
      return layer || t('common.na')
  }
}

function installationAgentLine(i: SkillInstallation): string {
  const name = i.agentName?.trim()
  const id = i.openclawAgentId?.trim()
  if (name && id) return `${name} (${id})`
  return name || id || t('common.na')
}

function diskLocationBadge(loc: string): string {
  return loc === 'shared'
    ? t('settings.openclawSkills.locationShared')
    : t('settings.openclawSkills.locationWorkspace')
}

function dataSourceLabel(s: OpenClawSkill): string {
  switch (s.dataSource) {
    case 'gateway':
      return t('settings.openclawSkills.dataSourceGateway')
    case 'managed':
      return t('settings.openclawSkills.dataSourceManaged')
    case 'bundled':
      return t('settings.openclawSkills.dataSourceBundled')
    case 'extra':
      return t('settings.openclawSkills.dataSourceExtra')
    case 'workspace':
      return t('settings.openclawSkills.dataSourceWorkspace')
    default:
      return s.dataSource || t('common.na')
  }
}

function eligibleLabelText(s: OpenClawSkill): string {
  if (s.dataSource !== 'gateway') return t('settings.openclawSkills.eligibleUnknown')
  if (s.eligible === true) return t('settings.openclawSkills.eligibleYes')
  if (s.eligible === false) return t('settings.openclawSkills.eligibleNo')
  return t('settings.openclawSkills.eligibleUnknown')
}

function listAgentHint(s: OpenClawSkill): string {
  const ins = pickWorkspaceInstallation(s)
  if (!ins) return ''
  const name = ins.agentName?.trim()
  const id = ins.openclawAgentId?.trim()
  if (name && id) return `${name} (${id})`
  return name || id || ''
}

function openAddDialog() {
  addDialogOpen.value = true
}

function closeAddDialog() {
  addDialogOpen.value = false
}

function switchCurrentTabToModule(module: NavModule) {
  const tab = navigationStore.tabs.find((t) => t.id === props.tabId)
  if (!tab) {
    navigationStore.navigateToModule(module)
    return
  }
  tab.module = module
  tab.title = undefined
  tab.titleKey = module === 'openclaw' ? 'nav.openclaw' : tab.titleKey
  tab.icon = undefined
  tab.iconIsDefault = true
  navigationStore.activeModule = module
  navigationStore.setActiveTab(props.tabId)
}

function handleCreateViaChat() {
  closeAddDialog()
  switchCurrentTabToModule('openclaw')
  const prompt = t('settings.openclawSkills.add.createViaChatPrompt')
  window.setTimeout(() => {
    // OpenClaw page listens to this event to prefill chat input and auto-send.
    // Delay to ensure the OpenClaw page is mounted and subscribed.
    Events.Emit('text-selection:send-to-assistant', { text: prompt })
  }, 150)
}

async function handleChooseSkillPackage() {
  closeAddDialog()
  try {
    await BrowserService.OpenURL('https://clawhub.ai/skills?sort=downloads')
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
  await openMainWorkspaceSkillsFolder()
}

async function loadSkillInstallDirs() {
  try {
    mainWorkspaceSkillsRoot.value = await OpenClawSkillsService.GetSkillsRoot()
  } catch {
    mainWorkspaceSkillsRoot.value = ''
  }
  try {
    managedSkillsRoot.value = await OpenClawSkillsService.GetManagedSkillsRoot()
  } catch {
    managedSkillsRoot.value = ''
  }
}

async function loadGatewayHint() {
  try {
    const st = await OpenClawRuntimeService.GetGatewayState()
    gatewayConnected.value = !!st.connected
  } catch {
    gatewayConnected.value = false
  }
}

async function loadList() {
  loading.value = true
  try {
    skills.value = await OpenClawSkillsService.ListSkills()
  } catch (e) {
    toast.error(getErrorMessage(e) || t('settings.openclawSkills.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function handleRefresh() {
  refreshing.value = true
  const minDelay = new Promise((r) => setTimeout(r, 400))
  try {
    await Promise.all([loadList(), loadSkillInstallDirs(), loadGatewayHint(), minDelay])
  } finally {
    refreshing.value = false
  }
}

async function openMainWorkspaceSkillsFolder() {
  if (!mainWorkspaceSkillsRoot.value) await loadSkillInstallDirs()
  if (!mainWorkspaceSkillsRoot.value) return
  try {
    await BrowserService.OpenDirectory(mainWorkspaceSkillsRoot.value)
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

async function openManagedSkillsFolder() {
  if (!managedSkillsRoot.value) await loadSkillInstallDirs()
  if (!managedSkillsRoot.value) return
  try {
    await BrowserService.OpenDirectory(managedSkillsRoot.value)
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

async function openSkillDir(s: OpenClawSkill) {
  if (!s.skillRoot) return
  try {
    await BrowserService.OpenDirectory(s.skillRoot)
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

async function openInstallationDir(root: string) {
  if (!root) return
  try {
    await BrowserService.OpenDirectory(root)
  } catch (e) {
    toast.error(getErrorMessage(e))
  }
}

function navigateToOpenClawRuntimeSettings() {
  navigationStore.navigateToModule('openclaw-runtime')
}

async function openDetail(s: OpenClawSkill) {
  activeSkill.value = s
  detailOpen.value = true
  detailFiles.value = []
  selectedFilePath.value = ''
  fileContent.value = ''
  if (s.skillRoot) {
    try {
      detailFiles.value = await OpenClawSkillsService.ListSkillFiles(s.skillRoot)
    } catch {
      detailFiles.value = []
    }
  } else {
    detailFiles.value = []
  }
  const hasSkillMd = detailFiles.value.some((f) => f.path === 'SKILL.md')
  if (hasSkillMd) {
    await selectFile('SKILL.md')
  } else if (detailFiles.value.length > 0) {
    await selectFile(detailFiles.value[0].path)
  }
}

function closeDetail() {
  detailOpen.value = false
  activeSkill.value = null
  detailFiles.value = []
  selectedFilePath.value = ''
  fileContent.value = ''
}

async function selectFile(path: string) {
  if (!activeSkill.value?.skillRoot) return
  selectedFilePath.value = path
  if (isBinaryFile(path)) {
    fileContent.value = ''
    return
  }
  const ver = ++fileLoadVersion
  fileLoading.value = true
  try {
    const text = await OpenClawSkillsService.ReadSkillFile(activeSkill.value.skillRoot, path)
    if (ver !== fileLoadVersion) return
    fileContent.value = text
  } catch (e) {
    if (ver !== fileLoadVersion) return
    fileContent.value = ''
    toast.error(getErrorMessage(e) || t('settings.skills.loadFailed'))
  } finally {
    if (ver === fileLoadVersion) fileLoading.value = false
  }
}

watch(detailOpen, (open) => {
  if (!open) {
    fileLoadVersion++
  }
})

onMounted(() => {
  void handleRefresh()
})
</script>

<!--
  Single root required: App.vue switches tabs with v-show + absolute inset-0 on each page component.
  Multiple roots (e.g. main div + Dialog sibling) break directive/fallthrough host binding in Vue 3,
  so the skills grid could stay visible under other tabs.
-->
<template>
  <div class="flex h-full w-full flex-col overflow-hidden bg-background text-foreground">
    <template v-if="!detailOpen">
      <div class="flex shrink-0 flex-col gap-2 border-b border-border px-4 py-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">
              {{ t('settings.openclawSkills.title') }}
            </div>
            <p class="text-xs text-muted-foreground">
              {{ t('settings.openclawSkills.pageDesc') }}
            </p>
          </div>
          <div class="flex items-center gap-1">
            <button
              type="button"
              class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              :title="t('nav.settings')"
              @click="navigateToOpenClawRuntimeSettings"
            >
              <Settings class="size-4" />
            </button>
            <button
              type="button"
              class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              @click="handleRefresh"
            >
              <RefreshCw class="size-4" :class="refreshing && 'animate-spin'" />
            </button>
            <button
              type="button"
              class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              :title="t('settings.openclawSkills.add.title')"
              @click="openAddDialog"
            >
              <Plus class="size-4" />
            </button>
            <button
              type="button"
              class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              :title="t('settings.openclawSkills.openMainWorkspaceSkillsDir')"
              @click="openMainWorkspaceSkillsFolder"
            >
              <FolderOpen class="size-4" />
            </button>
            <button
              type="button"
              class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              :title="t('settings.openclawSkills.openManagedSkillsDir')"
              @click="openManagedSkillsFolder"
            >
              <Folders class="size-4" />
            </button>
          </div>
        </div>
        <p v-if="!gatewayConnected" class="text-xs text-muted-foreground">
          {{ t('settings.openclawSkills.gatewayOfflineHint') }}
        </p>
        <div class="flex flex-wrap items-center gap-2">
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="opt in [
                { v: 'all' as SkillsFilter, key: 'settings.openclawSkills.filterAll' },
                { v: 'builtin' as SkillsFilter, key: 'settings.openclawSkills.filterBuiltin' },
                { v: 'installed' as SkillsFilter, key: 'settings.openclawSkills.filterInstalled' },
              ] as const"
              :key="String(opt.v)"
              type="button"
              class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
              :class="
                filter === opt.v
                  ? 'bg-foreground text-background'
                  : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground'
              "
              @click="filter = opt.v"
            >
              {{ t(opt.key) }}
            </button>
          </div>
          <div class="relative w-80 max-w-full sm:ml-auto">
            <Search
              class="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              v-model="searchQuery"
              class="h-9 pl-8 text-sm"
              :placeholder="t('settings.openclawSkills.searchPlaceholder')"
            />
          </div>
        </div>
      </div>

      <div class="min-h-0 flex-1 overflow-auto p-4">
        <div
          v-if="loading"
          class="flex items-center justify-center gap-2 py-16 text-sm text-muted-foreground"
        >
          <Loader2 class="size-4 animate-spin" />
          {{ t('common.loading') }}
        </div>
        <div
          v-else-if="displayedSkills.length === 0"
          class="flex flex-col items-center justify-center gap-2 py-16 text-center text-muted-foreground"
        >
          <span class="text-sm">{{ t('settings.openclawSkills.noSkills') }}</span>
          <span class="max-w-sm text-xs">{{ t('settings.openclawSkills.noSkillsHint') }}</span>
        </div>
        <div v-else class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
          <div
            v-for="s in displayedSkills"
            :key="s.slug"
            class="group flex cursor-pointer flex-col rounded-2xl border border-border bg-card p-4 text-left shadow-sm transition-colors hover:bg-accent/30 dark:shadow-none dark:ring-1 dark:ring-white/10"
            @click="openDetail(s)"
          >
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="flex items-center gap-2">
                  <span class="truncate text-sm font-medium text-foreground">
                    {{ s.name || s.slug }}
                  </span>
                  <Badge
                    variant="secondary"
                    class="shrink-0 bg-muted px-2 py-0.5 text-[10px] text-muted-foreground"
                  >
                    {{
                      isWorkspaceSkill(s)
                        ? t('settings.openclawSkills.filterInstalled')
                        : t('settings.openclawSkills.filterBuiltin')
                    }}
                  </Badge>
                </div>
                <div class="mt-0.5 truncate font-mono text-[10px] text-muted-foreground/60">
                  {{ s.slug }}
                </div>
              </div>
            </div>
            <p
              v-if="s.description"
              class="mt-2 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground"
            >
              {{ s.description }}
            </p>
            <div v-else class="mt-2 min-h-[2lh]" />
            <div class="mt-auto flex items-center justify-between gap-2 pt-4">
              <span v-if="s.version" class="text-[11px] text-muted-foreground">v{{ s.version }}</span>
              <span v-else class="text-[11px] text-muted-foreground" />
              <div class="flex min-w-0 items-center justify-end gap-1.5">
                <Badge
                  variant="secondary"
                  class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
                >
                  {{ skillSourceTag(s) }}
                </Badge>
                <Badge
                  v-if="listAgentHint(s)"
                  variant="secondary"
                  class="min-w-0 max-w-[140px] truncate bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
                  :title="listAgentHint(s)"
                >
                  {{ listAgentHint(s) }}
                </Badge>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>

    <template v-else-if="activeSkill">
      <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div class="flex shrink-0 items-center border-b border-border px-4 py-2">
          <button
            type="button"
            class="inline-flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            @click="closeDetail"
          >
            <ChevronLeft class="size-4" />
            {{ t('settings.openclawSkills.backToList') }}
          </button>
        </div>
        <div class="flex shrink-0 flex-col gap-2 border-b border-border px-4 py-3">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-base font-semibold">{{ activeSkill.name || activeSkill.slug }}</span>
                <Badge
                  variant="secondary"
                  class="bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
                >
                  {{ locationLabel(activeSkill) }}
                </Badge>
              </div>
              <p v-if="activeSkill.description" class="mt-1 text-xs text-muted-foreground">
                {{ activeSkill.description }}
              </p>
            </div>
            <button
              v-if="activeSkill.skillRoot"
              type="button"
              class="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              @click="openSkillDir(activeSkill)"
            >
              <FolderOpen class="size-3.5" />
              {{ t('settings.skills.openDir') }}
            </button>
          </div>
          <dl class="grid gap-2 rounded-md border border-border bg-muted/30 p-3 text-xs sm:grid-cols-2">
            <div>
              <dt class="font-medium text-foreground/90">
                {{ t('settings.openclawSkills.permissionLabel') }}
              </dt>
              <dd class="mt-0.5 text-muted-foreground">
                {{ activeSkill.permission || t('common.na') }}
              </dd>
            </div>
            <div>
              <dt class="font-medium text-foreground/90">{{ t('settings.openclawSkills.scopeLabel') }}</dt>
              <dd class="mt-0.5 text-muted-foreground">{{ activeSkill.scope || t('common.na') }}</dd>
            </div>
            <div v-if="activeSkill.location === 'workspace'">
              <dt class="font-medium text-foreground/90">{{ t('settings.openclawSkills.agentBinding') }}</dt>
              <dd class="mt-0.5 text-muted-foreground">{{ agentBindingLabel(activeSkill) || t('common.na') }}</dd>
            </div>
            <div v-if="activeSkill.version">
              <dt class="font-medium text-foreground/90">{{ t('settings.skills.version') }}</dt>
              <dd class="mt-0.5 text-muted-foreground">{{ activeSkill.version }}</dd>
            </div>
            <div>
              <dt class="font-medium text-foreground/90">{{ t('settings.openclawSkills.dataSourceLabel') }}</dt>
              <dd class="mt-0.5 text-muted-foreground">{{ dataSourceLabel(activeSkill) }}</dd>
            </div>
            <div v-if="activeSkill.dataSource === 'gateway'">
              <dt class="font-medium text-foreground/90">{{ t('settings.openclawSkills.eligibleLabel') }}</dt>
              <dd class="mt-0.5 text-muted-foreground">{{ eligibleLabelText(activeSkill) }}</dd>
            </div>
            <div v-if="activeSkill.dataSource === 'gateway' && activeSkill.ineligibleReason">
              <dt class="font-medium text-foreground/90">{{ t('settings.openclawSkills.gateHintLabel') }}</dt>
              <dd class="mt-0.5 text-muted-foreground">{{ activeSkill.ineligibleReason }}</dd>
            </div>
          </dl>
          <div
            v-if="activeSkill.installations && activeSkill.installations.length > 0"
            class="mt-3 rounded-md border border-border bg-muted/20 p-3"
          >
            <div class="mb-2 text-[11px] font-medium text-muted-foreground">
              {{ t('settings.openclawSkills.diskLocationsTitle') }}
            </div>
            <ul class="space-y-2">
              <li
                v-for="(ins, idx) in activeSkill.installations"
                :key="idx"
                class="rounded-md border border-border bg-background/60 px-2.5 py-2 text-xs"
              >
                <div class="flex flex-wrap items-start justify-between gap-2">
                  <div class="min-w-0 flex-1 space-y-1">
                    <div class="flex flex-wrap gap-1.5 text-[11px] text-muted-foreground">
                      <span class="font-medium text-foreground/90">{{ installationAgentLine(ins) }}</span>
                      <span class="rounded bg-muted px-1 py-0 font-mono text-[10px]">{{ installationLayerLabel(ins.layer) }}</span>
                      <span class="rounded bg-muted px-1 py-0 text-[10px]">{{ diskLocationBadge(ins.location) }}</span>
                    </div>
                    <div class="break-all font-mono text-[10px] text-muted-foreground">
                      {{ ins.skillRoot }}
                    </div>
                  </div>
                  <button
                    type="button"
                    class="inline-flex shrink-0 items-center gap-1 rounded-md border border-border px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                    @click="openInstallationDir(ins.skillRoot)"
                  >
                    <FolderOpen class="size-3" />
                    {{ t('settings.skills.openDir') }}
                  </button>
                </div>
              </li>
            </ul>
          </div>
        </div>
        <div v-if="activeSkill.skillRoot" class="flex min-h-0 flex-1 overflow-hidden">
          <aside class="flex w-52 shrink-0 flex-col overflow-hidden border-r border-border bg-muted/20">
            <div class="border-b border-border px-2 py-1.5 text-[11px] font-medium text-muted-foreground">
              {{ t('settings.skills.selectFile') }}
            </div>
            <div class="min-h-0 flex-1 overflow-auto p-1">
              <button
                v-for="f in detailFiles"
                :key="f.path"
                type="button"
                class="flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-xs transition-colors"
                :class="
                  selectedFilePath === f.path
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
                "
                @click="selectFile(f.path)"
              >
                <FileText v-if="isMarkdownFile(f.path)" class="size-3.5 shrink-0 opacity-70" />
                <FileCode v-else class="size-3.5 shrink-0 opacity-70" />
                <span class="truncate">{{ f.path }}</span>
              </button>
            </div>
          </aside>
          <div class="min-h-0 flex-1 overflow-auto p-4">
            <div v-if="fileLoading" class="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 class="size-4 animate-spin" />
              {{ t('common.loading') }}
            </div>
            <template v-else-if="selectedFilePath">
              <p v-if="isSelectedBinary" class="text-sm text-muted-foreground">
                {{ t('settings.skills.binaryFile') }}
              </p>
              <MarkdownRenderer
                v-else-if="isSelectedMarkdown"
                :content="renderedFileContent"
                class="prose prose-sm dark:prose-invert max-w-none"
              />
              <MarkdownRenderer
                v-else
                :content="fileContentAsMarkdown"
                class="prose prose-sm dark:prose-invert max-w-none"
              />
            </template>
            <p v-else class="text-sm text-muted-foreground">
              {{ t('settings.skills.selectFile') }}
            </p>
          </div>
        </div>
        <div
          v-else
          class="flex min-h-0 flex-1 items-center justify-center p-8 text-center text-sm text-muted-foreground"
        >
          {{ t('settings.openclawSkills.previewNoLocalPath') }}
        </div>
      </div>
    </template>

    <Dialog :open="addDialogOpen" @update:open="(v) => (addDialogOpen = v)">
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{{ t('settings.openclawSkills.add.title') }}</DialogTitle>
        </DialogHeader>

        <div class="grid gap-3 py-2">
          <button
            type="button"
            class="flex w-full items-start gap-3 rounded-xl border border-border bg-background p-4 text-left transition-colors hover:bg-accent/30"
            @click="handleCreateViaChat"
          >
            <div class="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <Plus class="size-4" />
            </div>
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ t('settings.openclawSkills.add.createViaChatTitle') }}
              </div>
              <div class="mt-1 text-xs leading-relaxed text-muted-foreground">
                {{ t('settings.openclawSkills.add.createViaChatDesc') }}
              </div>
            </div>
          </button>

          <button
            type="button"
            class="flex w-full items-start gap-3 rounded-xl border border-border bg-background p-4 text-left transition-colors hover:bg-accent/30"
            @click="handleChooseSkillPackage"
          >
            <div class="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <FolderOpen class="size-4" />
            </div>
            <div class="min-w-0">
              <div class="text-sm font-medium text-foreground">
                {{ t('settings.openclawSkills.add.choosePackageTitle') }}
              </div>
              <div class="mt-1 text-xs leading-relaxed text-muted-foreground">
                {{ t('settings.openclawSkills.add.choosePackageDesc') }}
              </div>
            </div>
          </button>
        </div>
      </DialogContent>
    </Dialog>
  </div>
</template>
