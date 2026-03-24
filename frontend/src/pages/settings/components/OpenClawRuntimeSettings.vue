<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { RefreshCw, Loader2, Circle, ExternalLink } from 'lucide-vue-next'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import { useNavigationStore } from '@/stores'
import {
  RuntimeStatus,
  GatewayConnectionState,
} from '@bindings/chatclaw/internal/openclaw/runtime/models'
import SettingsCard from './SettingsCard.vue'
import SettingsItem from './SettingsItem.vue'

const { t } = useI18n()
const navigationStore = useNavigationStore()

const status = ref<RuntimeStatus>(new RuntimeStatus({ phase: 'idle' }))
const gatewayState = ref<GatewayConnectionState>(new GatewayConnectionState())
const restarting = ref(false)

const phaseLabel = computed(() => {
  const phase = status.value.phase || 'idle'
  return t(`settings.openclawRuntime.phase.${phase}`)
})

const isActive = computed(() => status.value.phase === 'connected')
const isTransitioning = computed(() => {
  const p = status.value.phase
  return p === 'starting' || p === 'connecting' || p === 'restarting'
})

const gatewayConnectionLabel = computed(() => {
  if (gatewayState.value.authenticated) return t('settings.openclawRuntime.gateway.authenticated')
  if (gatewayState.value.connected) return t('settings.openclawRuntime.gateway.connected')
  if (gatewayState.value.reconnecting) return t('settings.openclawRuntime.gateway.reconnecting')
  return t('settings.openclawRuntime.gateway.disconnected')
})

const gatewayConnected = computed(
  () => gatewayState.value.connected || gatewayState.value.authenticated
)

const displayVersion = computed(() => {
  return status.value.installedVersion || t('settings.openclawRuntime.notInstalled')
})

const displayGatewayURL = computed(() => {
  return status.value.gatewayURL || 'http://127.0.0.1'
})

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
}

const handleRestart = async () => {
  restarting.value = true
  try {
    status.value = await OpenClawRuntimeService.RestartGateway()
  } catch (e) {
    console.error('Failed to restart OpenClaw gateway:', e)
  } finally {
    restarting.value = false
  }
}

const handleOpenDashboard = () => {
  navigationStore.navigateToModule('openclaw-dashboard')
}

let unsubscribeStatus: (() => void) | null = null
let unsubscribeGateway: (() => void) | null = null

onMounted(() => {
  void loadStatus()

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
  <div class="flex flex-col gap-4">
    <SettingsCard :title="t('settings.openclawRuntime.title')">
      <!-- Runtime Status -->
      <SettingsItem :label="t('settings.openclawRuntime.runtimeStatus')">
        <div class="flex items-center gap-2">
          <Loader2 v-if="isTransitioning" class="size-3.5 animate-spin text-muted-foreground" />
          <Circle
            v-else
            class="size-2"
            :class="isActive ? 'fill-foreground text-foreground' : 'fill-muted-foreground/40 text-muted-foreground/40'"
          />
          <span class="text-sm" :class="isActive ? 'text-foreground' : 'text-muted-foreground'">
            {{ phaseLabel }}
          </span>
        </div>
      </SettingsItem>

      <!-- Gateway Connection -->
      <SettingsItem :label="t('settings.openclawRuntime.gatewayConnection')">
        <div class="flex items-center gap-2">
          <Loader2
            v-if="gatewayState.reconnecting"
            class="size-3.5 animate-spin text-muted-foreground"
          />
          <Circle
            v-else
            class="size-2"
            :class="gatewayConnected ? 'fill-foreground text-foreground' : 'fill-muted-foreground/40 text-muted-foreground/40'"
          />
          <span
            class="text-sm"
            :class="gatewayConnected ? 'text-foreground' : 'text-muted-foreground'"
          >
            {{ gatewayConnectionLabel }}
          </span>
        </div>
      </SettingsItem>

      <!-- Gateway Endpoint -->
      <SettingsItem :label="t('settings.openclawRuntime.gatewayEndpoint')">
        <span class="font-mono text-sm text-muted-foreground">{{ displayGatewayURL }}</span>
      </SettingsItem>

      <!-- Version -->
      <SettingsItem :label="t('settings.openclawRuntime.version')">
        <span class="text-sm text-muted-foreground">{{ displayVersion }}</span>
      </SettingsItem>

      <!-- Error Message -->
      <div
        v-if="status.message && status.phase === 'error'"
        class="border-t border-border px-4 py-3 dark:border-white/10"
      >
        <p class="text-xs text-muted-foreground">{{ status.message }}</p>
      </div>

      <!-- Actions -->
      <div
        class="flex items-center justify-end gap-2 border-t border-border px-4 py-3 dark:border-white/10"
      >
        <Button size="sm" variant="outline" :disabled="restarting" @click="handleRestart">
          <RefreshCw v-if="!restarting" class="mr-1.5 size-3.5" />
          <Loader2 v-else class="mr-1.5 size-3.5 animate-spin" />
          {{
            restarting
              ? t('settings.openclawRuntime.restarting')
              : t('settings.openclawRuntime.restartButton')
          }}
        </Button>
        <Button size="sm" variant="outline" :disabled="!isActive" @click="handleOpenDashboard">
          <ExternalLink class="mr-1.5 size-3.5" />
          {{ t('settings.openclawRuntime.openDashboard') }}
        </Button>
      </div>
    </SettingsCard>
  </div>
</template>
