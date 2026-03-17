type BindingStatePayload = boolean | { chatwiki_version?: string | null } | null

type BindingStateListener = (payload: BindingStatePayload) => void

const listeners = new Set<BindingStateListener>()

export function notifyChatwikiBindingChanged(payload: BindingStatePayload): void {
  for (const listener of listeners) {
    listener(payload)
  }
}

export function onChatwikiBindingChanged(listener: BindingStateListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
