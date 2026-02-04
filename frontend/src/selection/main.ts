import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { initI18n } from '../i18n'
import { fetchLocale } from '../composables/useLocale'
import { useAppStore } from '../stores'
import '../assets/index.css'

async function bootstrap() {
  // Init i18n with backend locale (same as main window)
  const locale = await fetchLocale()
  const i18n = initI18n(locale)

  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(i18n)

  // Init theme so colors resolve correctly
  const appStore = useAppStore()
  appStore.initTheme()

  app.mount('#app')
}

bootstrap()
