<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useColorMode } from '@vueuse/core'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { RefreshCw, Loader2, ExternalLink, Download, Square, ChevronDown, ChevronUp, RotateCcw } from 'lucide-vue-next'
import AnsiToHtml from 'ansi-to-html'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import {
  useNavigationStore,
  useOpenClawGatewayStore,
  gatewayBadgeClass,
  gatewaySidebarTagLoaderClass,
  GatewayVisualStatus,
} from '@/stores'
import { cn } from '@/lib/utils'
import {
  RuntimeStatus,
  GatewayConnectionState,
  RuntimeUpgradeResult,
} from '@bindings/chatclaw/internal/openclaw/runtime/models'
import { toast, TOAST_DURATION_HINT } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import SettingsCard from './SettingsCard.vue'
import OpenClawDoctorConsole from '@/components/openclaw/OpenClawDoctorConsole.vue'
import { Events } from '@wailsio/runtime'

/** When true, match Figma ChatClaw openclaw管家: page column 874px, cards 700px centered */
const props = withDefaults(
  defineProps<{
    figmaLayout?: boolean
  }>(),
  { figmaLayout: false }
)

const { t } = useI18n()
const navigationStore = useNavigationStore()
const gatewayStore = useOpenClawGatewayStore()

const status = ref<RuntimeStatus>(new RuntimeStatus({ phase: 'idle' }))
const gatewayState = ref<GatewayConnectionState>(new GatewayConnectionState())
const restarting = ref(false)
const stopping = ref(false)
const upgrading = ref(false)
const resetting = ref(false)
const resetConfirmOpen = ref(false)
const resetElapsed = ref(0)
let resetTimer: ReturnType<typeof setInterval> | null = null

// 后端事件取消订阅函数
let unsubscribeStatus: (() => void) | undefined
let unsubscribeGatewayState: (() => void) | undefined

// 升级详情相关
const showUpgradeDetails = ref(false)
const upgradeOutputEl = ref<HTMLDivElement | null>(null)

// Gateway 日志相关
const showGatewayLog = ref(false)
const gatewayLogEl = ref<HTMLDivElement | null>(null)
const gatewayLogHtml = ref('')
let gatewayLogPollInterval: ReturnType<typeof setInterval> | null = null
const colorMode = useColorMode()

/** Build ansi-to-html converter with theme-aware colors (mirrors OpenClawDoctorConsole). */
function buildAnsiConverter(dark: boolean) {
  return new AnsiToHtml({
    fg: dark ? '#e2e8f0' : '#1e293b',
    bg: dark ? '#0f172a' : '#f8fafc',
    newline: true,
    escapeXML: true,
    colors: {
      0: dark ? '#6b7280' : '#6b7280',
      1: dark ? '#f87171' : '#dc2626',
      2: dark ? '#4ade80' : '#16a34a',
      3: dark ? '#facc15' : '#ca8a04',
      4: dark ? '#60a5fa' : '#2563eb',
      5: dark ? '#e879f9' : '#c026d3',
      6: dark ? '#22d3ee' : '#0891b2',
      7: dark ? '#f1f5f9' : '#1e293b',
      30: dark ? '#64748b' : '#374151',
      31: dark ? '#f87171' : '#dc2626',
      32: dark ? '#4ade80' : '#16a34a',
      33: dark ? '#facc15' : '#ca8a04',
      34: dark ? '#60a5fa' : '#2563eb',
      35: dark ? '#e879f9' : '#c026d3',
      36: dark ? '#22d3ee' : '#0891b2',
      37: dark ? '#f1f5f9' : '#1e293b',
      90: dark ? '#6b7280' : '#9ca3af',
      91: dark ? '#fca5a5' : '#ef4444',
      92: dark ? '#86efac' : '#22c55e',
      93: dark ? '#fde047' : '#eab308',
      94: dark ? '#93c5fd' : '#3b82f6',
      95: dark ? '#f5d0fe' : '#d946ef',
      96: dark ? '#67e8f9' : '#06b6d4',
      97: dark ? '#ffffff' : '#f8fafc',
    },
  })
}

/** Convert raw gateway log text to ANSI-colored HTML. */
function ansiToHtml(raw: string, dark: boolean): string {
  if (!raw) return ''
  const converter = buildAnsiConverter(dark)
  // Join lines with line breaks so the gateway log renders as a vertical list.
  return converter.toHtml(raw.split('\n').join('\n'))
}

