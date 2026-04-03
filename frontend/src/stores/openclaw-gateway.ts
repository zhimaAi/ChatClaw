import { ref } from 'vue'
import { defineStore } from 'pinia'
import { Events } from '@wailsio/runtime'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import {
  RuntimeStatus,
  GatewayConnectionState,
} from '@bindings/chatclaw/internal/openclaw/runtime/models'

/**
 * Gateway status badge aligned with Figma (running / error / stop / starting).
 * Shared store so other modules can read the same visual state (heartbeat + events).
 */
export enum GatewayVisualStatus {
  Running = 'running',
  Error = 'error',
  Stop = 'stop',
  Starting = 'starting',
  /** Runtime bundle download/install; distinct from gateway process starting. */
  Upgrading = 'upgrading',
}

export const gatewayBadgeClass: Record<GatewayVisualStatus, string> = {
  [GatewayVisualStatus.Running]:
    'inline-flex items-center rounded-md border border-emerald-300 px-1.5 py-0.5 text-sm text-emerald-700 dark:border-emerald-600/50 dark:text-emerald-400',
  [GatewayVisualStatus.Error]:
    'inline-flex items-center rounded-md border border-rose-300 px-1.5 py-0.5 text-sm text-rose-600 dark:border-rose-600/50 dark:text-rose-400',
  [GatewayVisualStatus.Stop]:
    'inline-flex items-center rounded-md border border-neutral-300 px-1.5 py-0.5 text-sm text-neutral-600 dark:border-white/20 dark:text-muted-foreground',
  [GatewayVisualStatus.Starting]:
    'inline-flex items-center rounded-md border border-amber-300 px-1.5 py-0.5 text-sm text-amber-700 dark:border-amber-600/50 dark:text-amber-400',
  [GatewayVisualStatus.Upgrading]:
    'inline-flex items-center rounded-md border border-amber-300 px-1.5 py-0.5 text-sm text-amber-700 dark:border-amber-600/50 dark:text-amber-400',
}

/** Sidebar pill: border + background match state (Figma 1902-51631 / 34 / 39 / 41). */
export const gatewaySidebarTagShellClass: Record<GatewayVisualStatus, string> = {
  [GatewayVisualStatus.Running]:
    'border-emerald-300 bg-emerald-50/95 dark:border-emerald-600/50 dark:bg-emerald-950/45 dark:ring-1 dark:ring-emerald-500/20',
  [GatewayVisualStatus.Error]:
    'border-rose-300 bg-rose-50/95 dark:border-rose-600/50 dark:bg-rose-950/45 dark:ring-1 dark:ring-rose-500/20',
  [GatewayVisualStatus.Stop]:
    'border-neutral-300 bg-neutral-100/90 dark:border-white/20 dark:bg-neutral-900/55 dark:ring-1 dark:ring-white/10',
  [GatewayVisualStatus.Starting]:
    'border-amber-300 bg-amber-50/95 dark:border-amber-600/50 dark:bg-amber-950/45 dark:ring-1 dark:ring-amber-500/25',
  [GatewayVisualStatus.Upgrading]:
    'border-amber-300 bg-amber-50/95 dark:border-amber-600/50 dark:bg-amber-950/45 dark:ring-1 dark:ring-amber-500/25',
}

/** Prefix + separator text (same hue family as border). */
export const gatewaySidebarTagLabelClass: Record<GatewayVisualStatus, string> = {
  [GatewayVisualStatus.Running]:
    'text-emerald-900/90 dark:text-emerald-200/90',
  [GatewayVisualStatus.Error]: 'text-rose-950/90 dark:text-rose-200/88',
  [GatewayVisualStatus.Stop]: 'text-neutral-700 dark:text-neutral-400',
  [GatewayVisualStatus.Starting]:
    'text-amber-950/90 dark:text-amber-200/88',
  [GatewayVisualStatus.Upgrading]:
    'text-amber-950/90 dark:text-amber-200/88',
}

/** Status word after colon (weight comes from parent font-bold). */
export const gatewaySidebarTagStatusClass: Record<GatewayVisualStatus, string> = {
  [GatewayVisualStatus.Running]:
    'text-emerald-700 dark:text-emerald-400',
  [GatewayVisualStatus.Error]: 'text-rose-600 dark:text-rose-400',
  [GatewayVisualStatus.Stop]: 'text-neutral-600 dark:text-neutral-300',
  [GatewayVisualStatus.Starting]: 'text-amber-700 dark:text-amber-400',
  [GatewayVisualStatus.Upgrading]: 'text-amber-700 dark:text-amber-400',
}

