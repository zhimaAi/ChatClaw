<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle, RefreshCw, QrCode } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { OpenClawChannelService } from '@bindings/chatclaw/internal/services/openclaw/channels'
const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{
  connected: [channelId: number]
}>()

const { t } = useI18n()

type Step = 'initial' | 'generating' | 'qrcode'

const step = ref<Step>('initial')
const qrcodeDataUrl = ref('')
const sessionKey = ref('')
const refreshing = ref(false)
const isPolling = ref(false)
/** True after login wait ends without success (timeout, API expired, or error). */
const qrExpired = ref(false)

function wechatResultChannelId(result: unknown): number {
  if (!result || typeof result !== 'object') return 0
  const r = result as { channel_id?: number; channelId?: number }
  const a = r.channel_id
  const b = r.channelId
  if (typeof a === 'number' && a > 0) return a
  if (typeof b === 'number' && b > 0) return b
  return 0
}

watch(open, (val) => {
  if (!val) {
    step.value = 'initial'
    qrcodeDataUrl.value = ''
    sessionKey.value = ''
    refreshing.value = false
    isPolling.value = false
    qrExpired.value = false
  }
})

/** Core QR generation: fetches a new code, updates state, and kicks off polling. Returns true on success. */
async function doGenerateQRCode(): Promise<boolean> {
  qrExpired.value = false
  step.value = 'generating'
  try {
    const result = await OpenClawChannelService.GenerateWechatQRCode()
    if (!result) throw new Error('No result returned')
    qrcodeDataUrl.value = result.qrcode_data_url
    sessionKey.value = result.session_key
    step.value = 'qrcode'
    void startWaitingForScan(result.session_key)
    return true
  } catch (error) {
    toast.error(getErrorMessage(error))
    return false
  }
}

async function handleGenerateQRCode() {
  const ok = await doGenerateQRCode()
  if (!ok) step.value = 'initial'
}

function isStalePoll(key: string) {
  return sessionKey.value !== key
}

async function startWaitingForScan(key: string) {
  if (!key) return
  isPolling.value = true
  try {
    const result = await OpenClawChannelService.WaitForWechatLogin(key, '')
    if (isStalePoll(key)) return
    if (!result) {
      if (open.value) qrExpired.value = true
      return
    }
    // Guard: if the user closed the dialog before scanning, discard the result.
    if (!open.value) return
    if (result.connected) {
      const cid = wechatResultChannelId(result)
      emit('connected', cid)
      open.value = false
    } else if (open.value) {
      qrExpired.value = true
    }
  } catch {
    // On error or timeout, stay on qrcode step so the user can refresh.
    if (open.value && !isStalePoll(key)) qrExpired.value = true
  } finally {
    if (!isStalePoll(key)) isPolling.value = false
  }
}

async function handleRefreshQRCode() {
  if (refreshing.value) return
  refreshing.value = true
  try {
    const ok = await doGenerateQRCode()
    if (!ok) step.value = 'qrcode' // stay on qrcode step on refresh error
  } finally {
    refreshing.value = false
  }
}

</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-[520px] gap-0 overflow-hidden p-0">
      <DialogHeader class="px-6 pt-6 pb-4">
        <DialogTitle class="text-lg font-semibold text-foreground">
          {{ t('channels.wechat.configTitle') }}
        </DialogTitle>
        <p class="mt-1 text-sm text-muted-foreground">
          {{ t('channels.wechat.configSubtitle') }}
        </p>
      </DialogHeader>

      <!-- Initial / Generating Step -->
      <div v-if="step === 'initial' || step === 'generating'" class="px-6 pb-6 space-y-5">
        <div
          class="rounded-lg bg-muted/50 border border-border p-4 space-y-1.5 text-sm text-foreground"
        >
          <p class="font-medium text-muted-foreground">{{ t('channels.wechat.howToConnect') }}</p>
          <p>{{ t('channels.wechat.step1') }}</p>
          <p>{{ t('channels.wechat.step3') }}</p>
          <p>{{ t('channels.wechat.step4') }}</p>
          <p>{{ t('channels.wechat.step5') }}</p>
        </div>

        <div class="flex justify-end">
          <Button
            class="h-10 gap-2 bg-foreground text-background hover:bg-foreground/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
            :disabled="step === 'generating'"
            @click="handleGenerateQRCode"
          >
            <LoaderCircle v-if="step === 'generating'" class="h-4 w-4 animate-spin" />
            <QrCode v-else class="h-4 w-4" />
            {{
              step === 'generating'
                ? t('channels.wechat.generating')
                : t('channels.wechat.generateQRCode')
            }}
          </Button>
        </div>
      </div>

      <!-- QR Code Step -->
      <div v-else-if="step === 'qrcode'" class="px-6 pb-6 space-y-5">
        <p class="text-sm text-muted-foreground">{{ t('channels.wechat.scanHint') }}</p>

        <div
          v-if="qrExpired"
          class="rounded-md border border-border border-l-[3px] border-l-muted-foreground bg-muted/30 px-3 py-2.5 text-center text-sm text-muted-foreground shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
        >
          {{ t('channels.wechat.qrExpiredHint') }}
        </div>

        <div class="flex justify-center">
          <div
            class="relative flex h-[220px] w-[220px] items-center justify-center overflow-hidden rounded-xl border border-border bg-white shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
          >
            <img
              v-if="qrcodeDataUrl"
              :src="qrcodeDataUrl"
              alt="WeChat QR Code"
              class="h-full w-full object-contain p-3 transition-[filter,opacity] duration-200"
              :class="{ 'grayscale opacity-[0.42]': qrExpired }"
            />
            <div v-else class="flex h-full w-full items-center justify-center">
              <LoaderCircle class="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
            <div
              v-if="qrExpired && qrcodeDataUrl"
              class="pointer-events-none absolute inset-0 flex items-center justify-center rounded-xl bg-background/55 dark:bg-background/50"
            >
              <span
                class="rounded-md border border-border bg-popover px-3 py-1.5 text-sm font-medium text-muted-foreground shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
              >
                {{ t('channels.wechat.qrExpired') }}
              </span>
            </div>
          </div>
        </div>

        <div v-if="isPolling" class="flex items-center justify-center gap-2 text-xs text-muted-foreground">
          <LoaderCircle class="h-3.5 w-3.5 animate-spin shrink-0" />
          <span>{{ t('channels.wechat.waitingForScan') }}</span>
        </div>

        <div class="flex justify-center">
          <Button
            variant="outline"
            class="h-10 gap-2 border-border px-6 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
            :disabled="refreshing || isPolling"
            @click="handleRefreshQRCode"
          >
            <LoaderCircle v-if="refreshing" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('channels.wechat.refresh') }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
