<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast, useToast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  OpenClawChannelService,
  CreateChannelInput,
} from '@bindings/chatclaw/internal/services/openclaw/channels'
import type { Channel } from '@bindings/chatclaw/internal/services/channels'

/** Poll interval: avoid high frequency (rate limits). */
const POLL_INTERVAL_MS = 2000
/** WeCom QR session validity (product requirement). */
const QR_VALID_MS = 5 * 60 * 1000

function qrImageUrlForAuth(authUrl: string) {
  return `https://api.qrserver.com/v1/create-qr-code/?size=220x220&margin=8&data=${encodeURIComponent(authUrl)}`
}

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{
  saved: [channel: Channel, isEdit: boolean]
  manual: []
}>()

const { t } = useI18n()
const { toast: pushToast } = useToast()

type UiState = 'tips' | 'scan'
const uiState = ref<UiState>('tips')
const scode = ref('')
const authUrl = ref('')
const generating = ref(false)
const registering = ref(false)
/** Timestamp when current QR was issued (for 5-minute expiry). */
const qrIssuedAt = ref<number | null>(null)
const qrExpired = ref(false)

let pollTimer: ReturnType<typeof setInterval> | null = null

const qrImageSrc = computed(() => (authUrl.value ? qrImageUrlForAuth(authUrl.value) : ''))

