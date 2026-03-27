import { Events } from '@wailsio/runtime'
import { useAppStore, type SystemOwner } from '@/stores'

/**
 * Sync currentSystem changes broadcast from the main window (or other windows).
 * Call once in each secondary window's App.vue setup (e.g. snap window).
 * Returns an unsubscribe function.
 */
export function useSystemSync() {
  const appStore = useAppStore()

  const unsubscribe = Events.On('system:changed', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const newSystem = (payload?.system || '').trim() as SystemOwner

    if (newSystem !== 'chatclaw' && newSystem !== 'openclaw') return

    if (appStore.currentSystem === newSystem) return

    // Update local state without rebroadcasting.
    appStore.currentSystem = newSystem
    localStorage.setItem('currentSystem', newSystem)
  })

  return unsubscribe
}