/** Spinner icon on starting state. */
export const gatewaySidebarTagLoaderClass: Record<GatewayVisualStatus, string> = {
  [GatewayVisualStatus.Running]:
    'text-emerald-600 dark:text-emerald-400',
  [GatewayVisualStatus.Error]: 'text-rose-600 dark:text-rose-400',
  [GatewayVisualStatus.Stop]: 'text-neutral-500 dark:text-neutral-400',
  [GatewayVisualStatus.Starting]:
    'text-amber-600 dark:text-amber-400',
  [GatewayVisualStatus.Upgrading]:
    'text-amber-600 dark:text-amber-400',
}

/** True while the runtime bundle is being downloaded/installed (OSS install or upgrade). */
export function isOpenClawRuntimeMutatingPhase(phase: string | undefined): boolean {
  return phase === 'upgrading'
}

function mapToVisual(
  status: RuntimeStatus,
  _gw: GatewayConnectionState
): GatewayVisualStatus {
  const phase = status.phase || 'idle'
  if (phase === 'error') return GatewayVisualStatus.Error
  if (phase === 'upgrading') return GatewayVisualStatus.Upgrading
  if (phase === 'starting' || phase === 'connecting' || phase === 'restarting') {
    return GatewayVisualStatus.Starting
  }
  if (phase === 'connected') return GatewayVisualStatus.Running
  return GatewayVisualStatus.Stop
}

export const useOpenClawGatewayStore = defineStore('openclawGateway', () => {
  const visualStatus = ref<GatewayVisualStatus>(GatewayVisualStatus.Stop)
  /** Mirrors backend RuntimeStatus.phase (e.g. idle, connected, upgrading). */
  const runtimePhase = ref<string>('idle')
  const lastGatewayState = ref<GatewayConnectionState>(new GatewayConnectionState())
  let heartbeatId: ReturnType<typeof setInterval> | null = null
  /** Tracks whether global Wails event listeners have been registered. */
  let statusUnsubscribe: (() => void) | null = null
  let gatewayUnsubscribe: (() => void) | null = null

  function applySnapshot(status: RuntimeStatus, gw: GatewayConnectionState) {
    lastGatewayState.value = gw
    runtimePhase.value = status.phase || 'idle'
    visualStatus.value = mapToVisual(status, gw)
  }

  /**
   * Apply status-only updates (e.g. openclaw:status) so UI stays in sync when
   * OpenClawRuntimeSettings is not mounted. Keeps last gateway state for badge mapping.
   */
  function ingestRuntimeStatus(status: RuntimeStatus) {
    runtimePhase.value = status.phase || 'idle'
    visualStatus.value = mapToVisual(status, lastGatewayState.value)
  }

  /**
   * Apply gateway connection state updates (e.g. openclaw:gateway-state).
   * Combines with last known runtime status to update visual state.
   */
  function ingestGatewayState(gw: GatewayConnectionState) {
    lastGatewayState.value = gw
  }

  /**
   * Subscribe to global Wails events for backend status updates.
   * Called once on first heartbeat start; idempotent.
   */
  function subscribeToEvents() {
    if (statusUnsubscribe) return // Already subscribed

    statusUnsubscribe = Events.On('openclaw:status', (event: unknown) => {
      const data = (event as any)?.data?.[0] ?? (event as any)?.data ?? event
      if (data) {
        ingestRuntimeStatus(RuntimeStatus.createFrom(data))
      }
    })

    gatewayUnsubscribe = Events.On('openclaw:gateway-state', (event: unknown) => {
      const data = (event as any)?.data?.[0] ?? (event as any)?.data ?? event
      if (data) {
        ingestGatewayState(GatewayConnectionState.createFrom(data))
      }
    })
  }

  /**
   * Unsubscribe from Wails events to prevent duplicate handlers on re-subscribe.
   */
  function unsubscribeFromEvents() {
    statusUnsubscribe?.()
    statusUnsubscribe = null
    gatewayUnsubscribe?.()
    gatewayUnsubscribe = null
  }

  async function poll() {
    try {
      const s = await OpenClawRuntimeService.GetStatus()
      const g = await OpenClawRuntimeService.GetGatewayState()
      applySnapshot(RuntimeStatus.createFrom(s), GatewayConnectionState.createFrom(g))
    } catch {
      /* ignore */
    }
  }

  function startHeartbeat() {
    if (heartbeatId != null) return
    subscribeToEvents()
    heartbeatId = setInterval(() => {
      void poll()
    }, 5000)
  }

  function stopHeartbeat() {
    if (heartbeatId != null) {
      clearInterval(heartbeatId)
      heartbeatId = null
    }
    // Keep event subscriptions active — they are lightweight and allow
    // the store to stay in sync even when the heartbeat interval stops.
    // Duplicate subscribeToEvents() calls are prevented by the null check.
  }

  return {
    visualStatus,
    runtimePhase,
    lastGatewayState,
    applySnapshot,
    ingestRuntimeStatus,
    ingestGatewayState,
    poll,
    startHeartbeat,
    stopHeartbeat,
  }
})
