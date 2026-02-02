<script setup lang="ts">
import { cn } from '@/lib/utils'
import { useI18n } from 'vue-i18n'
import { ProviderIcon } from '@/components/ui/provider-icon'
import type { Provider } from '@/../bindings/willchat/internal/services/providers'

const { t } = useI18n()

const props = defineProps<{
  providers: Provider[]
  selectedProviderId: string | null
  loading?: boolean
}>()

const emit = defineEmits<{
  select: [providerId: string]
}>()

const handleSelect = (providerId: string) => {
  emit('select', providerId)
}
</script>

<template>
  <div
    class="flex h-full w-56 flex-col gap-1 border-r border-border bg-background px-2 pt-2 dark:border-white/10"
  >
    <!-- 加载状态 -->
    <div v-if="loading" class="flex items-center justify-center py-8">
      <div class="size-5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
    </div>

    <!-- 供应商列表 -->
    <template v-else>
      <button
        v-for="provider in providers"
        :key="provider.provider_id"
        :class="
          cn(
            'flex w-full items-center gap-2 rounded-md px-2 py-2 text-left transition-colors',
            selectedProviderId === provider.provider_id
              ? 'bg-accent text-foreground'
              : 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'
          )
        "
        @click="handleSelect(provider.provider_id)"
      >
        <ProviderIcon :icon="provider.icon" :size="24" />
        <span class="flex-1 truncate text-sm">{{ provider.name }}</span>
        <span
          v-if="provider.enabled"
          class="shrink-0 rounded bg-green-500/15 px-1.5 py-0.5 text-xs text-green-600 dark:bg-green-500/20 dark:text-green-400"
        >
          {{ t('settings.modelService.enabled') }}
        </span>
      </button>
    </template>
  </div>
</template>
