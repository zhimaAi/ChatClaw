
<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle, RefreshCw, QrCode, CheckCircle } from 'lucide-vue-next'
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
const emit = defineEmits<{ connected: [] }>()

const { t } = useI18n()

type Step = 'initial' | 'generating' | 'qrcode' | 'waiting' | 'success'

const step = ref<Step>('initial')
const qrcodeDataUrl = ref('')
const sessionKey = ref('')
const refreshing = ref(false)
const successMessage = ref('')

watch(open, (val) => {
  if (!val) {
    step.value = 'initial'
    qrcodeDataUrl.value = ''
    sessionKey.value = ''
    refreshing.value = false
    successMessage.value = ''
  }
})

async function handleGenerateQRCode() {
  step.value = 'generating'
  try {
    const result = await OpenClawChannelService.GenerateWechatQRCode()
    if (!result) throw new Error('No result returned')
    qrcodeDataUrl.value = result.qrcode_data_url
    sessionKey.value = result.session_key
    step.value = 'qrcode'
    // Start waiting for scan in background
    void startWaitingForScan(result.session_key)
  } catch (error) {
    toast.error(getErrorMessage(error))
    step.value = 'initial'
  }
}

async function startWaitingForScan(key: string) {
  if (!key) return
  step.value = 'waiting'
  try {
    const result = await OpenClawChannelService.WaitForWechatLogin(key, '')
    if (!result) return
    if (result.connected) {
      successMessage.value = result.message || t('channels.wechat.loginSuccess')
      step.value = 'success'
      emit('connected')
    }
    // If not connected (timeout/cancelled), go back to qrcode step to allow refresh
    if (!result.connected && step.value === 'waiting') {
      step.value = 'qrcode'
    }
  } catch {
    // On error or timeout, go back to qrcode step
    if (step.value === 'waiting') {
      step.value = 'qrcode'
    }
  }
}

async function handleRefreshQRCode() {
  if (refreshing.value) return
  refreshing.value = true
  step.value = 'generating'
  try {
    const result = await OpenClawChannelService.GenerateWechatQRCode()
    if (!result) throw new Error('No result returned')
    qrcodeDataUrl.value = result.qrcode_data_url
    sessionKey.value = result.session_key
    step.value = 'qrcode'
    void startWaitingForScan(result.session_key)
  } catch (error) {
    toast.error(getErrorMessage(error))
    step.value = 'qrcode'
  } finally {
    refreshing.value = false
  }
}

function openConfigSteps() {
  void openExternalLink('https://docs.openclaw.io/channels/wechat')
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
        <!-- Instructions Box -->
        <div
          class="rounded-lg bg-muted/50 border border-border p-4 space-y-1.5 text-sm text-foreground"
        >
          <p class="font-medium text-muted-foreground">{{ t('channels.wechat.howToConnect') }}</p>
          <p>
            {{ t('channels.wechat.step1') }}
            <button
              class="ml-1 text-primary underline-offset-2 hover:underline"
              @click="openConfigSteps"
            >
              {{ t('channels.wechat.configStepsLink') }}
            </button>
          </p>
          <p>{{ t('channels.wechat.step3') }}</p>
          <p>{{ t('channels.wechat.step4') }}</p>
          <p>{{ t('channels.wechat.step5') }}</p>
        </div>

        <!-- Actions -->
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

      <!-- QR Code Step (waiting for scan) -->
      <div v-else-if="step === 'qrcode' || step === 'waiting'" class="px-6 pb-6 space-y-5">
        <p class="text-sm text-muted-foreground">{{ t('channels.wechat.scanHint') }}</p>

        <!-- QR Code Image -->
        <div class="flex justify-center">
          <div
            class="relative flex h-[220px] w-[220px] items-center justify-center overflow-hidden rounded-xl border border-border bg-white shadow-sm"
          >
            <!-- Scanning overlay -->
            <div
              v-if="step === 'waiting'"
              class="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-white/80 dark:bg-black/50 backdrop-blur-[2px] rounded-xl z-10"
            >
              <LoaderCircle class="h-7 w-7 animate-spin text-muted-foreground" />
              <span class="text-xs text-muted-foreground px-4 text-center">
                {{ t('channels.wechat.waitingForScan') }}
              </span>
            </div>
            <img
              v-if="qrcodeDataUrl"
              :src="qrcodeDataUrl"
              alt="WeChat QR Code"
              class="h-full w-full object-contain p-3"
            />
            <div v-else class="flex h-full w-full items-center justify-center">
              <LoaderCircle class="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          </div>
        </div>

        <!-- Refresh Button -->
        <div class="flex justify-center">
          <Button
            variant="outline"
            class="h-10 gap-2 border-border px-6 shadow-sm dark:ring-1 dark:ring-white/10"
            :disabled="refreshing || step === 'waiting'"
            @click="handleRefreshQRCode"
          >
            <LoaderCircle v-if="refreshing || step === 'waiting'" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('channels.wechat.refresh') }}
          </Button>
        </div>
      </div>

      <!-- Success Step -->
      <div v-else-if="step === 'success'" class="px-6 pb-6 space-y-5">
        <div class="flex flex-col items-center gap-4 py-6 text-center">
          <CheckCircle class="h-12 w-12 text-green-500" />
          <div class="space-y-1">
            <p class="text-base font-medium text-foreground">{{ t('channels.wechat.loginSuccess') }}</p>
            <p class="text-sm text-muted-foreground">{{ successMessage }}</p>
          </div>
          <Button
            class="mt-2 h-10 gap-2 bg-foreground text-background hover:bg-foreground/90 dark:bg-primary dark:text-primary-foreground dark:hover:bg-primary/90"
            @click="open = false"
          >
            {{ t('common.close') }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