// 检查用户是否滚动到了日志底部（底部 50px 范围内）
const isAtLogBottom = () => {
  if (!gatewayLogEl.value) return true
  const el = gatewayLogEl.value
  const threshold = 50
  return el.scrollHeight - el.scrollTop - el.clientHeight <= threshold
}

// Gateway 日志轮询（启动期间实时读取日志尾部）
const startGatewayLogPolling = async () => {
  stopGatewayLogPolling()
  gatewayLogHtml.value = ''
  // 启动时显示日志区域，等待内容
  showGatewayLog.value = true

  const poll = async () => {
    try {
      const log = await OpenClawRuntimeService.GatewayLogTail(200)
      if (log && log.trim()) {
        gatewayLogHtml.value = ansiToHtml(log, colorMode.value === 'dark')
        await nextTick()
        // 只有用户在底部时才自动滚动
        if (gatewayLogEl.value && isAtLogBottom()) {
          gatewayLogEl.value.scrollTop = gatewayLogEl.value.scrollHeight
        }
      }
    } catch {
      // ignore errors during polling
    }
  }

  await poll()
  gatewayLogPollInterval = setInterval(poll, 1000)
}

const stopGatewayLogPolling = () => {
  if (gatewayLogPollInterval) {
    clearInterval(gatewayLogPollInterval)
    gatewayLogPollInterval = null
  }
}

// 手动查看网关日志
const handleShowGatewayLog = async () => {
  showGatewayLog.value = true
  stopGatewayLogPolling()

  const poll = async () => {
    try {
      const log = await OpenClawRuntimeService.GatewayLogTail(200)
      if (log && log.trim()) {
        gatewayLogHtml.value = ansiToHtml(log, colorMode.value === 'dark')
        await nextTick()
        // 只有用户在底部时才自动滚动
        if (gatewayLogEl.value && isAtLogBottom()) {
          gatewayLogEl.value.scrollTop = gatewayLogEl.value.scrollHeight
        }
      }
    } catch {
      // ignore errors
    }
  }

  await poll()
  gatewayLogPollInterval = setInterval(poll, 2000)
}

// 自动启动开关
const autoStart = ref(true)

// 继续/重新升级弹窗
const showContinueRestartDialog = ref(false)
const existingUpgradeResult = ref<RuntimeUpgradeResult | null>(null)

const isActive = computed(() => status.value.phase === 'connected')
const isTransitioning = computed(() => {
  const p = status.value.phase
  return p === 'starting' || p === 'connecting' || p === 'restarting' || p === 'upgrading'
})

// 是否处于升级阶段（显示进度条）
const isUpgrading = computed(() => status.value.phase === 'upgrading')

const gatewayConnectionLabel = computed(() => {
  if (gatewayState.value.authenticated) return t('settings.openclawRuntime.gateway.authenticated')
  if (gatewayState.value.connected) return t('settings.openclawRuntime.gateway.connected')
  if (gatewayState.value.reconnecting) {
    const err = gatewayState.value.lastError
    if (err) return `${t('settings.openclawRuntime.gateway.reconnecting')} - ${err}`
    return t('settings.openclawRuntime.gateway.reconnecting')
  }
  return t('settings.openclawRuntime.gateway.disconnected')
})

const displayVersion = computed(() => {
  return status.value.installedVersion || t('settings.openclawRuntime.notInstalled')
})

const displayRuntimeSource = computed(() => {
  const source = status.value.runtimeSource
  if (!source) return t('common.na')
  return t(`settings.openclawRuntime.source.${source}`)
})

const displayRuntimePath = computed(() => {
  return status.value.runtimePath || '-'
})

const displayGatewayURL = computed(() => {
  return status.value.gatewayURL || 'http://127.0.0.1'
})

const upgradeProgress = computed(() => {
  return status.value.progress || 0
})

// 格式化为 mm:ss 或 s
const upgradeElapsedDisplay = computed(() => {
  const s = gatewayStore.upgradeElapsed
  if (s < 0) return ''
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const sec = s % 60
  return `${m}m ${sec}s`
})

// 升级输出行列表
const upgradeOutputLines = computed(() => {
  const out = gatewayStore.upgradeOutput || ''
  return out.split('\n').filter((l) => l.trim() !== '')
})

