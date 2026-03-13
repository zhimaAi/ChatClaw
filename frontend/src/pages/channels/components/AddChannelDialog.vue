<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { PlatformMeta } from '@bindings/chatclaw/internal/services/channels'
import { platformIconMap } from '@/assets/icons/snap/platformIcons'
import { toast } from '@/components/ui/toast'

defineProps<{
  platforms: PlatformMeta[]
}>()

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{
  select: [platform: PlatformMeta]
}>()

const { t, te } = useI18n()

const localSelected = ref<PlatformMeta | null>(null)

watch(open, (val) => {
  if (val) {
    localSelected.value = null
  }
})

function handleSelect(platform: PlatformMeta) {
  localSelected.value = platform
}

function handleConfirm() {
  if (localSelected.value) {
    emit('select', localSelected.value)
    open.value = false
  }
}

function getPlatformIcon(platformId: string): string | null {
  return platformIconMap[platformId] || null
}

function getPlatformDisplayName(platformId: string, fallbackName?: string): string {
  const key = `channels.platforms.${platformId}`
  if (te(key)) return t(key)
  return fallbackName || platformId
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-[480px] p-0 gap-0 overflow-hidden">
      <DialogHeader class="px-6 py-4">
        <DialogTitle class="text-lg font-semibold text-[#0a0a0a] dark:text-foreground">
          {{ t('channels.add.title') }}
        </DialogTitle>
        <DialogDescription class="hidden">{{ t('channels.add.desc') }}</DialogDescription>
      </DialogHeader>
      
      <div class="px-6 pb-6 pt-2">
        <div class="flex flex-col items-start overflow-clip rounded-xl border border-[#e5e5e5] bg-white shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:border-border dark:bg-card">
          <div
            v-for="platform in platforms"
            :key="platform.id"
            class="flex w-full items-center justify-between border-b border-[#f0f0f0] p-4 last:border-b-0 transition-colors dark:border-border"
            :class="[
              platform.id === 'feishu' || platform.id === 'dingtalk' ? 'cursor-pointer hover:bg-[#fcfcfc] dark:hover:bg-muted/50' : 'cursor-not-allowed opacity-50 bg-[#f9f9f9] dark:bg-muted/20'
            ]"
            @click="platform.id === 'feishu' || platform.id === 'dingtalk' ? handleSelect(platform) : toast.default(t('channels.comingSoon'))"
          >
            <div class="flex flex-1 items-center gap-2">
              <div class="relative flex h-5 w-5 shrink-0 items-center justify-center overflow-hidden">
                <img
                  v-if="getPlatformIcon(platform.id)"
                  :src="getPlatformIcon(platform.id)!"
                  :alt="platform.id"
                  class="block h-full w-full object-contain"
                />
                <span v-else class="text-xs">🤖</span>
              </div>
              <p class="text-sm font-medium leading-[20px] text-[#0a0a0a] dark:text-foreground">
                {{ getPlatformDisplayName(platform.id, platform.name) }}
              </p>
            </div>
            
            <div class="flex items-center shrink-0">
              <div
                class="flex h-4 w-4 items-center justify-center rounded-full border transition-colors"
                :class="localSelected?.id === platform.id ? 'border-[#171717] bg-[#171717] dark:border-primary dark:bg-primary' : 'border-[#d9d9d9] bg-white dark:border-border dark:bg-background'"
              >
                <div v-if="localSelected?.id === platform.id" class="h-1.5 w-1.5 rounded-full bg-white dark:bg-primary-foreground"></div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <DialogFooter class="px-6 py-4 border-t border-[#f0f0f0] bg-white dark:border-border/50 dark:bg-muted/20">
        <Button 
          class="h-9 px-4 bg-[#f5f5f5] text-[#171717] hover:bg-[#e5e5e5] border-none shadow-none dark:bg-muted dark:text-foreground dark:hover:bg-muted/80" 
          @click="open = false"
        >
          {{ t('common.cancel') }}
        </Button>
        <Button 
          class="h-9 px-4 bg-[#171717] text-white hover:bg-[#171717]/90 disabled:opacity-50 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90" 
          @click="handleConfirm" 
          :disabled="!localSelected"
        >
          {{ t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
