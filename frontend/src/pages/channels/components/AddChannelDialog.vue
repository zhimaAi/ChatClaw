<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { PlatformMeta } from '@bindings/chatclaw/internal/services/channels'

defineProps<{
  platforms: PlatformMeta[]
}>()

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{
  select: [platform: PlatformMeta]
}>()

const { t } = useI18n()

const platformIconMap: Record<string, string> = {
  feishu: '🐦',
  telegram: '✈️',
  discord: '🎮',
  whatsapp: '📱',
  dingtalk: '💬',
}

function handleSelect(platform: PlatformMeta) {
  emit('select', platform)
  open.value = false
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ t('channels.add.title') }}</DialogTitle>
        <DialogDescription>{{ t('channels.add.desc') }}</DialogDescription>
      </DialogHeader>
      <div class="grid grid-cols-2 gap-3 py-2">
        <button
          v-for="platform in platforms"
          :key="platform.id"
          class="flex items-start gap-3 rounded-lg border border-border p-3 text-left transition-all hover:border-primary/40 hover:bg-accent/50"
          @click="handleSelect(platform)"
        >
          <span class="text-xl">{{ platformIconMap[platform.id] || '🤖' }}</span>
          <div class="min-w-0">
            <p class="text-sm font-medium text-foreground">
              {{ t(`channels.meta.${platform.id}.name`) }}
            </p>
            <p class="text-xs text-muted-foreground">
              {{ t(`channels.authType.${platform.auth_type}`) }}
            </p>
          </div>
        </button>
      </div>
    </DialogContent>
  </Dialog>
</template>
