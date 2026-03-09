<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  RefreshCw,
  Download,
  Star,
  Search,
  Loader2,
  Package,
  Trash2,
  Settings,
  FolderOpen,
  ChevronLeft,
  User,
  X,
  FileText,
  FileCode,
  Plus,
} from 'lucide-vue-next'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import MarkdownRenderer from '@/components/MarkdownRenderer.vue'
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

import { SkillsService } from '@bindings/chatclaw/internal/services/skills'
import type {
  InstalledSkill,
  RemoteSkill,
  SkillDetail,
  SkillFileInfo,
} from '@bindings/chatclaw/internal/services/skills'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { useNavigationStore, useSettingsStore } from '@/stores'

defineProps<{
  tabId: string
}>()

const { t } = useI18n()
const navigationStore = useNavigationStore()
const settingsStore = useSettingsStore()

type MainTab = 'installed' | 'market'

const activeTab = ref<MainTab>('installed')
const skillsDir = ref('')

// --- Installed tab ---
const installedSkills = ref<InstalledSkill[]>([])
const installedFilter = ref<'all' | 'builtin' | 'market' | 'local'>('all')
const installedLoading = ref(false)
const installedSearchQuery = ref('')
const installedPage = ref(1)
const INSTALLED_PAGE_SIZE = 20

const filteredSkills = computed(() => {
  let list = installedSkills.value
  if (installedSearchQuery.value.trim()) {
    const q = installedSearchQuery.value.trim().toLowerCase()
    list = list.filter(
      (s) =>
        s.slug.toLowerCase().includes(q) ||
        (s.name && s.name.toLowerCase().includes(q)) ||
        (s.description && s.description.toLowerCase().includes(q)),
    )
  }
  return list
})

const paginatedSkills = computed(() => {
  const end = installedPage.value * INSTALLED_PAGE_SIZE
  return filteredSkills.value.slice(0, end)
})

const installedHasMore = computed(
  () => paginatedSkills.value.length < filteredSkills.value.length,
)

const isLocalFilter = computed(() => installedFilter.value === 'local')
const hasLocalSkills = computed(() => isLocalFilter.value && filteredSkills.value.length > 0)

// --- Market tab ---
const marketSkills = ref<RemoteSkill[]>([])
const marketLoading = ref(false)
const marketSort = ref('trending')
const marketCursor = ref('')
const marketHasMore = ref(false)
const searchQuery = ref('')
const searchResults = ref<RemoteSkill[]>([])
const isSearching = ref(false)
const searchMode = ref(false)

const displayedMarketSkills = computed(() =>
  searchMode.value ? searchResults.value : marketSkills.value,
)

// --- Installing / deleting state ---
const installingSet = ref(new Set<string>())
const deleteTarget = ref<InstalledSkill | null>(null)

// --- Detail view ---
type DetailView = 'none' | 'installed' | 'market'
const detailView = ref<DetailView>('none')
const detailSkill = ref<InstalledSkill | null>(null)
const detailRemoteSkill = ref<RemoteSkill | null>(null)
const detailLoading = ref(false)
const detailMeta = ref<SkillDetail | null>(null)

// File browser
const detailFiles = ref<SkillFileInfo[]>([])
const selectedFilePath = ref('')
const fileContent = ref('')
const fileLoading = ref(false)

// Guard against stale async results when navigating quickly
let detailLoadVersion = 0
let fileLoadVersion = 0

const sortOptions = [
  { value: 'trending', key: 'settings.skills.sortTrending' },
  { value: 'downloads', key: 'settings.skills.sortDownloads' },
  { value: 'newest', key: 'settings.skills.sortNewest' },
  { value: 'installsAllTime', key: 'settings.skills.sortInstalls' },
]

const filterOptions = [
  { value: 'all' as const, key: 'settings.skills.filterAll' },
  { value: 'builtin' as const, key: 'settings.skills.filterBuiltin' },
  { value: 'market' as const, key: 'settings.skills.filterMarket' },
  { value: 'local' as const, key: 'settings.skills.filterLocal' },
]

