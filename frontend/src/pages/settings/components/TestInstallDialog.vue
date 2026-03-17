<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import * as ToolchainService from '@bindings/chatclaw/internal/services/toolchain/toolchainservice'
import {
  DownloadMethod,
  TestInstallConfig,
} from '@bindings/chatclaw/internal/services/toolchain/models'
import { Download, X, Loader2, Play, Square } from 'lucide-vue-next'

const { t } = useI18n()

// Props
const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

// 工具列表
const tools = [
  { id: 'codex', nameKey: 'settings.general.toolchain.codex.name' },
  { id: 'uv', nameKey: 'settings.general.toolchain.uv.name' },
  { id: 'bun', nameKey: 'settings.general.toolchain.bun.name' },
]

// 下载方式
const downloadMethods = [
  { id: 'direct', nameKey: 'settings.general.toolchain.testInstall.direct' },
  { id: 'proxy', nameKey: 'settings.general.toolchain.testInstall.proxy' },
  { id: 'oss', nameKey: 'settings.general.toolchain.testInstall.oss' },
]

// 代理列表（预设）
const proxyList = [
  { id: 'gh-proxy', name: 'gh-proxy.org', url: 'https://gh-proxy.org/' },
  { id: 'hk-proxy', name: 'hk.gh-proxy.org', url: 'https://hk.gh-proxy.org/' },
  { id: 'cdn-proxy', name: 'cdn.gh-proxy.org', url: 'https://cdn.gh-proxy.org/' },
  { id: 'edge-proxy', name: 'edgeone.gh-proxy.org', url: 'https://edgeone.gh-proxy.org/' },
  { id: 'custom', name: t('settings.general.toolchain.testInstall.custom'), url: '' },
]

// 状态
const selectedTool = ref('codex')
const selectedMethod = ref('direct')
const selectedProxy = ref('gh-proxy')
const customProxyURL = ref('')
const isRunning = ref(false)
const isFinished = ref(false)
const progress = reactive({
  percent: 0,
  downloaded: 0,
  totalSize: 0,
  speed: 0,
  remaining: 0,
  status: '',
  message: '',
})
const result = ref<{
  success: boolean
  message: string
  version: string
  methodUsed: string
} | null>(null)

// 计算当前使用的代理 URL
const currentProxyURL = computed(() => {
  if (selectedMethod.value !== 'proxy') return ''
  if (selectedProxy.value === 'custom') return customProxyURL.value
  const proxy = proxyList.find((p) => p.id === selectedProxy.value)
  return proxy?.url || ''
})

// 是否显示自定义代理输入框
const showCustomProxyInput = computed(() => {
  return selectedMethod.value === 'proxy' && selectedProxy.value === 'custom'
})

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

// 开始测试安装
const handleStart = async () => {
  isRunning.value = true
  isFinished.value = false
  result.value = null
  progress.percent = 0
  progress.downloaded = 0
  progress.totalSize = 0
  progress.speed = 0
  progress.remaining = 0
  progress.status = 'starting'
  progress.message = t('settings.general.toolchain.testInstall.starting')

  try {
    // 使用正确的类型创建配置
    const config = new TestInstallConfig({
      tool: selectedTool.value,
      downloadMethod: selectedMethod.value as unknown as DownloadMethod,
      proxyURL: currentProxyURL.value,
    })

    const res = await ToolchainService.TestInstall(config)
    isFinished.value = true
    isRunning.value = false

    if (!res) {
      result.value = {
        success: false,
        message: 'No response from server',
        version: '',
        methodUsed: '',
      }
      progress.status = 'failed'
      progress.message = 'No response from server'
      return
    }

    result.value = {
      success: res.success,
      message: res.message,
      version: res.version,
      methodUsed: res.methodUsed,
    }

    if (res.success) {
      progress.status = 'completed'
      progress.message = t('settings.general.toolchain.testInstall.completed')
    } else {
      progress.status = 'failed'
      progress.message = res.message
    }
  } catch (error) {
    isFinished.value = true
    isRunning.value = false
    progress.status = 'failed'
    progress.message = String(error)
    result.value = {
      success: false,
      message: String(error),
      version: '',
      methodUsed: '',
    }
  }
}

// 取消下载
const handleCancel = async () => {
  try {
    await ToolchainService.AbortDownload(selectedTool.value)
  } catch (error) {
    console.error('Failed to abort download:', error)
  }
}

// 关闭对话框
const handleClose = () => {
  if (isRunning.value) {
    handleCancel()
  }
  emit('update:open', false)
}

// 重置状态
const resetState = () => {
  isRunning.value = false
  isFinished.value = false
  result.value = null
  progress.percent = 0
  progress.downloaded = 0
  progress.totalSize = 0
  progress.speed = 0
  progress.remaining = 0
  progress.status = ''
  progress.message = ''
}

// 监听对话框打开状态
const handleOpenChange = (open: boolean) => {
  if (!open) {
    if (isRunning.value) {
      handleCancel()
    }
    resetState()
  }
  emit('update:open', open)
}

// 订阅事件
let unsubscribeProgress: (() => void) | null = null
let unsubscribeStatus: (() => void) | null = null

onMounted(() => {
  unsubscribeProgress = Events.On('toolchain:download-progress', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data && data.tool === selectedTool.value) {
      progress.percent = data.percent
      progress.downloaded = data.downloaded
      progress.totalSize = data.totalSize
      progress.speed = data.speed
      progress.remaining = data.remaining
    }
  })

  unsubscribeStatus = Events.On('toolchain:test-install-status', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data && data.tool === selectedTool.value) {
      progress.status = data.status
      progress.message = data.message
    }
  })
})

