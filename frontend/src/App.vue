<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import HelloWorld from './components/HelloWorld.vue'
import { Button } from '@/components/ui/button'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const toggleTheme = () => {
  const themes = ['light', 'dark', 'system'] as const
  const currentIndex = themes.indexOf(appStore.theme)
  const nextIndex = (currentIndex + 1) % themes.length
  appStore.setTheme(themes[nextIndex])
}
</script>

<template>
  <div class="flex min-h-screen flex-col items-center justify-center bg-background text-foreground">
    <div class="mb-4 flex gap-4">
      <a data-wml-openURL="https://wails.io">
        <img src="/wails.png" class="logo" alt="Wails logo" />
      </a>
      <a data-wml-openURL="https://vuejs.org/">
        <img src="/vue.svg" class="logo vue" alt="Vue logo" />
      </a>
    </div>

    <HelloWorld :msg="t('app.title')" />

    <div class="mt-6">
      <Button variant="outline" @click="toggleTheme">
        {{ t('app.theme') }}: {{ appStore.theme }}
      </Button>
    </div>
  </div>
</template>

<style scoped>
.logo {
  height: 6em;
  padding: 1.5em;
  will-change: filter;
  transition: filter 0.3s;
}
.logo:hover {
  filter: drop-shadow(0 0 2em #e80000aa);
}
.logo.vue:hover {
  filter: drop-shadow(0 0 2em #42b883aa);
}
</style>
