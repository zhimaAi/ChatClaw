<script setup lang="ts">
import { computed, ref, reactive, onMounted, onUnmounted, nextTick } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import type { AcceptableValue } from 'reka-ui'
import { Events } from '@wailsio/runtime'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import {
  useAppStore,
  useOpenClawGatewayStore,
  isOpenClawRuntimeMutatingPhase,
  type Theme,
} from '@/stores'
import { useLocale, SUPPORTED_LOCALES, type Locale } from '@/composables/useLocale'
import * as ToolchainService from '@bindings/chatclaw/internal/services/toolchain/toolchainservice'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import { ToolStatus } from '@bindings/chatclaw/internal/services/toolchain/models'
import { Download, Check, Loader2, Package, FolderOpen, Play } from 'lucide-vue-next'
import { RuntimeStatus } from '@bindings/chatclaw/internal/openclaw/runtime/models'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'
import TestInstallDialog from './TestInstallDialog.vue'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const appStore = useAppStore()
const gatewayStore = useOpenClawGatewayStore()
const { runtimePhase } = storeToRefs(gatewayStore)
const { locale: currentLocale, switchLocale } = useLocale()
const testInstallOpen = ref(false)

// 语言选项 - 直接用对应语言的名称显示
const languageOptions = computed(() => {
  const labels: Record<Locale, string> = {
    'zh-CN': '简体中文',
    'en-US': 'English',
    'ar-SA': 'العربية',
    'bn-BD': 'বাংলা',
    'de-DE': 'Deutsch',
    'es-ES': 'Español',
    'fr-FR': 'Français',
    'hi-IN': 'हिन्दी',
    'it-IT': 'Italiano',
    'ja-JP': '日本語',
    'ko-KR': '한국어',
    'pt-BR': 'Português',
    'sl-SI': 'Slovenščina',
    'tr-TR': 'Türkçe',
    'vi-VN': 'Tiếng Việt',
    'zh-TW': '繁體中文',
  }
  return SUPPORTED_LOCALES.map((locale) => ({
    value: locale,
    label: labels[locale] || locale,
  }))
})

// 主题选项
const themeOptions = [
  { value: 'light', label: 'settings.themes.light' },
  { value: 'dark', label: 'settings.themes.dark' },
  { value: 'system', label: 'settings.themes.system' },
]

// 当前语言显示文本
const currentLanguageLabel = computed(() => {
  const option = languageOptions.value.find((opt) => opt.value === currentLocale.value)
  return option ? option.label : ''
})

// 当前主题显示文本
const currentThemeLabel = computed(() => {
  const option = themeOptions.find((opt) => opt.value === appStore.theme)
  return option ? t(option.label) : ''
})

// 处理语言切换
const handleLanguageChange = (value: AcceptableValue) => {
  if (typeof value === 'string' && SUPPORTED_LOCALES.includes(value as Locale)) {
    void switchLocale(value as Locale)
  }
}

// 处理主题切换
const handleThemeChange = (value: AcceptableValue) => {
  if (typeof value === 'string') {
    appStore.setTheme(value as Theme)
  }
}

// ---- Toolchain ----

interface ToolDef {
  id: string
  nameKey: string
  descKey: string
}

interface DownloadProgress {
  tool: string
  url: string
  totalSize: number
  downloaded: number
  percent: number
  speed: number
  elapsedTime: number
  remaining: number
}

const toolDefs: ToolDef[] = [
  {
    id: 'uv',
    nameKey: 'settings.general.toolchain.uv.name',
    descKey: 'settings.general.toolchain.uv.description',
  },
  {
    id: 'bun',
    nameKey: 'settings.general.toolchain.bun.name',
    descKey: 'settings.general.toolchain.bun.description',
  },
  {
    id: 'codex',
    nameKey: 'settings.general.toolchain.codex.name',
    descKey: 'settings.general.toolchain.codex.description',
  },
]

const toolStatuses = reactive<Record<string, ToolStatus>>({})
const installErrors = reactive<Record<string, boolean>>({})
const downloadProgress = reactive<Record<string, DownloadProgress>>({})

