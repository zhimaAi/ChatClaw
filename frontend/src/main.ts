import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { initI18n } from './i18n'
import { fetchLocale } from './composables/useLocale'
import { useAppStore } from './stores'
import { AppService } from '@bindings/willchat/internal/services/app'
import './assets/index.css'

async function bootstrap() {
  // 从后端获取语言配置，初始化 i18n
  const locale = await fetchLocale()
  const i18n = initI18n(locale)

  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(i18n)

  // 初始化主题
  const appStore = useAppStore()
  appStore.initTheme()

  app.mount('#app')

  // Show main window after Vue app is mounted (Windows: avoid black screen flash)
  // Safe to call on all platforms; backend handles platform-specific logic.
  AppService.ShowMainWindow().catch((err) => {
    console.error('Failed to show main window:', err)
  })
}

bootstrap()
