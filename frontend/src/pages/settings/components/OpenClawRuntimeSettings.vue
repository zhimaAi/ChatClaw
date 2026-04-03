<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { RefreshCw, Loader2, ExternalLink, Download } from 'lucide-vue-next'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import {
  useNavigationStore,
  useOpenClawGatewayStore,
  gatewayBadgeClass,
  GatewayVisualStatus,
} from '@/stores'
import {
  RuntimeStatus,
  GatewayConnectionState,
} from '@bindings/chatclaw/internal/openclaw/runtime/models'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import SettingsCard from './SettingsCard.vue'
import OpenClawDoctorConsole from '@/components/openclaw/OpenClawDoctorConsole.vue'

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
const upgrading = ref(false)
/** True while handling gateway on/off from the title-bar switch */
const gatewaySwitchBusy = ref(false)

const isActive = computed(() => status.value.phase === 'connected')
const isTransitioning = computed(() => {
  const p = status.value.phase
  return p === 'starting' || p === 'connecting' || p === 'restarting' || p === 'upgrading'
})

/**
 * Same source as sidebar + status badge: Pinia store is refreshed by global heartbeat poll,
 * while local `status` is only updated on mount/events — using `status.phase` here caused OFF while badge showed running.
 */
const gatewaySwitchOn = computed(() => {
  if (gatewayStore.runtimePhase === 'upgrading') return false
  const v = gatewayStore.visualStatus
  return v === GatewayVisualStatus.Running || v === GatewayVisualStatus.Starting
})

// 是否处于升级阶段（显示进度条）
const isUpgrading = computed(() => status.value.phase === 'upgrading')

const gatewayConnectionLabel = computed(() => {
  if (gatewayState.value.authenticated) return t('settings.openclawRuntime.gateway.authenticated')
  if (gatewayState.value.connected) return t('settings.openclawRuntime.gateway.connected')
  if (gatewayState.value.reconnecting) return t('settings.openclawRuntime.gateway.reconnecting')
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

// 显示升级进度条：当处于 upgrading 阶段时显示
const showUpgradeProgress = computed(() => {
  return status.value.phase === 'upgrading'
})

const badgeText = computed(() => {
  const v = gatewayStore.visualStatus
  return t(`settings.openclawRuntime.statusBadge.${v}`)
})

const badgeClass = computed(() => gatewayBadgeClass[gatewayStore.visualStatus])

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
  syncGatewayStore()
}

/** Title switch: off → stop + disable autostart; on → enable autostart + start/restart gateway */
const handleGatewaySwitchToggle = async (checked: boolean) => {
  if (gatewaySwitchBusy.value) return
  gatewaySwitchBusy.value = true
  try {
    await OpenClawRuntimeService.SetAutoStart(checked)
    if (checked) {
      toast.success(t('settings.openclawRuntime.autoStartEnabled'))
    } else {
      toast.success(t('settings.openclawRuntime.autoStartDisabled'))
    }
    await loadStatus()
  } catch (e) {
    console.error('Failed to toggle gateway:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.autoStartFailed'))
  } finally {
    gatewaySwitchBusy.value = false
  }
}

const handleRestart = async () => {
  restarting.value = true
  try {
    status.value = await OpenClawRuntimeService.RestartGateway()
    syncGatewayStore()
    toast.success(t('settings.openclawRuntime.restartSuccess'))
  } catch (e) {
    console.error('Failed to restart OpenClaw gateway:', e)
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.restartFailed'))
  } finally {
    restarting.value = false
  }
}

const handleUpgrade = async () => {
  if (upgrading.value) return
  upgrading.value = true
  try {
    const result = await OpenClawRuntimeService.UpgradeRuntime()
    if (result?.upgraded) {
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
  }
}

const handleOpenDashboard = () => {
  navigationStore.navigateToModule('openclaw-dashboard')
}

let unsubscribeStatus: (() => void) | null = null
let unsubscribeGateway: (() => void) | null = null

watch([status, gatewayState], () => syncGatewayStore(), { deep: true })

onMounted(() => {
  void loadStatus()
  void gatewayStore.poll()

  unsubscribeStatus = Events.On('openclaw:status', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data) status.value = RuntimeStatus.createFrom(data)
  })

  unsubscribeGateway = Events.On('openclaw:gateway-state', (event: any) => {
    const data = event?.data?.[0] ?? event?.data ?? event
    if (data) gatewayState.value = GatewayConnectionState.createFrom(data)
  })
})

onUnmounted(() => {
  unsubscribeStatus?.()
  unsubscribeStatus = null
  unsubscribeGateway?.()
  unsubscribeGateway = null
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
    <SettingsCard :title="t('settings.openclawRuntime.title')" fullWidth>
      <template #header-right>
        <Switch
          :model-value="gatewaySwitchOn"
          :disabled="gatewaySwitchBusy || gatewayStore.runtimePhase === 'upgrading'"
          :aria-label="t('settings.openclawRuntime.autoStartLabel')"
          @update:model-value="handleGatewaySwitchToggle"
        />
      </template>

      <!-- Gateway status row: badge + restart -->
      <div
        class="flex flex-wrap items-center justify-between gap-3 border-b border-border p-4 dark:border-white/10"
      >
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <span class="shrink-0 text-sm text-foreground">
            {{ t('settings.openclawRuntime.gatewayStatusLabel') }}
          </span>
          <Loader2 v-if="isTransitioning" class="size-3.5 shrink-0 animate-spin text-muted-foreground" />
          <span v-else :class="badgeClass">
            {{ badgeText }}
          </span>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <Button
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

      <!-- Gateway connection -->
      <div
        class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
      >
        <span class="shrink-0 text-sm text-foreground">
          {{ t('settings.openclawRuntime.gatewayConnection') }}
        </span>
        <span class="text-sm text-muted-foreground">{{ gatewayConnectionLabel }}</span>
      </div>

      <!-- Gateway URL -->
      <div
        class="flex items-center justify-between border-b border-border p-4 dark:border-white/10"
      >
        <span class="shrink-0 text-sm text-foreground">
          {{ t('settings.openclawRuntime.gatewayEndpoint') }}
        </span>
        <span class="break-all font-mono text-sm text-muted-foreground">{{ displayGatewayURL }}</span>
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

      <div
        v-if="showUpgradeProgress"
        class="border-t border-border px-4 py-3 dark:border-white/10"
      >
        <div class="mb-2 flex items-center justify-between">
          <span class="text-xs font-medium text-foreground">
            {{ t('settings.openclawRuntime.upgradeProgress') }}
          </span>
          <span class="text-xs text-muted-foreground">{{ upgradeProgress }}%</span>
        </div>
        <div class="h-2 overflow-hidden rounded-full bg-muted">
          <div
            class="h-full bg-primary transition-all duration-300"
            :style="{ width: upgradeProgress + '%' }"
          />
        </div>
        <p class="mt-2 text-xs text-muted-foreground">{{ status.message }}</p>
      </div>

      <!-- Bottom actions: upgrade + open console (Figma) -->
      <div class="flex flex-col gap-3 border-t border-border p-4 sm:flex-row dark:border-white/10">
        <Button
          class="min-h-10 flex-1"
          variant="outline"
          :disabled="
            upgrading ||
              restarting ||
              !status.installedVersion ||
              status.phase === 'upgrading'
          "
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
    </SettingsCard>

    <OpenClawDoctorConsole />
    </div>
  </div>
</template>