interface OpenClawStatus {
  name: string
  installed: boolean
  installed_version: string
  installing: boolean
  runtime_path: string
}
const openclawStatus = ref<OpenClawStatus | null>(null)
const openclawInstallError = ref(false)

/** Install row busy: local OSS install or runtime dir mutation from OpenClaw manager upgrade. */
const openclawExtensionRuntimeBusy = computed(
  () =>
    !!openclawStatus.value?.installing || isOpenClawRuntimeMutatingPhase(runtimePhase.value)
)

const isDevMode = ref(false)

const loadOpenClawStatus = async () => {
  try {
    const status = await ToolchainService.GetOpenClawRuntimeStatus()
    if (status) {
      openclawStatus.value = {
        name: status.name,
        installed: status.installed,
        installed_version: status.installed_version,
        installing: status.installing,
        runtime_path: status.runtime_path,
      }
      openclawInstallError.value = false
    }
  } catch (e) {
    console.error('Failed to load openclaw status:', e)
    openclawStatus.value = {
      name: 'openclaw',
      installed: false,
      installed_version: '',
      installing: false,
      runtime_path: '',
    }
  }
}

const handleInstallOpenClaw = async () => {
  if (isOpenClawRuntimeMutatingPhase(runtimePhase.value) && !openclawStatus.value?.installing) {
    return
  }
  openclawInstallError.value = false
  if (openclawStatus.value) {
    openclawStatus.value = { ...openclawStatus.value, installing: true }
  }
  try {
    await OpenClawRuntimeService.InstallAndStartRuntime()
    await nextTick()
    await loadOpenClawStatus()
  } catch (e) {
    console.error('Failed to install openclaw runtime:', e)
    openclawInstallError.value = true
    if (openclawStatus.value) {
      openclawStatus.value = { ...openclawStatus.value, installing: false }
    }
  }
}

// 加载是否为开发模式
const loadDevMode = async () => {
  try {
    isDevMode.value = await ToolchainService.IsDevMode()
  } catch (e) {
    console.error('Failed to load dev mode:', e)
    isDevMode.value = false
  }
}

// 清除卡住的安装状态
const clearInstallingState = async (toolId: string) => {
  try {
    await ToolchainService.ClearInstallingState(toolId)
    await loadToolStatuses()
  } catch (e) {
    console.error('Failed to clear installing state:', e)
  }
}

// 格式化文件大小
const formatFileSize = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// 格式化下载速度
const formatSpeed = (kbPerSec: number): string => {
  if (kbPerSec >= 1024) {
    return (kbPerSec / 1024).toFixed(1) + ' MB/s'
  }
  return kbPerSec.toFixed(1) + ' KB/s'
}

// 格式化剩余时间
const formatRemaining = (ms: number): string => {
  if (ms <= 0) return ''
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const secs = seconds % 60
  return `${minutes}m ${secs}s`
}

const loadToolStatuses = async () => {
  try {
    const statuses = await ToolchainService.GetAllToolStatus()
    for (const s of statuses) {
      toolStatuses[s.name] = s
    }
  } catch (e) {
    console.error('Failed to load toolchain statuses:', e)
  }
  // 加载 OpenClaw 运行时状态
  await loadOpenClawStatus()
  // 加载开发模式
  await loadDevMode()
}

const handleOpenExtensionPath = async (pathStr: string | undefined) => {
  const p = pathStr?.trim()
  if (!p) return
  try {
    await BrowserService.OpenPathInFileManager(p)
  } catch (e) {
    console.error('Failed to open path in file manager:', e)
    toast.error(getErrorMessage(e) || t('settings.general.toolchain.openPathFailed'))
  }
}