// 启动步骤行（从后端 upgradeOutput 解析，后端在启动期间写入步骤）
// 升级期间 upgradeOutput 包含的是 npm install 输出，不需要在此展示。
const startStepLines = computed(() => {
  // 升级期间不显示启动步骤。
  if (gatewayStore.visualStatus === GatewayVisualStatus.Upgrading) return []
  const out = gatewayStore.upgradeOutput || ''
  return out.split('\n').filter((l) => l.trim() !== '')
})

// 启动步骤面板：启动中（starting）或失败停止（stop，有步骤）时显示。
// running 之后清空步骤缓存。
const isStartInProgress = computed(() => {
  const lines = startStepLines.value
  const status = gatewayStore.visualStatus

  // running 之后立即隐藏，同时清空步骤缓存。
  if (status === GatewayVisualStatus.Running) {
    gatewayStore.upgradeOutput = ''
    return false
  }

  // 启动中显示步骤
  if (status === GatewayVisualStatus.Starting) {
    return true
  }

  // 停止状态：如果有步骤（说明是启动失败），也显示步骤
  if (status === GatewayVisualStatus.Stop && lines.length > 0) {
    return true
  }

  return false
})

const badgeText = computed(() => {
  const v = gatewayStore.visualStatus
  return t(`settings.openclawRuntime.statusBadge.${v}`)
})

const badgeClass = computed(() => gatewayBadgeClass[gatewayStore.visualStatus])

const isGatewayStartingUi = computed(
  () =>
    gatewayStore.visualStatus === GatewayVisualStatus.Starting ||
    gatewayStore.visualStatus === GatewayVisualStatus.Upgrading
)

// 网关是否处于停止状态（idle 或未连接）
const isGatewayStopped = computed(
  () => gatewayStore.visualStatus === GatewayVisualStatus.Stop || status.value.phase === 'idle'
)

function syncGatewayStore() {
  gatewayStore.applySnapshot(status.value, gatewayState.value)
}

const loadStatus = async () => {
  try {
    status.value = await OpenClawRuntimeService.GetStatus()
  } catch (e) {
    console.error('Failed to load OpenClaw status:', e)
  }
  try {
    gatewayState.value = await OpenClawRuntimeService.GetGatewayState()
  } catch (e) {
    console.error('Failed to load OpenClaw gateway state:', e)
  }
  try {
    autoStart.value = await OpenClawRuntimeService.GetAutoStart()
  } catch (e) {
    console.error('Failed to load auto start setting:', e)
  }
  syncGatewayStore()
}

const handleAutoStartChange = async (checked: boolean | 'indeterminate') => {
  const enabled = checked === true
  try {
    await OpenClawRuntimeService.SetAutoStart(enabled)
    autoStart.value = enabled
    toast.success(
      enabled ? t('settings.openclawRuntime.autoStartEnabled') : t('settings.openclawRuntime.autoStartDisabled')
    )
  } catch (e) {
    console.error('Failed to set auto start:', e)
    toast.error(t('settings.openclawRuntime.autoStartFailed'))
  }
}

const handleRestart = async () => {
  restarting.value = true
  try {
    // 清空旧日志
    await OpenClawRuntimeService.ClearGatewayLog()
    gatewayLogHtml.value = ''

    status.value = await OpenClawRuntimeService.RestartGateway()
    gatewayState.value = await OpenClawRuntimeService.GetGatewayState()
    syncGatewayStore()

    // 开始实时读取日志
    startGatewayLogPolling()

    if (status.value.phase === 'error') {
      toast.error(status.value.message || t('settings.openclawRuntime.restartFailed'))
    } else {
      toast.success(t('settings.openclawRuntime.restartSuccess'))
    }
  } catch (e) {
    console.error('Failed to restart OpenClaw gateway:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.restartFailed'))
  } finally {
    restarting.value = false
  }
}

const handleStart = async () => {
  restarting.value = true
  try {
    // 清空旧日志
    await OpenClawRuntimeService.ClearGatewayLog()
    gatewayLogHtml.value = ''

    status.value = await OpenClawRuntimeService.StartGateway()
    syncGatewayStore()

    // 开始实时读取日志
    startGatewayLogPolling()

    if (status.value.phase === 'error') {
      toast.error(status.value.message || t('settings.openclawRuntime.startFailed'))
    } else if (status.value.phase === 'not_installed') {
      toast.default(t('openclawGateway.banner.notInstalled'), TOAST_DURATION_HINT)
    } else {
      toast.success(t('settings.openclawRuntime.startSuccess'))
    }
  } catch (e) {
    console.error('Failed to start OpenClaw gateway:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.startFailed'))
  } finally {
    restarting.value = false
  }
}

