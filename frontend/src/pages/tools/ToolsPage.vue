<script setup lang="ts">
import { onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { useAppStore, type SystemOwner } from '@/stores/app'

const props = defineProps<{
  tabId: string
  systemOwner?: SystemOwner
}>()

const { t } = useI18n()
const appStore = useAppStore()
const { currentSystem } = storeToRefs(appStore)

function logSystemContext() {
  console.log('[ToolsPage] currentSystem:', currentSystem.value, 'systemOwner:', props.systemOwner)
}

onMounted(logSystemContext)
watch([currentSystem, () => props.systemOwner], logSystemContext)
</script>

<template>
  <div class="flex h-full w-full flex-col items-center justify-center gap-2 text-muted-foreground">
    <span>{{ t('nav.tools') }}</span>
    <div class="font-mono text-xs text-foreground/80">
      currentSystem: {{ currentSystem }} · systemOwner: {{ props.systemOwner ?? '—' }}
    </div>
  </div>
</template>
