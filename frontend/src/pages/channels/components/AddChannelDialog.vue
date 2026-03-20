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

function getPlatformIconBg(platformId: string): string {
  switch (platformId) {
    case 'wecom':
      return '#E6FFE6'
    case 'qq':
    case 'dingtalk':
      return '#E5F3FF'
    case 'feishu':
      return '#E1FAF7'
    default:
      return '#F5F5F5'
  }
}

/** Platforms that support adding a channel in UI (feishu + wecom + dingtalk + qq). */
function isChannelPlatformSelectable(platformId: string) {
  return (
    platformId === 'feishu' ||
    platformId === 'wecom' ||
    platformId === 'dingtalk' ||
    platformId === 'qq'
  )
}

function getPlatformDisplayName(platformId: string, fallbackName?: string): string {
  const key = `channels.platforms.${platformId}`
  if (te(key)) return t(key)
  return fallbackName || platformId
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent
      class="sm:max-w-[540px] gap-0 overflow-hidden p-0 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <DialogHeader class="gap-1 px-6 pb-3 pt-5 text-left sm:text-left">
        <DialogTitle class="text-lg font-semibold tracking-tight text-foreground">
          {{ t('channels.add.title') }}
        </DialogTitle>
        <DialogDescription class="text-left leading-5">
          {{ t('channels.add.desc') }}
        </DialogDescription>
      </DialogHeader>

      <div class="max-h-[min(420px,calc(100vh-12rem))] overflow-y-auto px-6 pb-6 pt-1">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div
            v-for="platform in platforms"
            :key="platform.id"
            class="flex min-w-0 items-center justify-between gap-2 rounded-2xl border p-3 transition-all shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10 dark:bg-card sm:p-4"
            :class="
              isChannelPlatformSelectable(platform.id)
                ? localSelected?.id === platform.id
                  ? 'cursor-pointer border-[#171717] bg-white dark:border-primary'
                  : 'cursor-pointer border-[#d4d4d4] bg-white hover:border-[#171717]/50 dark:border-border dark:hover:border-primary/50'
                : 'cursor-not-allowed border-border bg-muted/25 opacity-50 dark:bg-muted/20'
            "
            @click="
              isChannelPlatformSelectable(platform.id)
                ? handleSelect(platform)
                : toast.default(t('channels.comingSoon'))
            "
          >
            <div class="flex min-w-0 flex-1 items-center gap-2 sm:gap-[11px]">
              <div
                class="relative flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full"
                :style="{ backgroundColor: getPlatformIconBg(platform.id) }"
              >
                <img
                  v-if="getPlatformIcon(platform.id)"
                  :src="getPlatformIcon(platform.id)!"
                  :alt="platform.id"
                  class="block h-5 w-5 object-contain"
                />
                <span v-else class="text-xs leading-none text-muted-foreground">🤖</span>
              </div>
              <p class="truncate text-sm font-medium leading-[22px] text-[#171717] dark:text-foreground">
                {{ getPlatformDisplayName(platform.id, platform.name) }}
              </p>
            </div>

            <div class="flex shrink-0 items-center sm:ml-2">
              <div
                class="flex h-4 w-4 items-center justify-center rounded-full border transition-colors"
                :class="
                  localSelected?.id === platform.id
                    ? 'border-[#171717] bg-[#171717] dark:border-primary dark:bg-primary'
                    : 'border-[#d9d9d9] bg-white dark:border-border dark:bg-background'
                "
              >
                <div
                  v-if="localSelected?.id === platform.id"
                  class="h-1.5 w-1.5 rounded-full bg-white dark:bg-primary-foreground"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <DialogFooter
        class="border-t border-[#f0f0f0] bg-background px-6 py-4 dark:border-border/50 dark:bg-muted/20"
      >
        <Button
          variant="secondary"
          class="h-9 border-0 bg-[#f5f5f5] px-4 text-[#171717] shadow-none hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
          @click="open = false"
        >
          {{ t('common.cancel') }}
        </Button>
        <Button
          class="h-9 bg-[#171717] px-4 text-white shadow-none hover:bg-[#171717]/90 disabled:opacity-50 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
          :disabled="!localSelected"
          @click="handleConfirm"
        >
          {{ t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