const BINARY_EXTENSIONS = new Set([
  'png', 'jpg', 'jpeg', 'gif', 'webp', 'ico', 'bmp', 'svg',
  'woff', 'woff2', 'ttf', 'otf', 'eot',
  'zip', 'tar', 'gz', 'bz2', 'xz', '7z',
  'exe', 'dll', 'so', 'dylib',
  'pdf', 'doc', 'docx', 'xls', 'xlsx',
  'mp3', 'mp4', 'wav', 'avi', 'mov',
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
    js: 'javascript', mjs: 'javascript', cjs: 'javascript',
    ts: 'typescript', mts: 'typescript', cts: 'typescript',
    py: 'python', rb: 'ruby', go: 'go', rs: 'rust',
    java: 'java', kt: 'kotlin', swift: 'swift',
    sh: 'bash', bash: 'bash', zsh: 'bash',
    json: 'json', yaml: 'yaml', yml: 'yaml', toml: 'toml',
    xml: 'xml', html: 'html', css: 'css', scss: 'scss',
    sql: 'sql', dockerfile: 'dockerfile',
    md: 'markdown',
  }
  return langMap[ext] || ''
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`
}

async function loadSkillsDir() {
  try {
    skillsDir.value = await SkillsService.GetSkillsDir()
  } catch (error) {
    console.error('Failed to get skills directory:', error)
  }
}

async function handleOpenSkillsDir() {
  if (!skillsDir.value) return
  try {
    await BrowserService.OpenDirectory(skillsDir.value)
  } catch (error) {
    console.error('Failed to open skills directory:', error)
  }
}

// --- Installed skills ---
async function loadInstalledSkills() {
  installedLoading.value = true
  try {
    installedSkills.value = await SkillsService.ListInstalledSkills(installedFilter.value)
    installedPage.value = 1
  } catch (error) {
    console.error('Failed to load installed skills:', error)
  } finally {
    installedLoading.value = false
  }
}

const refreshing = ref(false)

async function handleRefresh() {
  refreshing.value = true
  const minDelay = new Promise((r) => setTimeout(r, 500))
  try {
    if (activeTab.value === 'market') {
      marketCursor.value = ''
      searchMode.value = false
      searchQuery.value = ''
      await Promise.all([loadMarketSkills(), minDelay])
    } else {
      await Promise.all([loadInstalledSkills(), minDelay])
    }
  } finally {
    refreshing.value = false
  }
}

async function handleToggleSkill(skill: InstalledSkill) {
  const newEnabled = !skill.enabled
  try {
    if (newEnabled) {
      await SkillsService.EnableSkill(skill.slug)
    } else {
      await SkillsService.DisableSkill(skill.slug)
    }
    skill.enabled = newEnabled
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

let pendingDeleteSkill: InstalledSkill | null = null

function confirmDelete(skill: InstalledSkill) {
  pendingDeleteSkill = skill
  deleteTarget.value = skill
}

async function handleDelete() {
  const skill = pendingDeleteSkill
  pendingDeleteSkill = null
  deleteTarget.value = null
  if (!skill) return
  try {
    await SkillsService.UninstallSkill(skill.slug)
    installedSkills.value = installedSkills.value.filter((s) => s.slug !== skill.slug)
    toast.success(t('settings.skills.deleteSuccess'))
    if (detailView.value === 'installed' && detailSkill.value?.slug === skill.slug) {
      detailView.value = 'none'
    }
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.skills.deleteFailed'))
  }
}

// --- Market skills ---
async function loadMarketSkills(append = false) {
  if (marketLoading.value) return
  marketLoading.value = true
  if (!append) {
    marketSkills.value = []
  }
  try {
    const result = await SkillsService.ExploreSkills(
      25,
      marketSort.value,
      append ? marketCursor.value : '',
    )
    if (result) {
      if (append) {
        marketSkills.value = [...marketSkills.value, ...result.items]
      } else {
        marketSkills.value = result.items
      }
      marketCursor.value = result.nextCursor
      marketHasMore.value = !!result.nextCursor
    }
  } catch (error) {
    const msg = getErrorMessage(error)
    if (msg && (msg.includes('429') || msg.toLowerCase().includes('rate'))) {
      toast.error(t('settings.skills.rateLimited'))
    } else {
      toast.error(t('settings.skills.loadFailed'))
    }
  } finally {
    marketLoading.value = false
  }
}

async function handleSearch() {
  const query = searchQuery.value.trim()
  if (!query) {
    searchMode.value = false
    return
  }
  searchMode.value = true
  isSearching.value = true
  try {
    searchResults.value = await SkillsService.SearchSkills(query, 30)
  } catch (error) {
    const msg = getErrorMessage(error)
    if (msg && (msg.includes('429') || msg.toLowerCase().includes('rate'))) {
      toast.error(t('settings.skills.rateLimited'))
    } else {
      toast.error(t('settings.skills.loadFailed'))
    }
  } finally {
    isSearching.value = false
  }
}

async function handleInstall(skill: RemoteSkill) {
  if (installingSet.value.has(skill.slug)) return
  installingSet.value.add(skill.slug)
  try {
    await SkillsService.InstallSkill(skill.slug, skill.version || 'latest')
    skill.installed = true
    toast.success(t('settings.skills.installSuccess'))
    void loadInstalledSkills()
    // Update detail view if we're looking at this skill
    if (detailView.value === 'market' && detailMeta.value) {
      detailMeta.value = { ...detailMeta.value, installed: true }
    }
  } catch (error) {
    toast.error(getErrorMessage(error) || t('settings.skills.installFailed'))
  } finally {
    installingSet.value.delete(skill.slug)
  }
}

// --- Detail: file selection ---
async function selectFile(path: string) {
  if (path === selectedFilePath.value) return
  selectedFilePath.value = path

  if (isBinaryFile(path)) {
    fileContent.value = ''
    return
  }

  const version = ++fileLoadVersion
  fileLoading.value = true
  try {
    let content = ''
    if (detailView.value === 'installed' && detailSkill.value) {
      content = await SkillsService.ReadSkillFile(detailSkill.value.slug, path)
    } else if (detailView.value === 'market' && detailRemoteSkill.value) {
      content = await SkillsService.GetRemoteSkillFile(
        detailRemoteSkill.value.slug,
        detailRemoteSkill.value.version || 'latest',
        path,
      )
    }
    if (version !== fileLoadVersion) return
    fileContent.value = content
  } catch {
    if (version !== fileLoadVersion) return
    fileContent.value = ''
  } finally {
    if (version === fileLoadVersion) {
      fileLoading.value = false
    }
  }
}

// --- Detail: open ---
async function showInstalledDetail(skill: InstalledSkill) {
  const version = ++detailLoadVersion
  detailView.value = 'installed'
  detailSkill.value = skill
  detailRemoteSkill.value = null
  detailMeta.value = null
  detailFiles.value = []
  selectedFilePath.value = ''
  fileContent.value = ''
  detailLoading.value = true
  try {
    const files = await SkillsService.ListSkillFiles(skill.slug)
    if (version !== detailLoadVersion) return
    detailFiles.value = files
    if (files.length > 0) {
      const skillMd = files.find((f) => f.path === 'SKILL.md')
      await selectFile(skillMd ? skillMd.path : files[0].path)
    }
  } catch {
    if (version !== detailLoadVersion) return
    detailFiles.value = []
  } finally {
    if (version === detailLoadVersion) {
      detailLoading.value = false
    }
  }
}

async function showMarketDetail(skill: RemoteSkill) {
  const version = ++detailLoadVersion
  detailView.value = 'market'
  detailRemoteSkill.value = skill
  detailSkill.value = null
  detailMeta.value = null
  detailFiles.value = []
  selectedFilePath.value = ''
  fileContent.value = ''
  detailLoading.value = true
  try {
    try {
      detailMeta.value = await SkillsService.GetSkillDetail(skill.slug)
    } catch (err) {
      const msg = String(err ?? '')
      if (msg.includes('429') || msg.toLowerCase().includes('rate')) {
        toast.error(t('settings.skills.rateLimited'))
      }
    }
    if (version !== detailLoadVersion) return

    try {
      const files = await SkillsService.GetRemoteSkillFiles(skill.slug, skill.version || 'latest')
      if (version !== detailLoadVersion) return
      detailFiles.value = files
      if (files.length > 0) {
        const skillMd = files.find((f) => f.path === 'SKILL.md')
        await selectFile(skillMd ? skillMd.path : files[0].path)
      }
    } catch (err) {
      const msg = String(err ?? '')
      if (msg.includes('429') || msg.toLowerCase().includes('rate')) {
        toast.error(t('settings.skills.rateLimited'))
      }
    }
  } catch {
    // ignore
  } finally {
    if (version === detailLoadVersion) {
      detailLoading.value = false
    }
  }
}

async function handleOpenSkillDir(slug: string) {
  try {
    const path = await SkillsService.GetSkillPath(slug)
    await BrowserService.OpenDirectory(path)
  } catch (error) {
    console.error('Failed to open skill directory:', error)
  }
}

function goBackFromDetail() {
  ++detailLoadVersion
  ++fileLoadVersion
  detailView.value = 'none'
  detailSkill.value = null
  detailRemoteSkill.value = null
  detailMeta.value = null
  detailFiles.value = []
  selectedFilePath.value = ''
  fileContent.value = ''
}

function navigateToSettings() {
  settingsStore.setActiveMenu('skills')
  navigationStore.navigateToModule('settings')
}

function sourceLabel(source: string): string {
  switch (source) {
    case 'builtin':
      return t('settings.skills.sourceBuiltin')
    case 'market':
      return t('settings.skills.sourceMarket')
    case 'local':
      return t('settings.skills.sourceLocal')
    default:
      return source
  }
}

function formatNumber(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}

// Detail: computed display name and description
const detailName = computed(() => {
  if (detailView.value === 'installed' && detailSkill.value) {
    return detailSkill.value.name || detailSkill.value.slug
  }
  if (detailView.value === 'market') {
    return detailMeta.value?.displayName || detailRemoteSkill.value?.displayName || detailRemoteSkill.value?.slug || ''
  }
  return ''
})

const detailDescription = computed(() => {
  if (detailView.value === 'installed' && detailSkill.value) {
    return detailSkill.value.description || ''
  }
  if (detailView.value === 'market') {
    return detailMeta.value?.summary || detailRemoteSkill.value?.summary || ''
  }
  return ''
})

const detailVersion = computed(() => {
  if (detailView.value === 'installed' && detailSkill.value) {
    return detailSkill.value.version
  }
  if (detailView.value === 'market') {
    return detailMeta.value?.version || detailRemoteSkill.value?.version || ''
  }
  return ''
})

const detailIsInstalled = computed(() => {
  if (detailView.value === 'installed') return true
  return detailMeta.value?.installed || detailRemoteSkill.value?.installed || false
})

const detailSlug = computed(() => {
  if (detailView.value === 'installed') return detailSkill.value?.slug || ''
  return detailRemoteSkill.value?.slug || ''
})

// Strip YAML frontmatter (--- ... ---) from markdown content
function stripFrontmatter(content: string): string {
  const trimmed = content.trimStart()
  if (!trimmed.startsWith('---')) return content
  const rest = trimmed.slice(3)
  const endIdx = rest.indexOf('\n---')
  if (endIdx === -1) return content
  return rest.slice(endIdx + 4).trimStart()
}

// Rendered file content
const renderedFileContent = computed(() => {
  if (!selectedFilePath.value || isBinaryFile(selectedFilePath.value)) return null
  let content = fileContent.value
  if (isMarkdownFile(selectedFilePath.value)) {
    content = stripFrontmatter(content)
  }
  return content
})

const isSelectedFileMarkdown = computed(() => isMarkdownFile(selectedFilePath.value))

const codeLanguageForFile = computed(() => getCodeLanguage(selectedFilePath.value))

// Wrap non-markdown content in a code block for MarkdownRenderer
const fileContentAsMarkdown = computed(() => {
  if (!renderedFileContent.value) return ''
  const lang = codeLanguageForFile.value || 'text'
  return '```' + lang + '\n' + renderedFileContent.value + '\n```'
})

watch(installedFilter, () => {
  void loadInstalledSkills()
})

watch(marketSort, () => {
  marketCursor.value = ''
  void loadMarketSkills()
})

watch(installedSearchQuery, () => {
  installedPage.value = 1
})

onMounted(() => {
  void loadSkillsDir()
  void loadInstalledSkills()
})

watch(activeTab, (tab) => {
  if (tab === 'market' && marketSkills.value.length === 0) {
    void loadMarketSkills()
  }
  detailView.value = 'none'
})
</script>

<template>
  <div class="flex h-full w-full flex-col overflow-hidden bg-background text-foreground">
    <!-- Header bar (hidden in detail view) -->
    <div v-if="detailView === 'none'" class="flex shrink-0 items-center justify-between border-b border-border px-4 py-2">
      <div class="flex items-center gap-2">
        <span class="text-sm font-medium text-foreground">{{ t('settings.skills.title') }}</span>
        <span class="text-xs text-muted-foreground">{{ t('settings.skills.pageDesc') }}</span>
      </div>
      <div class="flex items-center gap-1">
        <button
          class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          :title="t('nav.settings')"
          @click="navigateToSettings"
        >
          <Settings class="size-4" />
        </button>
        <button
          class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          @click="handleRefresh"
        >
          <RefreshCw class="size-4" :class="refreshing && 'animate-spin'" />
        </button>
        <button
          class="inline-flex cursor-pointer items-center justify-center rounded-md p-2 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          :title="t('settings.skills.openDir')"
          @click="handleOpenSkillsDir"
        >
          <FolderOpen class="size-4" />
        </button>
      </div>
    </div>

    <!-- ==================== DETAIL VIEW ==================== -->
    <template v-if="detailView !== 'none'">
      <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <!-- Row 1: back button -->
        <div class="flex shrink-0 items-center border-b border-border px-4 py-2">
          <button
            class="inline-flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            @click="goBackFromDetail"
          >
            <ChevronLeft class="size-4" />
            {{ activeTab === 'installed' ? t('settings.skills.tabInstalled') : t('settings.skills.tabMarket') }}
          </button>
        </div>

        <!-- Row 2: skill info + actions -->
        <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border px-4 py-3">
          <!-- Left: name, description, stats -->
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="text-base font-semibold text-foreground">{{ detailName }}</span>
              <Badge
                v-if="detailView === 'installed' && detailSkill"
                variant="secondary"
                class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground"
              >
                {{ sourceLabel(detailSkill.source) }}
              </Badge>
            </div>
            <p v-if="detailDescription" class="mt-1 text-xs leading-relaxed text-muted-foreground">
              {{ detailDescription }}
            </p>
            <!-- Market stats -->
            <div v-if="detailView === 'market' && detailMeta" class="mt-2 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
              <span v-if="detailMeta.ownerName" class="flex items-center gap-1">
                <img v-if="detailMeta.ownerImage" :src="detailMeta.ownerImage" class="size-4 rounded-full" alt="" />
                <User v-else class="size-3.5" />
                {{ detailMeta.ownerName }}
              </span>
              <span v-if="detailMeta.downloads" class="flex items-center gap-1">
                <Download class="size-3" />
                {{ formatNumber(detailMeta.downloads) }}
              </span>
              <span v-if="detailMeta.stars" class="flex items-center gap-1">
                <Star class="size-3" />
                {{ formatNumber(detailMeta.stars) }}
              </span>
            </div>
          </div>

          <!-- Right: version + action buttons -->
          <div class="flex shrink-0 flex-col items-end gap-2">
            <span v-if="detailVersion" class="text-xs text-muted-foreground">v{{ detailVersion }}</span>

            <div class="flex items-center gap-2">
              <!-- Installed: open dir + delete -->
              <template v-if="detailView === 'installed' && detailSkill">
                <button
                  class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  @click="handleOpenSkillDir(detailSkill.slug)"
                >
                  <FolderOpen class="size-3.5" />
                  {{ t('settings.skills.openDir') }}
                </button>
                <button
                  v-if="detailSkill.source === 'market'"
                  class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                  @click="confirmDelete(detailSkill)"
                >
                  <Trash2 class="size-3.5" />
                  {{ t('settings.skills.delete') }}
                </button>
              </template>

              <!-- Market: install / installed / open dir -->
              <template v-if="detailView === 'market'">
                <template v-if="detailIsInstalled">
                  <button
                    class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                    @click="handleOpenSkillDir(detailSlug)"
                  >
                    <FolderOpen class="size-3.5" />
                    {{ t('settings.skills.openDir') }}
                  </button>
                  <span class="rounded-md bg-muted px-3 py-1 text-xs font-medium text-muted-foreground">
                    {{ t('settings.skills.installed') }}
                  </span>
                </template>
                <template v-else-if="detailRemoteSkill">
                  <button
                    v-if="installingSet.has(detailRemoteSkill.slug)"
                    class="flex items-center gap-1.5 rounded-md bg-muted px-3 py-1 text-xs font-medium text-muted-foreground"
                    disabled
                  >
                    <Loader2 class="size-3 animate-spin" />
                    {{ t('settings.skills.installing') }}
                  </button>
                  <button
                    v-else
                    class="cursor-pointer rounded-md bg-foreground px-3 py-1 text-xs font-medium text-background transition-opacity hover:opacity-80"
                    @click="handleInstall(detailRemoteSkill)"
                  >
                    {{ t('settings.skills.install') }}
                  </button>
                </template>
              </template>
            </div>
          </div>
        </div>

        <!-- Detail body: file list + content -->
        <div v-if="detailLoading" class="flex flex-1 items-center justify-center">
          <Loader2 class="size-5 animate-spin text-muted-foreground" />
        </div>
        <div v-else class="flex min-h-0 flex-1">
          <!-- Left: file list -->
          <aside class="flex w-56 shrink-0 flex-col overflow-auto border-r border-border bg-muted/30">
            <div
              v-for="file in detailFiles"
              :key="file.path"
              :title="file.path + '  ' + formatFileSize(file.size)"
              :class="
                cn(
                  'flex cursor-pointer items-center gap-2 px-3 py-1.5 text-xs transition-colors hover:bg-accent/50',
                  selectedFilePath === file.path && 'bg-accent text-foreground',
                  selectedFilePath !== file.path && 'text-muted-foreground',
                )
              "
              @click="selectFile(file.path)"
            >
              <FileText v-if="isMarkdownFile(file.path)" class="size-3.5 shrink-0" />
              <FileCode v-else class="size-3.5 shrink-0" />
              <span class="min-w-0 flex-1 truncate">{{ file.path }}</span>
              <span class="shrink-0 text-[10px] opacity-60">{{ formatFileSize(file.size) }}</span>
            </div>
            <div v-if="detailFiles.length === 0" class="px-3 py-4 text-xs text-muted-foreground">
              {{ t('settings.skills.noDetailContent') }}
            </div>
          </aside>

          <!-- Right: file content -->
          <main class="skill-file-viewer flex min-w-0 flex-1 flex-col overflow-auto">
            <div v-if="fileLoading" class="flex flex-1 items-center justify-center">
              <Loader2 class="size-5 animate-spin text-muted-foreground" />
            </div>
            <div v-else-if="!selectedFilePath" class="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              {{ t('settings.skills.selectFile') }}
            </div>
            <div v-else-if="isBinaryFile(selectedFilePath)" class="flex flex-1 items-center justify-center text-sm text-muted-foreground">
              {{ t('settings.skills.binaryFile') }}
            </div>
            <div v-else-if="isSelectedFileMarkdown" class="p-4">
              <MarkdownRenderer :content="renderedFileContent || ''" />
            </div>
            <div v-else>
              <MarkdownRenderer :content="fileContentAsMarkdown" />
            </div>
          </main>
        </div>
      </div>
    </template>

    <!-- ==================== MAIN TAB CONTENT ==================== -->
    <template v-else>
      <!-- Tab bar (memory-page style) -->
      <div class="flex items-center border-b border-border px-4 py-2">
        <div class="inline-flex rounded-lg bg-muted p-1">
          <button
            v-for="tab in ([
              { key: 'installed' as MainTab, label: t('settings.skills.tabInstalled') },
              { key: 'market' as MainTab, label: t('settings.skills.tabMarket') },
            ])"
            :key="tab.key"
            type="button"
            :class="
              cn(
                'rounded-md px-4 py-1.5 text-sm font-medium transition-colors',
                activeTab === tab.key
                  ? 'bg-background text-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground',
              )
            "
            @click="activeTab = tab.key"
          >
            {{ tab.label }}
          </button>
        </div>
      </div>

      <!-- Installed tab -->
      <div v-if="activeTab === 'installed'" class="flex min-h-0 flex-1 flex-col">
        <!-- Filters + search -->
        <div class="flex shrink-0 items-center gap-3 px-4 py-2">
          <div class="flex gap-1">
            <button
              v-for="opt in filterOptions"
              :key="opt.value"
              class="rounded-md px-2.5 py-1 text-xs font-medium transition-colors"
              :class="
                installedFilter === opt.value
                  ? 'bg-foreground text-background'
                  : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground'
              "
              @click="installedFilter = opt.value"
            >
              {{ t(opt.key) }}
            </button>
          </div>
          <div class="ml-auto flex items-center gap-2">
            <TooltipProvider v-if="hasLocalSkills">
              <Tooltip>
                <TooltipTrigger as-child>
                  <button
                    class="inline-flex cursor-pointer items-center gap-1 rounded-md border border-border px-2 py-1 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                    @click="handleOpenSkillsDir"
                  >
                    <Plus class="size-3.5" />
                    {{ t('settings.skills.addLocalSkill') }}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top" class="max-w-xs">
                  {{ t('settings.skills.addLocalSkillHint') }}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <div class="relative w-52">
              <Search class="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                v-model="installedSearchQuery"
                :placeholder="t('settings.skills.searchPlaceholder')"
                class="h-7 pl-8 text-xs"
              />
            </div>
          </div>
        </div>

        <!-- Skill list -->
        <div class="flex-1 overflow-auto px-4 pb-4">
          <div
            v-if="installedLoading && filteredSkills.length === 0"
            class="flex items-center justify-center py-12"
          >
            <Loader2 class="size-5 animate-spin text-muted-foreground" />
          </div>
          <div
            v-else-if="filteredSkills.length === 0 && isLocalFilter"
            class="flex flex-col items-center justify-center gap-3 py-12 text-muted-foreground"
          >
            <Package class="size-8 opacity-40" />
            <span class="text-sm font-medium">{{ t('settings.skills.noLocalSkills') }}</span>
            <span class="text-center text-xs leading-relaxed">{{ t('settings.skills.noLocalSkillsHint') }}</span>
            <button
              class="mt-1 inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              @click="handleOpenSkillsDir"
            >
              <FolderOpen class="size-3.5" />
              {{ t('settings.skills.openDir') }}
            </button>
          </div>
          <div
            v-else-if="filteredSkills.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
          >
            <Package class="size-8 opacity-40" />
            <span class="text-sm">{{ t('settings.skills.noSkills') }}</span>
            <span class="text-xs">{{ t('settings.skills.noSkillsHint') }}</span>
          </div>
          <div v-else>
            <div class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
              <div
                v-for="skill in paginatedSkills"
                :key="skill.slug"
                class="group flex cursor-pointer flex-col rounded-lg border border-border p-3.5 transition-colors hover:bg-accent/30 dark:border-white/10"
                @click="showInstalledDetail(skill)"
              >
                <div class="flex items-center gap-2">
                  <span class="truncate text-sm font-medium text-foreground">{{ skill.name || skill.slug }}</span>
                  <Badge variant="secondary" class="shrink-0 bg-muted px-1.5 py-0 text-[10px] text-muted-foreground">
                    {{ sourceLabel(skill.source) }}
                  </Badge>
                </div>
                <p v-if="skill.description" class="mt-1.5 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground">
                  {{ skill.description }}
                </p>
                <div v-else class="mt-1.5 min-h-[2lh]" />
                <div class="mt-auto flex items-center justify-between gap-2 pt-3">
                  <span v-if="skill.version" class="text-[10px] text-muted-foreground">v{{ skill.version }}</span>
                  <span v-else />
                  <div class="flex items-center gap-2" @click.stop>
                    <Switch
                      :model-value="skill.enabled"
                      class="scale-90"
                      @update:model-value="() => handleToggleSkill(skill)"
                    />
                    <button
                      v-if="skill.source === 'market' || skill.source === 'local'"
                      class="inline-flex cursor-pointer items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                      @click.stop="confirmDelete(skill)"
                    >
                      <Trash2 class="size-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- Load more -->
            <div v-if="installedHasMore" class="flex justify-center py-4">
              <button
                class="cursor-pointer rounded-md bg-muted px-4 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                @click="installedPage++"
              >
                {{ t('settings.skills.loadMore') }}
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Market tab -->
      <div v-if="activeTab === 'market'" class="flex min-h-0 flex-1 flex-col">
        <!-- Search + Sort -->
        <div class="flex shrink-0 items-center gap-3 px-4 py-2">
          <div class="relative flex-1">
            <Loader2 v-if="isSearching" class="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 animate-spin text-muted-foreground" />
            <Search v-else class="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              v-model="searchQuery"
              :placeholder="t('settings.skills.searchPlaceholder')"
              class="h-8 pl-8 text-sm"
              :disabled="isSearching"
              @keyup.enter="handleSearch"
            />
          </div>
          <div v-if="!searchMode" class="flex gap-1">
            <button
              v-for="opt in sortOptions"
              :key="opt.value"
              class="rounded-md px-2 py-1 text-xs font-medium transition-colors"
              :class="
                marketSort === opt.value
                  ? 'bg-foreground text-background'
                  : 'bg-muted text-muted-foreground hover:bg-accent hover:text-foreground'
              "
              @click="marketSort = opt.value"
            >
              {{ t(opt.key) }}
            </button>
          </div>
          <button
            v-if="searchMode"
            class="inline-flex cursor-pointer items-center justify-center rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            @click="searchMode = false; searchQuery = ''"
          >
            <X class="size-4" />
          </button>
        </div>

        <!-- Market list -->
        <div class="flex-1 overflow-auto px-4 pb-4">
          <div
            v-if="(marketLoading || isSearching) && displayedMarketSkills.length === 0"
            class="flex items-center justify-center py-12"
          >
            <Loader2 class="size-5 animate-spin text-muted-foreground" />
          </div>
          <div
            v-else-if="displayedMarketSkills.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-12 text-muted-foreground"
          >
            <Package class="size-8 opacity-40" />
            <span class="text-sm">{{ t('settings.skills.noResults') }}</span>
          </div>
          <div v-else>
            <div class="grid grid-cols-[repeat(auto-fill,minmax(260px,1fr))] gap-3">
              <div
                v-for="skill in displayedMarketSkills"
                :key="skill.slug"
                class="flex cursor-pointer flex-col rounded-lg border border-border p-3.5 transition-colors hover:bg-accent/30 dark:border-white/10"
                @click="showMarketDetail(skill)"
              >
                <div class="flex items-center gap-2">
                  <span class="truncate text-sm font-medium text-foreground">{{ skill.displayName || skill.slug }}</span>
                  <span v-if="skill.version" class="shrink-0 text-[10px] text-muted-foreground">v{{ skill.version }}</span>
                </div>
                <span v-if="skill.displayName && skill.displayName !== skill.slug" class="mt-0.5 truncate text-[10px] text-muted-foreground/60">{{ skill.slug }}</span>
                <p v-if="skill.summary" class="mt-1.5 line-clamp-2 min-h-[2lh] text-xs leading-relaxed text-muted-foreground">
                  {{ skill.summary }}
                </p>
                <div v-else class="mt-1.5 min-h-[2lh]" />
                <div class="mt-auto flex items-center justify-between gap-2 pt-3">
                  <div class="flex items-center gap-3 text-[10px] text-muted-foreground">
                    <span v-if="skill.downloads" class="flex items-center gap-1">
                      <Download class="size-3" />
                      {{ formatNumber(skill.downloads) }}
                    </span>
                    <span v-if="skill.stars" class="flex items-center gap-1">
                      <Star class="size-3" />
                      {{ formatNumber(skill.stars) }}
                    </span>
                  </div>
                  <div class="shrink-0" @click.stop>
                    <button
                      v-if="skill.installed"
                      class="rounded-md bg-muted px-2.5 py-1 text-[11px] font-medium text-muted-foreground"
                      disabled
                    >
                      {{ t('settings.skills.installed') }}
                    </button>
                    <button
                      v-else-if="installingSet.has(skill.slug)"
                      class="flex items-center gap-1.5 rounded-md bg-muted px-2.5 py-1 text-[11px] font-medium text-muted-foreground"
                      disabled
                    >
                      <Loader2 class="size-3 animate-spin" />
                      {{ t('settings.skills.installing') }}
                    </button>
                    <button
                      v-else
                      class="cursor-pointer rounded-md bg-foreground px-2.5 py-1 text-[11px] font-medium text-background transition-opacity hover:opacity-80"
                      @click="handleInstall(skill)"
                    >
                      {{ t('settings.skills.install') }}
                    </button>
                  </div>
                </div>
              </div>
            </div>

            <!-- Load more -->
            <div v-if="!searchMode && marketHasMore" class="flex justify-center py-4">
              <button
                class="cursor-pointer rounded-md bg-muted px-4 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                :disabled="marketLoading"
                @click="loadMarketSkills(true)"
              >
                <Loader2 v-if="marketLoading" class="mr-1.5 inline size-3 animate-spin" />
                {{ t('settings.skills.loadMore') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- Delete confirm dialog -->
    <AlertDialog :open="!!deleteTarget" @update:open="(v) => !v && (deleteTarget = null)">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('settings.skills.delete') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.skills.deleteConfirm') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="handleDelete">{{ t('settings.skills.delete') }}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>

<style>
.skill-file-viewer .markdown-content > :first-child {
  margin-top: 0 !important;
}

.skill-file-viewer .code-block-wrapper {
  background: transparent !important;
  border: none !important;
  border-radius: 0 !important;
  margin: 0 !important;
}

.skill-file-viewer .code-block-wrapper > div:first-child {
  display: none !important;
}

.skill-file-viewer .code-block-wrapper pre {
  border-radius: 0 !important;
  background: transparent !important;
}

.skill-file-viewer .code-block-wrapper pre code.hljs {
  background: transparent !important;
}
</style>