const handleStop = async () => {
  if (stopping.value) return
  stopping.value = true
  try {
    await OpenClawRuntimeService.StopGateway()
    toast.success(t('settings.openclawRuntime.stopSuccess'))

    await new Promise((resolve) => setTimeout(resolve, 1500))

    const portStatus = await OpenClawRuntimeService.CheckPortOccupied()
    if (portStatus.occupied) {
      const processName = portStatus.processName || 'Unknown'
      toast.error(
        t('settings.openclawRuntime.portStillOccupiedAfterStopHint', {
          port: portStatus.port,
          pid: portStatus.pid,
        }) + ` (${processName})`
      )
    }

    await loadStatus()
  } catch (e) {
    console.error('Failed to stop OpenClaw gateway:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.stopFailed'))
  } finally {
    stopping.value = false
  }
}

// 取消升级
const handleCancelUpgrade = async () => {
  if (!upgrading.value) return
  try {
    await OpenClawRuntimeService.CancelUpgrade()
    toast.default(t('settings.openclawRuntime.upgradeCancelled'))
    upgrading.value = false
    showUpgradeDetails.value = false
    await loadStatus()
  } catch (e) {
    console.error('Failed to cancel upgrade:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.upgradeCancelFailed'))
  }
}

// 继续升级
const handleContinueUpgrade = async () => {
  if (!existingUpgradeResult.value?.existingVersion) return
  upgrading.value = true
  showContinueRestartDialog.value = false
  showUpgradeDetails.value = true
  try {
    const result = await OpenClawRuntimeService.ContinueUpgrade(existingUpgradeResult.value.existingVersion)
    if (result?.upgraded) {
      toast.success(
        t('settings.openclawRuntime.upgradeSuccess', {
          version: result.currentVersion || '',
        })
      )
    }
    await loadStatus()
  } catch (e) {
    console.error('Failed to continue upgrade:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.upgradeFailed'))
  } finally {
    upgrading.value = false
    showUpgradeDetails.value = false
  }
}

// 重新升级（删除缓存文件夹重走流程）
const handleRestartUpgrade = async () => {
  showContinueRestartDialog.value = false
  // 直接重新走升级流程，后端会删除旧的 staging dir
  void handleUpgradeInternal(false)
}

// 升级核心逻辑，allowCached 控制是否允许使用已有 staging dir
const handleUpgradeInternal = async (allowCached = true) => {
  upgrading.value = true
  showUpgradeDetails.value = true
  try {
    const result = await OpenClawRuntimeService.UpgradeRuntime()
    if (!result) {
      await loadStatus()
      upgrading.value = false
      showUpgradeDetails.value = false
      return
    }

    // 有已有的 staging dir，弹窗让用户选择
    if (result.hasExistingVersion && allowCached) {
      existingUpgradeResult.value = result
      showContinueRestartDialog.value = true
      upgrading.value = false
      return
    }

    if (result.upgraded) {
      toast.success(
        t('settings.openclawRuntime.upgradeSuccess', {
          version: result.currentVersion || result.latestVersion || '',
        })
      )
    } else {
      toast.success(t('settings.openclawRuntime.alreadyLatest'))
    }
    await loadStatus()
  } catch (e) {
    console.error('Failed to upgrade OpenClaw runtime:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.upgradeFailed'))
  } finally {
    upgrading.value = false
    showUpgradeDetails.value = false
  }
}

const handleUpgrade = () => handleUpgradeInternal(true)

const handleOpenDashboard = () => {
  navigationStore.navigateToModule('openclaw-dashboard')
}

// 重置到出厂设置
const handleResetToFactory = async () => {
  resetConfirmOpen.value = false
  resetting.value = true
  resetElapsed.value = 0

  // 启动计时器
  resetTimer = setInterval(() => {
    resetElapsed.value++
  }, 1000)

  try {
    await OpenClawRuntimeService.ResetToFactory()
  } catch (e) {
    console.error('Failed to reset OpenClaw to factory:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.resetFailed'))
    resetting.value = false
    if (resetTimer) {
      clearInterval(resetTimer)
      resetTimer = null
    }
  }
}

