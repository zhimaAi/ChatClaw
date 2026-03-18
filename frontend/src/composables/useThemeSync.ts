import { Events } from '@wailsio/runtime'
import { useAppStore, type Theme } from '@/stores'

/**
 * Sync theme changes broadcast from the main window (or other windows).
 * Call once in each secondary window's App.vue setup (e.g. snap window).
 * Returns an unsubscribe function.
 */
export function useThemeSync() {
  const appStore = useAppStore()

  const unsubscribe = Events.On('theme:changed', (event: any) => {
    const payload = Array.isArray(event?.data) ? event.data[0] : (event?.data ?? event)
    const newTheme = (payload?.theme || '').trim() as Theme

    if (newTheme !== 'light' && newTheme !== 'dark' && newTheme !== 'system') return

    // Avoid feedback loops: if theme is already in desired state, skip.
    if (appStore.theme === newTheme) return

    // Apply theme locally without rebroadcasting.
    appStore.setTheme(newTheme, { broadcast: false })
  })

  return unsubscribe
}
