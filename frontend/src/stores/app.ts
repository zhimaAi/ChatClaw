import { ref, watch } from 'vue'
import { defineStore } from 'pinia'
import { SettingsService } from '@bindings/willclaw/internal/services/settings'

export type Theme = 'light' | 'dark' | 'system'

export const useAppStore = defineStore('app', () => {
  // 主题设置
  const theme = ref<Theme>('system')

  // Whether a new version is available (used to show badge on "Check for Update" button)
  const hasAvailableUpdate = ref(false)

  // 获取系统主题
  const getSystemTheme = (): 'light' | 'dark' => {
    if (typeof window !== 'undefined') {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    }
    return 'light'
  }

  // 应用主题到 DOM
  const applyTheme = (newTheme: Theme) => {
    const root = document.documentElement
    const effectiveTheme = newTheme === 'system' ? getSystemTheme() : newTheme

    if (effectiveTheme === 'dark') {
      root.classList.add('dark')
    } else {
      root.classList.remove('dark')
    }
  }

  // 设置主题
  const setTheme = (newTheme: Theme) => {
    theme.value = newTheme
    localStorage.setItem('theme', newTheme)
    applyTheme(newTheme)
    // Persist to DB (fire-and-forget)
    void SettingsService.SetValue('theme', newTheme).catch((e: unknown) => {
      console.warn('Failed to persist theme to backend settings:', e)
    })
  }

  // 初始化主题
  const initTheme = () => {
    const savedTheme = localStorage.getItem('theme') as Theme | null
    if (savedTheme && ['light', 'dark', 'system'].includes(savedTheme)) {
      theme.value = savedTheme
    }
    applyTheme(theme.value)

    // Prefer DB value (async) so theme persists across machines/windows
    void SettingsService.Get('theme')
      .then((s) => {
        const v = (s?.value || '').trim() as Theme
        if (v && ['light', 'dark', 'system'].includes(v) && v !== theme.value) {
          theme.value = v
          localStorage.setItem('theme', v)
          applyTheme(v)
        }
      })
      .catch((e: unknown) => {
        console.warn('Failed to fetch theme from backend settings:', e)
      })

    // 监听系统主题变化
    if (typeof window !== 'undefined') {
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
        if (theme.value === 'system') {
          applyTheme('system')
        }
      })
    }
  }

  // 监听主题变化
  watch(theme, (newTheme) => {
    applyTheme(newTheme)
  })

  return {
    theme,
    setTheme,
    initTheme,
    getSystemTheme,
    hasAvailableUpdate,
  }
})
