<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { GatewayVisualStatus } from '@/stores/openclaw-gateway'
import { cn } from '@/lib/utils'

const props = defineProps<{
  visualStatus: GatewayVisualStatus
}>()

const { t } = useI18n()

const tagText = computed(() => {
  switch (props.visualStatus) {
    case GatewayVisualStatus.Error:
      return t('settings.openclawRuntime.composer.gatewayTagError')
    case GatewayVisualStatus.Stop:
      return t('settings.openclawRuntime.composer.gatewayTagStop')
    case GatewayVisualStatus.Starting:
      return t('settings.openclawRuntime.composer.gatewayTagStarting')
    default:
      return t('settings.openclawRuntime.composer.gatewayTagStop')
  }
})

const tagClass = computed(() =>
  cn(
    'inline-flex w-fit max-w-full shrink-0 rounded-md border px-2 py-0.5 text-xs font-semibold',
    props.visualStatus === GatewayVisualStatus.Error &&
      'border-rose-300 text-rose-600 dark:border-rose-600/50 dark:text-rose-400',
    props.visualStatus === GatewayVisualStatus.Stop &&
      'border-neutral-300 text-neutral-600 dark:border-white/25 dark:text-muted-foreground',
    props.visualStatus === GatewayVisualStatus.Starting &&
      'border-amber-300 text-amber-700 dark:border-amber-600/50 dark:text-amber-400'
  )
)
</script>

<template>
  <!-- Status tag only; disabled hint is shown as the textarea placeholder (no duplicate line). -->
  <div class="mb-3 flex w-full min-w-0 text-left">
    <span :class="tagClass">{{ tagText }}</span>
  </div>
</template>