const handleInstall = async (toolId: string) => {
  installErrors[toolId] = false
  const existing = toolStatuses[toolId]
  if (existing) {
    toolStatuses[toolId] = new ToolStatus({ ...existing, installing: true })
  }
  try {
    await ToolchainService.InstallTool(toolId)
    // When InstallTool resolves, install is done; refresh from backend so UI updates
    // even if toolchain:status event was delivered in a context that didn't trigger re-render
    await nextTick()
    await loadToolStatuses()
  } catch (e) {
    console.error(`Failed to install ${toolId}:`, e)
    installErrors[toolId] = true
    // 安装失败时需要重新加载状态
    await loadToolStatuses()
  }
}

let unsubscribeToolchain: (() => void) | null = null
let unsubscribeProgress: (() => void) | null = null
let unsubscribeOpenClawStatus: (() => void) | null = null

onMounted(() => {
  void loadToolStatuses()
  void gatewayStore.poll()
  unsubscribeOpenClawStatus = Events.On('openclaw:status', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data) {
      gatewayStore.ingestRuntimeStatus(RuntimeStatus.createFrom(data))
    }
  })
  unsubscribeToolchain = Events.On('toolchain:status', async (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data && data.name) {
      // Assign plain object so Vue reactivity tracks the update reliably
      const status = ToolStatus.createFrom(data)
      toolStatuses[data.name] = { ...status }
      installErrors[data.name] = false
      if (!data.installing) {
        delete downloadProgress[data.name]
      }
      await nextTick()
    }
  })
  // 监听下载进度
  unsubscribeProgress = Events.On('toolchain:download-progress', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data && data.tool) {
      downloadProgress[data.tool] = data
    }
  })
})

onUnmounted(() => {
  unsubscribeToolchain?.()
  unsubscribeToolchain = null
  unsubscribeProgress?.()
  unsubscribeProgress = null
  unsubscribeOpenClawStatus?.()
  unsubscribeOpenClawStatus = null
})
</script>