const cancelReset = () => {
  resetConfirmOpen.value = false
}

// 格式化重置耗时显示
const resetElapsedDisplay = computed(() => {
  const s = resetElapsed.value
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const sec = s % 60
  return `${m}m ${sec}s`
})

// 升级输出自动滚动
watch(
  () => gatewayStore.upgradeOutput,
  async () => {
    await nextTick()
    if (upgradeOutputEl.value) {
      upgradeOutputEl.value.scrollTop = upgradeOutputEl.value.scrollHeight
    }
  }
)

// Sync local refs when store's runtimePhase changes
watch(
  () => gatewayStore.runtimePhase,
  (phase) => {
    if (status.value.phase !== phase) {
      status.value.phase = phase
    }
    // 升级开始时自动展开详情
    if (phase === 'upgrading') {
      showUpgradeDetails.value = true
    }
    // 连接成功后停止日志轮询
    if (phase === 'connected') {
      stopGatewayLogPolling()
    }
    // 错误时也停止轮询
    if (phase === 'error') {
      stopGatewayLogPolling()
    }
  }
)

// Sync gateway connection state from store
watch(
  () => gatewayStore.lastGatewayState,
  (gw) => {
    gatewayState.value.connected = gw.connected
    gatewayState.value.authenticated = gw.authenticated
    gatewayState.value.reconnecting = gw.reconnecting
    gatewayState.value.lastError = gw.lastError
  },
  { deep: true }
)

// Sync full status from store when detailed fields change
watch(
  () => gatewayStore.visualStatus,
  () => {
    void loadStatus()
  }
)

onMounted(() => {
  void loadStatus()
  void gatewayStore.poll()
  // 直接订阅后端事件，确保 badge 实时跟随后端推送的状态变化
  // （gatewayStore 的订阅在 currentSystem === 'openclaw' 时才激活，
  //  而 settings 可能被 ChatClaw 系统下打开，此时 store 订阅未激活）
  unsubscribeStatus = Events.On('openclaw:status', (event: unknown) => {
    const data = (event as any)?.data?.[0] ?? (event as any)?.data ?? event
    if (data) {
      const s = RuntimeStatus.createFrom(data)
      status.value = s
      gatewayStore.ingestRuntimeStatus(s)
    }
  })
  unsubscribeGatewayState = Events.On('openclaw:gateway-state', (event: unknown) => {
    const data = (event as any)?.data?.[0] ?? (event as any)?.data ?? event
    if (data) {
      const g = GatewayConnectionState.createFrom(data)
      gatewayState.value = g
      gatewayStore.ingestGatewayState(g)
    }
  })
})

onUnmounted(() => {
  unsubscribeStatus?.()
  unsubscribeGatewayState?.()
})
</script>

