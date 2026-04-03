<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { storeToRefs } from 'pinia'
import { Info } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { useAppStore, useOpenClawGatewayStore, GatewayVisualStatus } from '@/stores'

const props = defineProps<{
  variant: 'channels' | 'scheduledTasks'
}>()

const { t } = useI18n()
const appStore = useAppStore()
const gatewayStore = useOpenClawGatewayStore()
const { visualStatus } = storeToRefs(gatewayStore)

onMounted(() => {
  if (appStore.currentSystem === 'openclaw') {
    void gatewayStore.poll()
  }
})

const show = computed(
  () =>
    appStore.currentSystem === 'openclaw' &&
    visualStatus.value !== GatewayVisualStatus.Running
)

const message = computed(() => {
  if (visualStatus.value === GatewayVisualStatus.Starting) {
    return t('openclawGateway.banner.starting')
  }
  if (props.variant === 'channels') {
    return t('openclawGateway.banner.channels')
  }
  return t('openclawGateway.banner.scheduledTasks')
})
</script>

<template>
  <div
    v-if="show"
    class="mb-4 flex w-full min-w-0 items-start gap-3 rounded-md border border-[#FFD591] bg-[#FFF7E6] px-4 py-3 text-sm leading-snug text-[#D46B08] shadow-sm dark:border-amber-600/45 dark:bg-amber-950/35 dark:text-amber-200/95 dark:shadow-none dark:ring-1 dark:ring-white/10"
    role="status"
  >
    <Info class="mt-0.5 size-5 shrink-0 text-current" aria-hidden="true" :stroke-width="2" />
    <span>{{ message }}</span>
  </div>
</template>