<template>
  <div class="flex w-full flex-col gap-4">
    <SettingsCard :title="t('settings.general.title')">
      <!-- 语言设置 -->
      <SettingsItem :label="t('settings.general.language')">
        <Select :model-value="currentLocale" @update:model-value="handleLanguageChange">
          <SelectTrigger class="w-80">
            <SelectValue>{{ currentLanguageLabel }}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="option in languageOptions" :key="option.value" :value="option.value">
              {{ option.label }}
            </SelectItem>
          </SelectContent>
        </Select>
      </SettingsItem>

      <!-- 主题设置 -->
      <SettingsItem :label="t('settings.general.theme')" :bordered="false">
        <Select :model-value="appStore.theme" @update:model-value="handleThemeChange">
          <SelectTrigger class="w-80">
            <SelectValue>{{ currentThemeLabel }}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="option in themeOptions" :key="option.value" :value="option.value">
              {{ t(option.label) }}
            </SelectItem>
          </SelectContent>
        </Select>
      </SettingsItem>
    </SettingsCard>

    <!-- 开发工具 -->
    <SettingsCard :title="t('settings.general.toolchain.title')">
      <!-- OpenClaw 运行环境（独立卡片，不走 toolDefs 循环） -->
      <div
        class="flex items-start gap-4 border-b border-border p-4 dark:border-white/10"
      >
        <div
          class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/50 text-muted-foreground dark:border-white/10 dark:bg-white/5"
        >
          <Package class="size-4" />
        </div>
        <div class="min-w-0 flex-1 pt-0.5">
            <span class="text-sm font-medium text-foreground">{{
              t('settings.general.toolchain.openclaw.name')
            }}</span>
            <p class="text-xs text-muted-foreground truncate">
              {{ t('settings.general.toolchain.openclaw.description') }}
            </p>
            <button
              v-if="openclawStatus?.runtime_path"
              type="button"
              class="mt-1 flex min-w-0 max-w-full items-center gap-1 rounded-sm text-left text-xs text-muted-foreground/70 truncate hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :title="t('settings.general.toolchain.openPathHint')"
              @click="handleOpenExtensionPath(openclawStatus.runtime_path)"
            >
              <FolderOpen class="size-3 shrink-0 text-muted-foreground" />
              <span class="truncate">{{ openclawStatus.runtime_path }}</span>
            </button>
            <p
              v-if="openclawStatus?.installed && openclawStatus?.installed_version"
              class="mt-0.5 text-xs text-muted-foreground/60"
              :title="openclawStatus.installed_version"
            >
              {{ t('settings.general.toolchain.testInstall.version') }}:
              {{ openclawStatus.installed_version }}
            </p>

            <!-- Download Progress -->
            <div
              v-if="downloadProgress['openclaw'] && openclawStatus?.installing"
              class="mt-2 flex flex-col gap-1"
            >
              <div class="flex items-center justify-between text-xs">
                <span class="text-muted-foreground"
                  >{{ downloadProgress['openclaw'].percent.toFixed(1) }}%</span
                >
                <span class="text-muted-foreground">{{
                  formatSpeed(downloadProgress['openclaw'].speed)
                }}</span>
              </div>
              <div class="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                <div
                  class="h-full bg-primary transition-all duration-300"
                  :style="{ width: `${downloadProgress['openclaw'].percent}%` }"
                />
              </div>
              <div class="flex items-center justify-between text-xs text-muted-foreground">
                <span
                  >{{ formatFileSize(downloadProgress['openclaw'].downloaded) }} /
                  {{ formatFileSize(downloadProgress['openclaw'].totalSize) }}</span
                >
                <span v-if="downloadProgress['openclaw'].remaining > 0">{{
                  formatRemaining(downloadProgress['openclaw'].remaining)
                }}</span>
              </div>
            </div>
        </div>

        <div class="flex shrink-0 flex-col items-end gap-2 pt-0.5">
          <template v-if="openclawStatus?.installed && !openclawExtensionRuntimeBusy">
            <span
              class="inline-flex items-center gap-1 whitespace-nowrap rounded-md px-2 py-1 text-xs font-medium text-muted-foreground ring-1 ring-border dark:ring-white/10"
            >
              <Check class="size-3 shrink-0" />
              {{ t('settings.general.toolchain.installed') }}
            </span>
          </template>

          <!-- Installing / runtime dir busy (e.g. upgrade from OpenClaw manager) -->
          <span
            v-else-if="openclawExtensionRuntimeBusy"
            class="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground"
          >
            <Loader2 class="size-3 animate-spin" />
            {{
              openclawStatus?.installing
                ? t('settings.general.toolchain.installing')
                : t('settings.openclawRuntime.upgrading')
            }}
          </span>

          <!-- Install (missing runtime) -->
          <template v-else>
            <span v-if="openclawInstallError" class="text-xs text-destructive">{{
              t('settings.general.toolchain.installFailed')
            }}</span>
            <Button size="sm" variant="outline" @click="handleInstallOpenClaw">
              <Download class="size-3.5" />
              {{ t('settings.general.toolchain.install') }}
            </Button>
          </template>
        </div>
      </div>

      <div
        v-for="(tool, index) in toolDefs"
        :key="tool.id"
        class="flex items-start gap-4 p-4"
        :class="index < toolDefs.length - 1 && 'border-b border-border dark:border-white/10'"
      >
        <div
          class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/50 text-muted-foreground dark:border-white/10 dark:bg-white/5"
        >
          <Package class="size-4" />
        </div>
        <div class="min-w-0 flex-1 pt-0.5">
            <span class="text-sm font-medium text-foreground">{{ t(tool.nameKey) }}</span>
            <p class="text-xs text-muted-foreground truncate">{{ t(tool.descKey) }}</p>
            <button
              v-if="toolStatuses[tool.id]?.bin_path"
              type="button"
              class="mt-1 flex min-w-0 max-w-full items-center gap-1 rounded-sm text-left text-xs text-muted-foreground/70 truncate hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              :title="t('settings.general.toolchain.openPathHint')"
              @click="handleOpenExtensionPath(toolStatuses[tool.id]?.bin_path)"
            >
              <FolderOpen class="size-3 shrink-0 text-muted-foreground" />
              <span class="truncate">{{ toolStatuses[tool.id]?.bin_path }}</span>
            </button>

            <!-- Download Progress（仅安装中时显示，完成后隐藏，不依赖后端事件顺序） -->
            <div
              v-if="downloadProgress[tool.id] && toolStatuses[tool.id]?.installing"
              class="mt-2 flex flex-col gap-1"
            >
              <div class="flex items-center justify-between text-xs">
                <span class="text-muted-foreground">
                  {{ downloadProgress[tool.id].percent.toFixed(1) }}%
                </span>
                <span class="text-muted-foreground">
                  {{ formatSpeed(downloadProgress[tool.id].speed) }}
                </span>
              </div>
              <div class="h-1.5 w-full overflow-hidden rounded-full bg-muted">
                <div
                  class="h-full bg-primary transition-all duration-300"
                  :style="{ width: `${downloadProgress[tool.id].percent}%` }"
                />
              </div>
              <div class="flex items-center justify-between text-xs text-muted-foreground">
                <span>
                  {{ formatFileSize(downloadProgress[tool.id].downloaded) }} /
                  {{ formatFileSize(downloadProgress[tool.id].totalSize) }}
                </span>
                <span v-if="downloadProgress[tool.id].remaining > 0">
                  {{ formatRemaining(downloadProgress[tool.id].remaining) }}
                </span>
              </div>
            </div>
        </div>

        <div class="flex shrink-0 flex-col items-end gap-2 pt-0.5 sm:flex-row sm:items-center">
          <template v-if="toolStatuses[tool.id]?.installed && !toolStatuses[tool.id]?.installing">
            <div class="flex flex-col items-end gap-2">
              <span
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground ring-1 ring-border dark:ring-white/10"
              >
                <Check class="size-3" />
                {{ t('settings.general.toolchain.installed') }}
              </span>
              <template v-if="toolStatuses[tool.id]?.has_update">
                <p
                  v-if="toolStatuses[tool.id]?.latest_version"
                  class="max-w-[200px] text-right text-xs text-muted-foreground"
                >
                  {{
                    t('settings.general.toolchain.newVersionHint', {
                      version: toolStatuses[tool.id]?.latest_version,
                    })
                  }}
                </p>
                <Button size="sm" variant="outline" @click="handleInstall(tool.id)">
                  <Download class="size-3.5" />
                  {{ t('settings.general.toolchain.update') }}
                </Button>
              </template>
            </div>
          </template>

          <!-- Installing state -->
          <span
            v-else-if="toolStatuses[tool.id]?.installing"
            class="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
          >
            <Loader2 class="size-3 animate-spin" />
            {{ t('settings.general.toolchain.installing') }}
            <Button
              v-if="isDevMode"
              size="sm"
              variant="ghost"
              class="ml-1 h-5 px-1 text-xs text-muted-foreground hover:text-destructive"
              @click="clearInstallingState(tool.id)"
            >
              {{ t('settings.general.toolchain.clearState') }}
            </Button>
          </span>

          <!-- Install button -->
          <template v-else>
            <span v-if="installErrors[tool.id]" class="text-xs text-destructive">
              {{ t('settings.general.toolchain.installFailed') }}
            </span>
            <Button size="sm" variant="outline" @click="handleInstall(tool.id)">
              <Download class="size-3.5" />
              {{ t('settings.general.toolchain.install') }}
            </Button>
          </template>
        </div>
      </div>

      <!-- Dev-only: same p-4 as toolchain rows so actions align on all breakpoints -->
      <div
        v-if="isDevMode"
        class="flex justify-end border-t border-border p-4 dark:border-white/10"
      >
        <Button variant="outline" size="sm" @click="testInstallOpen = true">
          <Play class="mr-1 size-3.5" />
          {{ t('settings.general.toolchain.testInstall.button') }}
        </Button>
      </div>
    </SettingsCard>

    <!-- 测试安装对话框 -->
    <TestInstallDialog v-model:open="testInstallOpen" />
  </div>
</template>
