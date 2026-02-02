<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ChevronDown, FileText, Image as ImageIcon, Paperclip, PenLine, PinOff, Plus, Send } from 'lucide-vue-next'
import Logo from '@/assets/images/logo.svg'
import { SettingsService } from '@bindings/willchat/internal/services/settings'
import { SnapService } from '@bindings/willchat/internal/services/windows'

const { t } = useI18n()

const question = ref('')
const modelLabel = ref('DeepSeek V3.2 Think')

const cancelSnap = async () => {
  const keys = [
    'snap_wechat',
    'snap_wecom',
    'snap_qq',
    'snap_dingtalk',
    'snap_feishu',
    'snap_douyin',
  ] as const

  try {
    for (const k of keys) {
      await SettingsService.SetValue(k, 'false')
    }
    await SnapService.SyncFromSettings()
  } catch (error) {
    console.error('Failed to cancel snap:', error)
  }
}
</script>

<template>
  <div class="flex h-screen w-screen flex-col overflow-hidden rounded-[12px] bg-white text-[#262626]">
    <!-- Custom titlebar (frameless window drag region) -->
    <div
      class="relative flex h-[28px] items-center justify-center bg-[rgba(234,234,234,0.80)] backdrop-blur-[15px] shadow-[0px_0.5px_0px_rgba(0,0,0,0.1),0px_1px_0px_rgba(0,0,0,0.1)]"
      style="--wails-draggable: drag"
    >
      <div class="absolute left-3 flex items-center gap-2" style="--wails-draggable: no-drag">
        <div class="size-[12px] rounded-full border border-black/20 bg-[#ff5f57]" />
        <div class="size-[12px] rounded-full border border-black/20 bg-[#febc2e]" />
        <div class="size-[12px] rounded-full border border-black/20 bg-[#28c840]" />
      </div>
      <div class="text-center text-[13px] font-semibold text-black/85">{{ t('winsnap.title') }}</div>
    </div>

    <!-- Header -->
    <div class="flex h-12 items-center justify-between bg-white px-3">
      <div class="flex items-center gap-1">
        <div class="text-base font-semibold text-[#262626]">{{ t('winsnap.assistantName') }}</div>
        <ChevronDown class="size-4 text-muted-foreground" />
      </div>

      <div class="flex items-center gap-2">
        <button class="rounded-md p-1 hover:bg-muted" style="--wails-draggable: no-drag" aria-label="add">
          <Plus class="size-5 text-muted-foreground" />
        </button>
        <button
          class="rounded-md bg-muted p-1 hover:bg-muted/80"
          style="--wails-draggable: no-drag"
          aria-label="edit"
        >
          <PenLine class="size-5 text-muted-foreground" />
        </button>

        <button
          class="ml-1 inline-flex items-center gap-2 rounded-[10px] bg-white px-3 py-2 shadow-[0px_2px_8px_rgba(0,0,0,0.15)]"
          style="--wails-draggable: no-drag"
          @click="cancelSnap"
        >
          <PinOff class="size-4 text-muted-foreground" />
          <span class="text-sm text-[#262626]">{{ t('winsnap.cancelSnap') }}</span>
        </button>
      </div>
    </div>

    <!-- Main -->
    <div class="flex flex-1 flex-col items-center justify-center gap-4 bg-white px-4">
      <div class="flex items-center gap-3">
        <Logo class="size-12" />
        <div class="text-center text-4xl font-semibold leading-[44px] text-[#0a0a0a]">WillChat</div>
      </div>
    </div>

    <!-- Composer -->
    <div class="p-3">
      <div class="rounded-[16px] border-2 border-[#e5e7eb] bg-white px-3 py-3 shadow-[0px_4px_8px_rgba(0,0,0,0.06)]">
        <textarea
          v-model="question"
          class="min-h-[64px] w-full resize-none border-0 bg-transparent p-0 text-base leading-6 text-[#262626] outline-none placeholder:text-[#8c8c8c]"
          :placeholder="t('winsnap.placeholder')"
        />

        <div class="mt-3 flex items-end justify-between gap-3">
          <div class="flex flex-1 items-center gap-2">
            <button
              class="inline-flex items-center gap-2 rounded-sm border border-[#d9d9d9] bg-white px-4 py-1 text-sm leading-[22px] text-[#595959]"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <span class="text-[#595959]">{{ modelLabel }}</span>
              <ChevronDown class="size-4 text-muted-foreground" />
            </button>

            <button
              class="inline-flex size-8 items-center justify-center rounded-full border border-[#d8dde5] bg-background"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <Paperclip class="size-4 text-muted-foreground" />
            </button>
            <button
              class="inline-flex size-8 items-center justify-center rounded-full border border-[#d8dde5] bg-background"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <FileText class="size-4 text-muted-foreground" />
            </button>
            <button
              class="inline-flex size-8 items-center justify-center rounded-full border border-[#d8dde5] bg-background"
              style="--wails-draggable: no-drag"
              type="button"
            >
              <ImageIcon class="size-4 text-muted-foreground" />
            </button>
          </div>

          <button
            class="inline-flex items-center justify-center rounded-full bg-[#d8dde5] p-[5px]"
            style="--wails-draggable: no-drag"
            type="button"
            aria-label="send"
          >
            <Send class="size-[22px] rotate-180 text-muted-foreground" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
