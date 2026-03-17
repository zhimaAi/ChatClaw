type BindingStateListener = (bound: boolean) => void

const listeners = new Set<BindingStateListener>()

export function notifyChatwikiBindingChanged(bound: boolean): void {
  for (const listener of listeners) {
    listener(bound)
  }
}

export function onChatwikiBindingChanged(listener: BindingStateListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