function stopPolling() {
  if (pollTimer != null) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

function resetDialogBody() {
  stopPolling()
  uiState.value = 'tips'
  scode.value = ''
  authUrl.value = ''
  generating.value = false
  registering.value = false
  qrIssuedAt.value = null
  qrExpired.value = false
}

watch(open, (val) => {
  if (!val) {
    resetDialogBody()
  }
})

onUnmounted(() => {
  stopPolling()
})

async function fetchGenerateFromBackend() {
  const res = await OpenClawChannelService.WecomAuthQRGenerate()
  const s = res?.scode?.trim()
  const u = res?.auth_url?.trim()
  if (!s || !u) throw new Error('invalid response')
  return { scode: s, authUrl: u }
}

function extractBotCredentials(botInfo: unknown): { botId: string; secret: string } | null {
  if (!botInfo || typeof botInfo !== 'object') return null
  const o = botInfo as Record<string, unknown>
  const botId = String(o.botid ?? o.botId ?? o.bot_id ?? '').trim()
  const secret = String(o.secret ?? o.app_secret ?? '').trim()
  if (!botId || !secret) return null
  return { botId, secret }
}

async function registerChannel(botId: string, secret: string) {
  const name = t('channels.wecomAdd.defaultName')
  const extraConfig = JSON.stringify({
    platform: 'wecom',
    app_id: botId,
    app_secret: secret,
  })
  const channel = await OpenClawChannelService.CreateChannel(
    new CreateChannelInput({
      platform: 'wecom',
      name,
      avatar: '',
      extra_config: extraConfig,
      agent_id: 0,
    })
  )
  if (!channel) throw new Error('no channel')
  return channel
}

function isQrTimedOut(): boolean {
  if (qrIssuedAt.value == null) return false
  return Date.now() - qrIssuedAt.value > QR_VALID_MS
}

async function handlePollTick() {
  if (!scode.value || !open.value) return
  if (qrExpired.value) return

  if (isQrTimedOut()) {
    stopPolling()
    qrExpired.value = true
    pushToast({
      title: t('channels.wecomAdd.qrExpired'),
      description: t('channels.wecomAdd.qrExpiredHint'),
      variant: 'default',
      duration: 8000,
    })
    return
  }

  try {
    const data = await OpenClawChannelService.WecomAuthQRQuery(scode.value)
    if (!data) return
    const status = data.status
    if (status === 'success') {
      stopPolling()
      const creds = extractBotCredentials(data.bot_info)
      if (!creds) {
        toast.error(t('channels.wecomAdd.missingCredentials'))
        return
      }
      registering.value = true
      try {
        const channel = await registerChannel(creds.botId, creds.secret)
        open.value = false
        emit('saved', channel, false)
      } catch (e) {
        toast.error(getErrorMessage(e) || t('channels.wecomAdd.registerFailed'))
      } finally {
        registering.value = false
      }
      return
    }
    if (status === 'failed' || status === 'expired' || status === 'error') {
      stopPolling()
      toast.error(t('channels.wecomAdd.authFailed'))
    }
  } catch {
    // keep polling on transient errors
  }
}

function startPolling() {
  stopPolling()
  void handlePollTick()
  pollTimer = setInterval(() => {
    void handlePollTick()
  }, POLL_INTERVAL_MS)
}

async function handleGenerateQr() {
  generating.value = true
  qrExpired.value = false
  try {
    const { scode: s, authUrl: u } = await fetchGenerateFromBackend()
    scode.value = s
    authUrl.value = u
    qrIssuedAt.value = Date.now()
    uiState.value = 'scan'
    startPolling()
  } catch (e) {
    toast.error(getErrorMessage(e) || t('channels.wecomAdd.generateFailed'))
  } finally {
    generating.value = false
  }
}

async function handleRefreshQr() {
  await handleGenerateQr()
}

function handleManualEntry() {
  open.value = false
  emit('manual')
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent
      class="sm:max-w-[500px] gap-0 overflow-hidden p-0 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <DialogHeader class="gap-1 px-6 pb-2 pt-5 text-left sm:text-left">
        <DialogTitle class="text-lg font-semibold tracking-tight text-foreground">
          {{ t('channels.wecomAdd.title') }}
        </DialogTitle>
        <DialogDescription class="text-left text-sm leading-5 text-muted-foreground">
          {{ t('channels.wecomAdd.subtitle') }}
        </DialogDescription>
      </DialogHeader>

      <div class="px-6 pb-4 pt-2">
        <!-- Tips state -->
        <div
          v-if="uiState === 'tips'"
          class="rounded-lg bg-[#f5f5f5] px-4 py-4 text-sm leading-6 text-[#171717] dark:bg-muted/60 dark:text-foreground"
        >
          <p class="font-medium">{{ t('channels.wecomAdd.howTitle') }}</p>
          <p class="mt-2">{{ t('channels.wecomAdd.tipsIntro') }}</p>
          <p class="mt-2 font-medium">{{ t('channels.wecomAdd.stepsLabel') }}</p>
          <ol class="mt-2 list-decimal space-y-2 pl-5 [list-style-position:outside]">
            <li>{{ t('channels.wecomAdd.step1') }}</li>
            <li>{{ t('channels.wecomAdd.step2') }}</li>
          </ol>
          <p class="mt-2">{{ t('channels.wecomAdd.tipsOrManual') }}</p>
        </div>

        <!-- Scan state -->
        <div v-else class="flex flex-col items-center py-2">
          <div
            v-if="qrExpired"
            class="mb-3 w-full rounded-md border border-border border-l-[3px] border-l-muted-foreground bg-muted/30 px-3 py-2.5 text-center text-sm text-muted-foreground shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
          >
            {{ t('channels.wecomAdd.qrExpiredHint') }}
          </div>
          <div
            class="flex h-[240px] w-[240px] items-center justify-center rounded-lg border border-border bg-white p-2 dark:bg-card"
            :class="{ 'opacity-50': qrExpired }"
          >
            <img
              v-if="qrImageSrc"
              :src="qrImageSrc"
              alt=""
              class="h-[220px] w-[220px] object-contain"
            />
          </div>
          <p class="mt-3 max-w-sm text-center text-xs text-muted-foreground">
            {{ t('channels.wecomAdd.scanHint') }}
          </p>
          <p v-if="registering" class="mt-2 flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle class="size-4 animate-spin text-muted-foreground" />
            {{ t('channels.wecomAdd.registering') }}
          </p>
        </div>
      </div>

      <DialogFooter
        v-if="uiState === 'tips'"
        class="flex flex-row items-center justify-end gap-3 border-t border-[#f0f0f0] bg-background px-6 py-4 dark:border-border/50 dark:bg-muted/20"
      >
        <Button
          type="button"
          variant="outline"
          class="h-9 border-border bg-background text-foreground shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
          :disabled="generating"
          @click="handleManualEntry"
        >
          {{ t('channels.wecomAdd.manualEntry') }}
        </Button>
        <Button
          type="button"
          class="h-9 bg-[#171717] px-4 text-white shadow-none hover:bg-[#171717]/90 disabled:opacity-50 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
          :disabled="generating"
          @click="handleGenerateQr"
        >
          <LoaderCircle v-if="generating" class="mr-2 size-4 shrink-0 animate-spin" />
          {{ generating ? t('channels.wecomAdd.generating') : t('channels.wecomAdd.generateQr') }}
        </Button>
      </DialogFooter>

      <DialogFooter
        v-else
        class="flex flex-row items-center justify-center border-t border-[#f0f0f0] bg-background px-6 py-4 dark:border-border/50 dark:bg-muted/20"
      >
        <Button
          type="button"
          variant="outline"
          class="h-9 min-w-[120px] border-border bg-background text-foreground shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
          :disabled="generating || registering"
          @click="handleRefreshQr"
        >
          <LoaderCircle v-if="generating" class="mr-2 size-4 shrink-0 animate-spin" />
          {{ t('channels.wecomAdd.refreshQr') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
