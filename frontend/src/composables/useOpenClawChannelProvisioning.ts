import { ref } from 'vue'
import type { Channel } from '@bindings/chatclaw/internal/services/channels'

/** OpenClaw backend may still be syncing gateway / config after CreateChannel returns. */
const pendingGatewayIds = ref<Set<number>>(new Set())
/** Auto-generate assistant flow: CreateAgent + bind may take a long time. */
const pendingAgentIds = ref<Set<number>>(new Set())

function replaceSet(target: typeof pendingGatewayIds, next: Set<number>) {
  target.value = next
}

export function addPendingGatewayProvisioning(channelId: number) {
  const next = new Set(pendingGatewayIds.value)
  next.add(channelId)
  replaceSet(pendingGatewayIds, next)
}

export function addPendingAgentProvisioning(channelId: number) {
  const next = new Set(pendingAgentIds.value)
  next.add(channelId)
  replaceSet(pendingAgentIds, next)
}

export function removePendingGatewayProvisioning(channelId: number) {
  const next = new Set(pendingGatewayIds.value)
  next.delete(channelId)
  replaceSet(pendingGatewayIds, next)
}

export function removePendingAgentProvisioning(channelId: number) {
  const next = new Set(pendingAgentIds.value)
  next.delete(channelId)
  replaceSet(pendingAgentIds, next)
}

/** Call after loading channel list so completed gateways/agents clear local pending state. */
export function syncPendingFromChannels(channels: Channel[]) {
  const byId = new Map(channels.map((c) => [c.id, c]))

  const nextG = new Set(pendingGatewayIds.value)
  for (const id of pendingGatewayIds.value) {
    const ch = byId.get(id)
    if (!ch) {
      nextG.delete(id)
      continue
    }
    if (ch.status === 'online' || ch.status === 'error') {
      nextG.delete(id)
    }
  }
  replaceSet(pendingGatewayIds, nextG)

  const nextA = new Set(pendingAgentIds.value)
  for (const id of pendingAgentIds.value) {
    const ch = byId.get(id)
    if (!ch) {
      nextA.delete(id)
      continue
    }
    if (ch.agent_id !== 0) {
      nextA.delete(id)
    }
  }
  replaceSet(pendingAgentIds, nextA)
}

export function isGatewayProvisioning(ch: Channel): boolean {
  if (!pendingGatewayIds.value.has(ch.id)) return false
  return ch.status !== 'online' && ch.status !== 'error'
}

export function isAgentProvisioning(ch: Channel): boolean {
  if (!pendingAgentIds.value.has(ch.id)) return false
  return ch.agent_id === 0
}

/** Bind pill: show "creating" while gateway syncs (unbound) or assistant is being created. */
export function isBindProvisioning(ch: Channel): boolean {
  if (isAgentProvisioning(ch)) return true
  if (ch.agent_id !== 0) return false
  return isGatewayProvisioning(ch)
}
