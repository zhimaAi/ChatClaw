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
import { openExternalLink } from '@/pages/common/platformDocs'

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{
  connected: [channelId: number]
}>()

const { t } = useI18n()

type Step = 'initial' | 'generating' | 'qrcode'

const step = ref<Step>('initial')
const qrcodeDataUrl = ref('')
const sessionKey = ref('')
const preparing = ref(false)
const refreshing = ref(false)
const isPolling = ref(false)
const qrExpired = ref(false)
let prepareRequestId = 0

function whatsappResultChannelId(result: unknown): number {
  if (!result || typeof result !== 'object') return 0
  const r = result as { channel_id?: number; channelId?: number }
  const a = r.channel_id
  const b = r.channelId
  if (typeof a === 'number' && a > 0) return a
  if (typeof b === 'number' && b > 0) return b
  return 0
}

watch(open, (val) => {
  const requestId = ++prepareRequestId
  if (val) {
    preparing.value = true
    step.value = 'initial'
    qrcodeDataUrl.value = ''
    sessionKey.value = ''
    refreshing.value = false
    isPolling.value = false
    qrExpired.value = false
    void (async () => {
      try {
        const prep = await OpenClawChannelService.PrepareWhatsappChannel()
        if (requestId !== prepareRequestId || !open.value) return
        if (!prep?.ready) {
          toast.default(t('channels.whatsapp.pluginInstallTryLater'))
          open.value = false
        }
      } catch (error) {
        if (requestId !== prepareRequestId || !open.value) return
        toast.error(getErrorMessage(error))
        open.value = false
      } finally {
        if (requestId === prepareRequestId) {
          preparing.value = false
        }
      }
    })()
    return
  }
  preparing.value = false
  if (!val) {
    void OpenClawChannelService.CancelWhatsappLogin(sessionKey.value)
    step.value = 'initial'
    qrcodeDataUrl.value = ''
    sessionKey.value = ''
    refreshing.value = false
    isPolling.value = false
    qrExpired.value = false
  }
})

async function doGenerateQRCode(): Promise<boolean> {
  qrExpired.value = false
  step.value = 'generating'
  try {
    const result = await OpenClawChannelService.GenerateWhatsappQRCode()
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
  if (preparing.value) return
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
    const result = await OpenClawChannelService.WaitForWhatsappLogin(key, '')
    if (isStalePoll(key)) return
    if (!result) {
      if (open.value) qrExpired.value = true
      return
    }
    if (!open.value) return
    if (result.connected) {
      const cid = whatsappResultChannelId(result)
      emit('connected', cid)
      open.value = false
    } else if (open.value) {
      qrExpired.value = true
    }
  } catch {
    if (open.value && !isStalePoll(key)) qrExpired.value = true
  } finally {
    if (!isStalePoll(key)) isPolling.value = false
  }
}

async function handleRefreshQRCode() {
  if (preparing.value || refreshing.value) return
  refreshing.value = true
  try {
    const ok = await doGenerateQRCode()
    if (!ok) step.value = 'qrcode'
  } finally {
    refreshing.value = false
  }
}

function openDocs() {
  void openExternalLink('https://docs.openclaw.ai/channels/whatsapp')
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="max-w-[520px] gap-0 overflow-hidden p-0">
      <DialogHeader class="px-6 pt-6 pb-4">
        <DialogTitle class="text-lg font-semibold text-foreground">
          {{ t("channels.whatsapp.configTitle") }}
        </DialogTitle>
      </DialogHeader>

      <div v-if="preparing" class="px-6 pb-6">
        <div
          class="flex min-h-[240px] flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground"
        >
          <LoaderCircle class="h-6 w-6 animate-spin" />
          <p>{{ t('common.loading', '处理中...') }}</p>
        </div>
      </div>

      <div v-else-if="step === 'initial' || step === 'generating'" class="px-6 pb-6 space-y-5">
        <div
          class="rounded-lg bg-muted/50 border border-border p-4 space-y-1.5 text-sm text-foreground"
        >
          <p class="font-medium text-muted-foreground">{{ t("channels.whatsapp.howToConnect") }}</p>
          <p>
            {{ t("channels.whatsapp.step1") }}
            <button
              type="button"
              class="ml-1 text-primary underline-offset-2 hover:underline"
              @click="openDocs"
            >
              {{ t("channels.whatsapp.configStepsLink") }}
            </button>
          </p>
          <p>{{ t("channels.whatsapp.step2") }}</p>
          <p>{{ t("channels.whatsapp.step3") }}</p>
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
                ? t('channels.whatsapp.generating')
                : t('channels.whatsapp.generateQRCode')
            }}
          </Button>
        </div>
      </div>

      <div v-else-if="step === 'qrcode'" class="px-6 pb-6 space-y-5">
        <p class="text-sm text-muted-foreground">{{ t("channels.whatsapp.scanHint") }}</p>

        <div
          v-if="qrExpired"
          class="rounded-md border border-border border-l-[3px] border-l-muted-foreground bg-muted/30 px-3 py-2.5 text-center text-sm text-muted-foreground shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
        >
          {{ t('channels.whatsapp.qrExpiredHint') }}
        </div>

        <div class="flex justify-center">
          <div
            class="relative flex h-[220px] w-[220px] items-center justify-center overflow-hidden rounded-xl border border-border bg-white shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
          >
            <img
              v-if="qrcodeDataUrl"
              :src="qrcodeDataUrl"
              alt="WhatsApp QR Code"
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
                {{ t('channels.whatsapp.qrExpired') }}
              </span>
            </div>
          </div>
        </div>

        <div
          v-if="isPolling"
          class="flex items-center justify-center gap-2 text-xs text-muted-foreground"
        >
          <LoaderCircle class="h-3.5 w-3.5 animate-spin shrink-0" />
          <span>{{ t("channels.whatsapp.waitingForScan") }}</span>
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
            {{ t("channels.whatsapp.refresh") }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