<template>
  <div class="flex w-full flex-col gap-6">
    <div class="flex flex-col gap-1" :class="props.figmaLayout ? 'px-0' : 'px-1'">
      <h1 class="text-base font-semibold text-foreground">
        {{ t('settings.openclawRuntime.title') }}
      </h1>
      <p class="text-sm text-muted-foreground">
        {{ t('settings.openclawRuntime.pageSubtitle') }}
      </p>
    </div>

    <div
      :class="
        props.figmaLayout
          ? 'mx-auto flex w-full max-w-[96%] flex-col gap-6'
          : 'flex w-full flex-col gap-6'
      "
    >
      <SettingsCard :title="t('settings.openclawRuntime.title')" full-width>
        <template #header-right />

        <!-- Gateway status row: badge + restart -->
        <div
          class="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4 dark:border-white/10"
        >
          <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <span class="shrink-0 text-sm text-foreground">
              {{ t('settings.openclawRuntime.gatewayStatusLabel') }}
            </span>
            <span class="inline-flex min-w-0 items-center gap-1.5">
              <span :class="badgeClass">
                {{ badgeText }}
              </span>
              <Loader2
                v-if="isGatewayStartingUi"
                :class="
                  cn(
                    'size-3.5 shrink-0 animate-spin',
                    gatewaySidebarTagLoaderClass[gatewayStore.visualStatus]
                  )
                "
              />
            </span>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <Button
              size="sm"
              variant="outline"
              :disabled="stopping || restarting || upgrading"
              @click="handleStop"
            >
              <Square v-if="!stopping" class="mr-1.5 size-3.5" />
              <Loader2 v-else class="mr-1.5 size-3.5 animate-spin" />
              {{ t('settings.openclawRuntime.stop') }}
            </Button>
            <Button
              v-if="isGatewayStopped"
              size="sm"
              variant="outline"
              :disabled="restarting || upgrading || isTransitioning"
              @click="handleStart"
            >
              <Loader2 v-if="restarting" class="mr-1.5 size-3.5 animate-spin" />
              {{
                restarting
                  ? t('settings.openclawRuntime.starting')
                  : t('settings.openclawRuntime.start')
              }}
            </Button>
            <Button
              v-else
              size="sm"
              variant="outline"
              :disabled="restarting || upgrading || isTransitioning"
              @click="handleRestart"
            >
              <RefreshCw v-if="!restarting" class="mr-1.5 size-3.5" />
              <Loader2 v-else class="mr-1.5 size-3.5 animate-spin" />
              {{ t('settings.openclawRuntime.restart') }}
            </Button>
          </div>
        </div>

        <!-- 启动步骤进度（仅启动中显示） -->
        <div
          v-if="isStartInProgress"
          class="flex flex-col gap-1.5 border-b border-border p-4"
        >
          <span class="text-xs font-medium text-muted-foreground">
            {{ t('settings.openclawRuntime.starting') }}
          </span>
          <div class="flex flex-col gap-1">
            <div
              v-for="(line, index) in startStepLines"
              :key="index"
              class="flex items-center gap-2 text-xs"
            >
              <span
                v-if="index < startStepLines.length - 1"
                class="inline-flex size-4 shrink-0 items-center justify-center rounded-sm bg-amber-100 text-amber-600 dark:bg-amber-900/50 dark:text-amber-400"
              >
                <Loader2 class="size-2.5 animate-spin" />
              </span>
              <span
                v-else
                class="inline-flex size-4 shrink-0 items-center justify-center rounded-sm bg-emerald-100 text-emerald-600 dark:bg-emerald-900/50 dark:text-emerald-400"
              >
                <svg class="size-2.5" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2.5">
                  <path d="M2 6l3 3 5-5" />
                </svg>
              </span>
              <span class="text-muted-foreground">{{ line }}</span>
            </div>
          </div>
        </div>

        <!-- Gateway 日志（启动中实时显示） -->
        <div
          v-if="showGatewayLog"
          class="border-b border-border dark:border-white/10"
        >
          <div
            class="flex items-center justify-between px-4 pt-3"
          >
            <span class="text-xs font-medium text-foreground">
              {{ t('settings.openclawRuntime.gatewayLog') }}
            </span>
            <Button
              size="sm"
              variant="ghost"
              class="h-5 text-xs"
              @click="showGatewayLog = false"
            >
              <ChevronUp class="size-3" />
            </Button>
          </div>
          <div
            ref="gatewayLogEl"
            class="max-h-60 overflow-y-auto px-4 pb-3"
          >
            <div
              class="rounded border border-border bg-muted/50 px-3 py-2 text-xs dark:border-white/10"
            >
              <div
                v-if="!gatewayLogHtml"
                class="italic text-muted-foreground/50"
              >
                {{ t('settings.openclawRuntime.waitingForLog') }}
              </div>
              <div
                v-if="gatewayLogHtml"
                class="font-mono leading-5 whitespace-pre-wrap"
                v-html="gatewayLogHtml"
              />
            </div>
          </div>
        </div>

        <!-- Gateway connection -->
        <div
          class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
        >
          <span class="shrink-0 text-sm text-foreground">
            {{ t('settings.openclawRuntime.gatewayConnection') }}
          </span>
          <div class="flex items-center gap-2">
            <span class="text-sm text-muted-foreground">{{ gatewayConnectionLabel }}</span>
            <Button
              size="sm"
              variant="ghost"
              class="h-6 text-xs"
              @click="handleShowGatewayLog"
            >
              <svg class="mr-1 size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                <polyline points="14 2 14 8 20 8" />
                <line x1="16" y1="13" x2="8" y2="13" />
                <line x1="16" y1="17" x2="8" y2="17" />
              </svg>
              {{ t('settings.openclawRuntime.viewGatewayLog') }}
            </Button>
          </div>
        </div>

        <!-- Auto-start setting -->
        <div
          class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
        >
          <div class="flex min-w-0 flex-1 flex-col gap-0.5 pr-4">
            <span class="shrink-0 text-sm text-foreground">
              {{ t('settings.openclawRuntime.autoStartLabel') }}
            </span>
            <span class="shrink-0 text-xs text-muted-foreground">
              {{ t('settings.openclawRuntime.autoStartTooltip') }}
            </span>
          </div>
          <Switch :model-value="autoStart" @update:model-value="handleAutoStartChange" />
        </div>

        <!-- Gateway URL -->
        <div
          class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
        >
          <span class="shrink-0 text-sm text-foreground">
            {{ t('settings.openclawRuntime.gatewayEndpoint') }}
          </span>
          <span class="break-all font-mono text-sm text-muted-foreground">{{
            displayGatewayURL
          }}</span>
        </div>

        <!-- Version -->
        <div
          class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
        >
          <span class="shrink-0 text-sm text-foreground">
            {{ t('settings.openclawRuntime.version') }}
          </span>
          <span class="text-sm text-muted-foreground">{{ displayVersion }}</span>
        </div>

        <!-- Runtime source -->
        <div
          class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
        >
          <span class="shrink-0 text-sm text-foreground">
            {{ t('settings.openclawRuntime.runtimeSource') }}
          </span>
          <span class="text-sm text-muted-foreground">{{ displayRuntimeSource }}</span>
        </div>

        <!-- Runtime path -->
        <div
          class="flex items-start justify-between border-b border-border p-4 dark:border-white/10"
        >
          <span class="shrink-0 whitespace-nowrap pt-0.5 text-sm font-medium text-foreground">
            {{ t('settings.openclawRuntime.runtimePath') }}
          </span>
          <div class="min-w-0 flex-1 pl-6 text-right">
            <span class="block break-all font-mono text-sm text-muted-foreground">
              {{ displayRuntimePath }}
            </span>
          </div>
        </div>

        <div
          v-if="status.message && status.phase === 'error'"
          class="border-t border-border px-4 py-3 dark:border-white/10"
        >
          <p class="text-xs text-muted-foreground">{{ status.message }}</p>
        </div>

        <!-- 升级详情区：进度条 + 耗时 + 输出 + 取消按钮 -->
        <div
          v-if="isUpgrading"
          class="border-t border-border dark:border-white/10"
        >
          <!-- 进度条头部 -->
          <div class="flex items-center justify-between px-4 pt-3">
            <div class="flex items-center gap-2">
              <span class="text-xs font-medium text-foreground">
                {{ t('settings.openclawRuntime.upgradeProgress') }}
              </span>
              <span class="text-xs text-muted-foreground">{{ upgradeProgress }}%</span>
              <span
                v-if="upgradeElapsedDisplay"
                class="text-xs text-muted-foreground"
              >
                ({{ upgradeElapsedDisplay }})
              </span>
            </div>
            <Button
              size="sm"
              variant="outline"
              class="h-6 text-xs"
              @click="showUpgradeDetails = !showUpgradeDetails"
            >
              <ChevronDown v-if="!showUpgradeDetails" class="size-3" />
              <ChevronUp v-else class="size-3" />
              {{ t('settings.openclawRuntime.upgradeDetails') }}
            </Button>
          </div>

          <!-- 进度条本体 -->
          <div class="px-4 pb-1">
            <div class="h-2 overflow-hidden rounded-full bg-muted">
              <div
                class="h-full bg-primary transition-all duration-300"
                :style="{ width: upgradeProgress + '%' }"
              />
            </div>
          </div>

          <p class="px-4 pb-2 text-xs text-muted-foreground">{{ status.message }}</p>

          <!-- 展开：命令输出详情 -->
          <div
            v-if="showUpgradeDetails"
            class="px-4 pb-3"
          >
            <div
              ref="upgradeOutputEl"
              class="max-h-48 overflow-y-auto rounded border border-border bg-muted/50 px-3 py-2 font-mono text-xs text-muted-foreground dark:border-white/10"
            >
              <div
                v-for="(line, idx) in upgradeOutputLines"
                :key="idx"
                class="leading-5"
              >
                {{ line }}
              </div>
              <div
                v-if="!upgradeOutputLines.length"
                class="italic text-muted-foreground/50"
              >
                {{ t('settings.openclawRuntime.upgradeOutputWaiting') }}
              </div>
            </div>
          </div>

          <!-- 取消升级按钮 -->
          <div class="flex justify-end border-t border-border px-4 py-2 dark:border-white/10">
            <Button
              size="sm"
              variant="outline"
              class="text-xs"
              @click="handleCancelUpgrade"
            >
              <Square class="mr-1.5 size-3" />
              {{ t('settings.openclawRuntime.cancelUpgrade') }}
            </Button>
          </div>
        </div>

        <!-- Bottom actions: upgrade + open console (Figma) -->
        <div
          class="flex flex-col gap-3 border-t border-border p-4 sm:flex-row dark:border-white/10"
        >
          <Button
            class="min-h-10 flex-1"
            variant="outline"
            :disabled="
              isActive || upgrading || restarting || !status.installedVersion || isTransitioning
            "
            :title="isActive ? t('settings.openclawRuntime.upgradeButtonDisabledWhenActive') : undefined"
            @click="handleUpgrade"
          >
            <Download v-if="!upgrading" class="mr-1.5 size-3.5" />
            <Loader2 v-else class="mr-1.5 size-3.5 animate-spin" />
            {{
              upgrading
                ? t('settings.openclawRuntime.upgrading')
                : t('settings.openclawRuntime.upgradeButton')
            }}
          </Button>
          <Button
            class="min-h-10 flex-1"
            variant="outline"
            :disabled="!isActive"
            @click="handleOpenDashboard"
          >
            <ExternalLink class="mr-1.5 size-3.5" />
            {{ t('settings.openclawRuntime.openDashboard') }}
          </Button>
        </div>

        <!-- 重置到出厂设置按钮 -->
        <div
          class="flex items-center justify-between border-t border-border p-4 dark:border-white/10"
        >
          <div class="flex min-w-0 flex-1 flex-col gap-0.5 pr-4">
            <span class="shrink-0 text-sm font-medium text-foreground">
              {{ t('settings.openclawRuntime.resetToFactory') }}
            </span>
            <span class="shrink-0 text-xs text-muted-foreground">
              {{ t('settings.openclawRuntime.resetToFactoryHint') }}
            </span>
          </div>
          <Button
            size="sm"
            variant="outline"
            :disabled="!isGatewayStopped || resetting"
            @click="resetConfirmOpen = true"
          >
            <Loader2 v-if="resetting" class="mr-1.5 size-3.5 animate-spin" />
            <RotateCcw v-else class="mr-1.5 size-3.5" />
            {{ resetting ? resetElapsedDisplay : t('settings.openclawRuntime.resetToFactory') }}
          </Button>
        </div>
      </SettingsCard>

      <!-- 继续/重新升级弹窗 -->
      <div
        v-if="showContinueRestartDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
        @click.self="showContinueRestartDialog = false"
      >
        <div class="mx-4 w-full max-w-sm rounded-lg border border-border bg-popover p-5 shadow-lg dark:border-white/10 dark:ring-1 dark:ring-white/10">
          <h3 class="mb-1 text-base font-semibold text-foreground">
            {{ t('settings.openclawRuntime.continueOrRestartTitle') }}
          </h3>
          <p class="mb-5 text-sm text-muted-foreground">
            {{
              t('settings.openclawRuntime.continueOrRestartDesc', {
                version: existingUpgradeResult?.existingVersion || '',
              })
            }}
          </p>
          <div class="flex flex-col gap-2 sm:flex-row sm:justify-end">
            <Button
              variant="outline"
              size="sm"
              @click="showContinueRestartDialog = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              variant="outline"
              size="sm"
              @click="handleRestartUpgrade"
            >
              {{ t('settings.openclawRuntime.restartUpgrade') }}
            </Button>
            <Button
              size="sm"
              @click="handleContinueUpgrade"
            >
              {{ t('settings.openclawRuntime.continueUpgrade') }}
            </Button>
          </div>
        </div>
      </div>

      <!-- 重置到出厂设置确认弹窗 -->
      <AlertDialog :open="resetConfirmOpen" @update:open="(v) => !v && cancelReset()">
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{{ t('settings.openclawRuntime.resetConfirmTitle') }}</AlertDialogTitle>
            <AlertDialogDescription>
              {{ t('settings.openclawRuntime.resetConfirmDesc') }}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
            <AlertDialogAction @click="handleResetToFactory">
              {{ t('common.confirm') }}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <OpenClawDoctorConsole />
    </div>
  </div>
</template>