onUnmounted(() => {
  unsubscribeProgress?.()
  unsubscribeProgress = null
  unsubscribeStatus?.()
  unsubscribeStatus = null
})
</script>

<template>
  <Dialog :open="open" @update:open="handleOpenChange">
    <DialogContent class="max-w-md" :show-close-button="!isRunning">
      <DialogHeader>
        <DialogTitle>{{ t('settings.general.toolchain.testInstall.title') }}</DialogTitle>
      </DialogHeader>

      <div class="space-y-4 py-4">
        <!-- 工具选择 -->
        <div class="space-y-2">
          <Label>{{ t('settings.general.toolchain.testInstall.selectTool') }}</Label>
          <Select v-model="selectedTool" :disabled="isRunning">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="tool in tools" :key="tool.id" :value="tool.id">
                {{ t(tool.nameKey) }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <!-- 下载方式选择 -->
        <div class="space-y-2">
          <Label>{{ t('settings.general.toolchain.testInstall.downloadMethod') }}</Label>
          <Select v-model="selectedMethod" :disabled="isRunning">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="method in downloadMethods" :key="method.id" :value="method.id">
                {{ t(method.nameKey) }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <!-- 代理选择（当选择代理时） -->
        <div v-if="selectedMethod === 'proxy'" class="space-y-2">
          <Label>{{ t('settings.general.toolchain.testInstall.selectProxy') }}</Label>
          <Select v-model="selectedProxy" :disabled="isRunning">
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem v-for="proxy in proxyList" :key="proxy.id" :value="proxy.id">
                {{ proxy.name }}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <!-- 自定义代理 URL 输入 -->
        <div v-if="showCustomProxyInput" class="space-y-2">
          <Label>{{ t('settings.general.toolchain.testInstall.customProxyURL') }}</Label>
          <Input
            v-model="customProxyURL"
            :placeholder="t('settings.general.toolchain.testInstall.customProxyPlaceholder')"
            :disabled="isRunning"
          />
        </div>

        <!-- 进度显示（仅下载中显示，完成后隐藏） -->
        <div v-if="isRunning" class="rounded-lg border border-border bg-muted/50 p-4">
          <div class="mb-2 flex items-center justify-between">
            <span class="text-sm font-medium">
              {{
                progress.status === 'completed'
                  ? t('settings.general.toolchain.testInstall.completed')
                  : progress.status === 'failed'
                    ? t('settings.general.toolchain.testInstall.failed')
                    : t('settings.general.toolchain.testInstall.downloading')
              }}
            </span>
            <span v-if="isRunning" class="text-sm text-muted-foreground">
              {{ progress.percent.toFixed(1) }}%
            </span>
          </div>

          <!-- 进度条 -->
          <div v-if="isRunning" class="mb-3 h-2 w-full overflow-hidden rounded-full bg-muted">
            <div
              class="h-full bg-primary transition-all duration-300"
              :style="{ width: `${progress.percent}%` }"
            />
          </div>

          <!-- 进度详情 -->
          <div
            v-if="isRunning"
            class="flex items-center justify-between text-xs text-muted-foreground"
          >
            <span>
              {{ formatFileSize(progress.downloaded) }} / {{ formatFileSize(progress.totalSize) }}
            </span>
            <div class="flex items-center gap-2">
              <span>{{ formatSpeed(progress.speed) }}</span>
              <span v-if="progress.remaining > 0">· {{ formatRemaining(progress.remaining) }}</span>
            </div>
          </div>

          <!-- 状态消息 -->
          <div class="mt-2 text-xs text-muted-foreground">
            {{ progress.message }}
          </div>
        </div>

        <!-- 结果显示 -->
        <div
          v-if="result && isFinished"
          class="rounded-lg border p-4"
          :class="
            result.success
              ? 'border-green-500/50 bg-green-500/10'
              : 'border-destructive/50 bg-destructive/10'
          "
        >
          <div class="text-sm">
            <span
              :class="result.success ? 'text-green-600 dark:text-green-400' : 'text-destructive'"
            >
              {{
                result.success
                  ? t('settings.general.toolchain.testInstall.success')
                  : t('settings.general.toolchain.testInstall.failed')
              }}
            </span>
          </div>
          <div v-if="result.version" class="mt-1 text-xs text-muted-foreground">
            {{ t('settings.general.toolchain.testInstall.version') }}: {{ result.version }}
          </div>
          <div v-if="result.methodUsed" class="mt-1 text-xs text-muted-foreground">
            {{ t('settings.general.toolchain.testInstall.methodUsed') }}: {{ result.methodUsed }}
          </div>
          <div class="mt-1 text-xs text-muted-foreground">
            {{ result.message }}
          </div>
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="handleClose">
          {{ isRunning ? t('settings.general.toolchain.testInstall.cancel') : t('common.close') }}
        </Button>
        <Button v-if="isRunning" variant="destructive" @click="handleCancel">
          <Square class="mr-1 size-3.5" />
          {{ t('settings.general.toolchain.testInstall.abort') }}
        </Button>
        <Button v-else :disabled="isRunning" @click="handleStart">
          <Play class="mr-1 size-3.5" />
          {{ t('settings.general.toolchain.testInstall.start') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
