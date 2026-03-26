import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { initI18n } from '../i18n'
import { fetchLocale } from '../composables/useLocale'
import { useAppStore } from '../stores'
import '../assets/index.css'

async function bootstrap() {
  const locale = await fetchLocale()
  const i18n = await initI18n(locale)

  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(i18n)

  // Initialize theme so assistant styles stay consistent inside the iframe page.
  const appStore = useAppStore()
  appStore.initTheme()

  app.mount('#app')
}

bootstrap()
