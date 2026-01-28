import { createApp } from 'vue'
import App from './App.vue'
import { initI18n } from './i18n'
import { fetchLocale } from './composables/useLocale'

async function bootstrap() {
  // 从后端获取语言配置，初始化 i18n
  const locale = await fetchLocale()
  const i18n = initI18n(locale)

  const app = createApp(App)
  app.use(i18n)
  app.mount('#app')
}

bootstrap()
