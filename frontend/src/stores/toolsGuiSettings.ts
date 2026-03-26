import { ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { Events, Window } from '@wailsio/runtime'
import { SettingsService, Category } from '@bindings/chatclaw/internal/services/settings'
import { TextSelectionService } from '@bindings/chatclaw/internal/services/textselection'
import { FloatingBallService } from '@bindings/chatclaw/internal/services/floatingball'

/**
 * Shared GUI toggles from Settings → 功能工具 (CategoryTools):
 * - enable_selection_search (滑词搜索)
 * - show_floating_window (显示悬浮窗)
 *
 * Used by ToolsSettings.vue and ToolsPage.vue so both stay in sync when tabs stay mounted (v-show).
 */
export const useToolsGuiSettingsStore = defineStore('toolsGuiSettings', () => {
  const enableSelectionSearch = ref(false)
  const showFloatingWindow = ref(false)

  let floatingWindowDesired = false
  let floatingWindowApplied = false
  let suppressFloatingWindowSync = true
  let floatingWindowSyncing = false
  let floatingWindowPending = false
  let floatingWindowDebounceTimer: ReturnType<typeof setTimeout> | null = null
  const floatingClickCount = ref(0)

  const debugFloatingToggle = () => {
    try {
      return localStorage.getItem('debugFloatingToggle') === '1'
    } catch {
      return false
    }
  }

  const logFloating = (...args: unknown[]) => {
    if (!debugFloatingToggle()) return
    console.log('[floating-toggle]', new Date().toISOString(), ...args)
  }

  const updateSetting = async (key: string, value: string) => {
    await SettingsService.SetValue(key, value)
  }

  const syncFloatingWindowDesired = async () => {
    if (floatingWindowSyncing) {
      floatingWindowPending = true
      logFloating('sync:already_in_flight -> pending', {
        desired: floatingWindowDesired,
        applied: floatingWindowApplied,
        ui: showFloatingWindow.value,
      })
      return
    }
    floatingWindowSyncing = true
    floatingWindowPending = false
    const target = floatingWindowDesired
    logFloating('sync:start', {
      target,
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
    try {
      await updateSetting('show_floating_window', String(target))
      try {
        await FloatingBallService.SetVisible(target)
      } catch (error) {
        console.error('Failed to update floating ball visibility:', error)
      }
      try {
        await Window.Focus()
      } catch (e) {
        console.error('Failed to refocus window after floating toggle:', e)
      }
      floatingWindowApplied = target
      logFloating('sync:success', { applied: floatingWindowApplied })
    } catch (error) {
      console.error('Failed to update show_floating_window setting:', error)
      floatingWindowDesired = floatingWindowApplied
      showFloatingWindow.value = floatingWindowApplied
      try {
        await FloatingBallService.SetVisible(floatingWindowApplied)
      } catch (e) {
        console.error('Failed to rollback floating ball visibility:', e)
      }
      try {
        await Window.Focus()
      } catch (e) {
        console.error('Failed to refocus window after rollback:', e)
      }
      logFloating('sync:failed -> rollback', {
        desired: floatingWindowDesired,
        applied: floatingWindowApplied,
        ui: showFloatingWindow.value,
      })
    } finally {
      floatingWindowSyncing = false
      if (floatingWindowPending && floatingWindowDesired !== floatingWindowApplied) {
        logFloating('sync:pending_flush', {
          desired: floatingWindowDesired,
          applied: floatingWindowApplied,
          ui: showFloatingWindow.value,
        })
        void syncFloatingWindowDesired()
      } else {
        logFloating('sync:end', {
          desired: floatingWindowDesired,
          applied: floatingWindowApplied,
          ui: showFloatingWindow.value,
        })
      }
    }
  }

  const scheduleFloatingWindowSync = (val: boolean) => {
    floatingWindowDesired = val
    if (floatingWindowDebounceTimer) {
      clearTimeout(floatingWindowDebounceTimer)
    }
    logFloating('schedule', {
      val,
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      ui: showFloatingWindow.value,
    })
    floatingWindowDebounceTimer = setTimeout(() => {
      floatingWindowDebounceTimer = null
      logFloating('schedule:fire', {
        desired: floatingWindowDesired,
        applied: floatingWindowApplied,
        ui: showFloatingWindow.value,
      })
      void syncFloatingWindowDesired()
    }, 160)
  }

  watch(
    showFloatingWindow,
    (val) => {
      if (suppressFloatingWindowSync) return
      logFloating('watch:model', {
        val,
        desired: floatingWindowDesired,
        applied: floatingWindowApplied,
        ui: showFloatingWindow.value,
      })
      scheduleFloatingWindowSync(val)
    },
    { flush: 'sync' }
  )

  /**
   * Apply keys from CategoryTools list (also used by ToolsSettings when loading tray keys).
   */
  function applyFromSettingsList(settings: Array<{ key: string; value: string }>) {
    for (const s of settings) {
      if (s.key === 'enable_selection_search') {
        enableSelectionSearch.value = s.value === 'true'
      }
      if (s.key === 'show_floating_window') {
        showFloatingWindow.value = s.value === 'true'
      }
    }
  }

  /**
   * After DB values applied: align floating ball and enable floating watch.
   */
  async function initFloatingBallAfterLoad() {
    try {
      const currentVisible = await FloatingBallService.IsVisible()
      if (currentVisible !== showFloatingWindow.value) {
        await FloatingBallService.SetVisible(showFloatingWindow.value)
      }
      floatingWindowDesired = showFloatingWindow.value
      floatingWindowApplied = showFloatingWindow.value
      suppressFloatingWindowSync = false
    } catch (error) {
      console.error('Failed to sync floating ball visibility on load:', error)
      suppressFloatingWindowSync = false
    }
  }

  async function reloadFromBackend() {
    const settings = await SettingsService.List(Category.CategoryTools)
    applyFromSettingsList(settings)
    await initFloatingBallAfterLoad()
  }

  async function setSelectionSearch(val: boolean) {
    const prev = enableSelectionSearch.value
    enableSelectionSearch.value = val
    try {
      await updateSetting('enable_selection_search', String(val))
      Events.Emit('settings:selection-search-changed', { enabled: val })
      await TextSelectionService.SyncFromSettings()
    } catch {
      enableSelectionSearch.value = prev
    }
  }

  function handleFloatingWindowClickCapture() {
    floatingClickCount.value += 1
    logFloating('click:capture', {
      n: floatingClickCount.value,
      ui_before: showFloatingWindow.value,
      desired: floatingWindowDesired,
      applied: floatingWindowApplied,
      syncing: floatingWindowSyncing,
    })
  }

  Events.On('settings:selection-search-changed', (event: unknown) => {
    const ev = event as { data?: unknown }
    const payload = Array.isArray(ev?.data) ? ev.data[0] : (ev?.data ?? ev)
    const enabled = (payload as { enabled?: boolean } | undefined)?.enabled
    enableSelectionSearch.value = !!enabled
  })

  return {
    enableSelectionSearch,
    showFloatingWindow,
    floatingClickCount,
    applyFromSettingsList,
    initFloatingBallAfterLoad,
    reloadFromBackend,
    setSelectionSearch,
    handleFloatingWindowClickCapture,
  }
})
