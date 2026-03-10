<script setup lang="ts">
import { computed, ref, reactive, onMounted, onUnmounted } from 'vue'
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
import { useAppStore, type Theme } from '@/stores'
import { useLocale } from '@/composables/useLocale'
import * as ToolchainService from '@bindings/chatclaw/internal/services/toolchain/toolchainservice'
import { ToolStatus } from '@bindings/chatclaw/internal/services/toolchain/models'
import { Download, Check, Loader2, Package } from 'lucide-vue-next'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

const { t } = useI18n()
const appStore = useAppStore()
const { locale: currentLocale, switchLocale } = useLocale()

// 语言选项
const languageOptions = [
  { value: 'zh-CN', label: 'settings.languages.zhCN' },
  { value: 'en-US', label: 'settings.languages.enUS' },
]

// 主题选项
const themeOptions = [
  { value: 'light', label: 'settings.themes.light' },
  { value: 'dark', label: 'settings.themes.dark' },
  { value: 'system', label: 'settings.themes.system' },
]

// 当前语言显示文本
const currentLanguageLabel = computed(() => {
  const option = languageOptions.find((opt) => opt.value === currentLocale.value)
  return option ? t(option.label) : ''
})

// 当前主题显示文本
const currentThemeLabel = computed(() => {
  const option = themeOptions.find((opt) => opt.value === appStore.theme)
  return option ? t(option.label) : ''
})

// 处理语言切换
const handleLanguageChange = (value: AcceptableValue) => {
  if (typeof value === 'string') {
    void switchLocale(value as 'zh-CN' | 'en-US')
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
  { id: 'uv', nameKey: 'settings.general.toolchain.uv.name', descKey: 'settings.general.toolchain.uv.description' },
  { id: 'bun', nameKey: 'settings.general.toolchain.bun.name', descKey: 'settings.general.toolchain.bun.description' },
  { id: 'codex', nameKey: 'settings.general.toolchain.codex.name', descKey: 'settings.general.toolchain.codex.description' },
]

const toolStatuses = reactive<Record<string, ToolStatus>>({})
const installErrors = reactive<Record<string, boolean>>({})
const downloadProgress = reactive<Record<string, DownloadProgress>>({})

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
}

const handleInstall = async (toolId: string) => {
  installErrors[toolId] = false
  const existing = toolStatuses[toolId]
  if (existing) {
    toolStatuses[toolId] = new ToolStatus({ ...existing, installing: true })
  }
  try {
    await ToolchainService.InstallTool(toolId)
    await loadToolStatuses()
  } catch (e) {
    console.error(`Failed to install ${toolId}:`, e)
    installErrors[toolId] = true
    await loadToolStatuses()
  }
}

let unsubscribeToolchain: (() => void) | null = null
let unsubscribeProgress: (() => void) | null = null

onMounted(() => {
  void loadToolStatuses()
  unsubscribeToolchain = Events.On('toolchain:status', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data && data.name) {
      toolStatuses[data.name] = ToolStatus.createFrom(data)
      installErrors[data.name] = false
      // 安装完成后清除进度
      if (!data.installing) {
        delete downloadProgress[data.name]
      }
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
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <SettingsCard :title="t('settings.general.title')">
      <!-- 语言设置 -->
      <SettingsItem :label="t('settings.general.language')">
        <Select :model-value="currentLocale" @update:model-value="handleLanguageChange">
          <SelectTrigger class="w-80">
            <SelectValue>{{ currentLanguageLabel }}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="option in languageOptions" :key="option.value" :value="option.value">
              {{ t(option.label) }}
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
      <div
        v-for="(tool, index) in toolDefs"
        :key="tool.id"
        class="flex items-center justify-between gap-4 p-4"
        :class="index < toolDefs.length - 1 && 'border-b border-border dark:border-white/10'"
      >
        <div class="flex items-center gap-3 min-w-0">
          <div
            class="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border bg-muted/50 text-muted-foreground dark:border-white/10 dark:bg-white/5"
          >
            <Package class="size-4" />
          </div>
          <div class="min-w-0">
            <span class="text-sm font-medium text-foreground">{{ t(tool.nameKey) }}</span>
            <p class="text-xs text-muted-foreground truncate">{{ t(tool.descKey) }}</p>
          </div>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <!-- Installed badge -->
          <span
            v-if="toolStatuses[tool.id]?.installed && !toolStatuses[tool.id]?.installing"
            class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground ring-1 ring-border dark:ring-white/10"
          >
            <Check class="size-3" />
            {{ t('settings.general.toolchain.installed') }}
          </span>

          <!-- Installing spinner -->
          <span
            v-else-if="toolStatuses[tool.id]?.installing"
            class="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
          >
            <Loader2 class="size-3 animate-spin" />
            {{ t('settings.general.toolchain.installing') }}
          </span>

          <!-- Download Progress -->
          <div
            v-else-if="downloadProgress[tool.id]"
            class="flex flex-col gap-1 min-w-[120px]"
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
                {{ formatFileSize(downloadProgress[tool.id].downloaded) }} / {{ formatFileSize(downloadProgress[tool.id].totalSize) }}
              </span>
              <span v-if="downloadProgress[tool.id].remaining > 0">
                {{ formatRemaining(downloadProgress[tool.id].remaining) }}
              </span>
            </div>
          </div>

          <!-- Install button -->
          <template v-else>
            <span
              v-if="installErrors[tool.id]"
              class="text-xs text-destructive"
            >
              {{ t('settings.general.toolchain.installFailed') }}
            </span>
            <Button
              size="sm"
              variant="outline"
              @click="handleInstall(tool.id)"
            >
              <Download class="size-3.5" />
              {{ t('settings.general.toolchain.install') }}
            </Button>
          </template>
        </div>
      </div>
    </SettingsCard>
  </div>
</template>
