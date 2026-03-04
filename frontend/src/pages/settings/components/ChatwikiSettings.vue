<script setup lang="ts">
/**
 * ChatWiki binding: list view + full-page flow (choose → binding → success/failure).
 * No dialog; flow is rendered as a new page in the content area.
 */
import { ref, computed, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, CheckCircle2, AlertTriangle, RotateCcw } from 'lucide-vue-next'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { Events } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import SettingsCard from './SettingsCard.vue'

const { t } = useI18n()

const CHATWIKI_CLOUD_AUTH_URL = 'http://dev9.zhima_chat_ai.applnk.cn/'
const BINDING_TIMEOUT_SEC = 120

type View = 'list' | 'choose' | 'binding' | 'success' | 'failure'

const view = ref<View>('list')
const openSourceUrl = ref('')
const showOpenSourceInput = ref(false)
const remainingSeconds = ref(BINDING_TIMEOUT_SEC)
let countdownTimer: ReturnType<typeof setInterval> | null = null
let unbindSuccess: (() => void) | null = null
let unbindFailed: (() => void) | null = null

const isOpenSourceUrlValid = computed(() => {
  const u = openSourceUrl.value.trim()
  if (!u) return false
  try {
    const url = new URL(u)
    return url.protocol === 'https:' || url.protocol === 'http:'
  } catch {
    return false
  }
})

const remainingTimeText = computed(() =>
  t('settings.chatwiki.remainingTime', { seconds: remainingSeconds.value })
)

function goToChoose() {
  view.value = 'choose'
  openSourceUrl.value = ''
  showOpenSourceInput.value = false
}

function backToList() {
  view.value = 'list'
  openSourceUrl.value = ''
}

function showOpenSourceInputStep() {
  showOpenSourceInput.value = true
  openSourceUrl.value = ''
}

async function startBinding(authUrl: string) {
  try {
    await BrowserService.OpenURL(authUrl)
  } catch (error) {
    console.error('Failed to open auth URL:', error)
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }
  view.value = 'binding'
  remainingSeconds.value = BINDING_TIMEOUT_SEC
  startCountdown()
  listenBindingResult()
}

function startCountdown() {
  stopCountdown()
  countdownTimer = setInterval(() => {
    remainingSeconds.value -= 1
    if (remainingSeconds.value <= 0) {
      stopCountdown()
      unbindSuccess?.()
      unbindFailed?.()
      view.value = 'failure'
    }
  }, 1000)
}

function stopCountdown() {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

function listenBindingResult() {
  unbindSuccess = Events.On('chatwiki:binding-success', () => {
    stopCountdown()
    unbindSuccess?.()
    unbindFailed?.()
    view.value = 'success'
  })
  unbindFailed = Events.On('chatwiki:binding-failed', () => {
    stopCountdown()
    unbindSuccess?.()
    unbindFailed?.()
    view.value = 'failure'
  })
}

async function handleLoginCloud() {
  await startBinding(CHATWIKI_CLOUD_AUTH_URL)
}

async function handleGoToAuth() {
  if (!isOpenSourceUrlValid.value) {
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }
  const base = openSourceUrl.value.trim().replace(/\/+$/, '')
  const authUrl = `${base}/oauth/authorize`
  await startBinding(authUrl)
}

function cancelBinding() {
  stopCountdown()
  unbindSuccess?.()
  unbindFailed?.()
  unbindSuccess = null
  unbindFailed = null
  view.value = 'choose'
}

function retry() {
  view.value = 'choose'
  openSourceUrl.value = ''
  showOpenSourceInput.value = false
}

function finishSuccess() {
  view.value = 'list'
}

onUnmounted(() => {
  stopCountdown()
  unbindSuccess?.()
  unbindFailed?.()
})
</script>

<template>
  <!-- List view: card with 新增绑定 -->
  <template v-if="view === 'list'">
    <div
      class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
    >
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold tracking-tight text-foreground">
            {{ t('settings.chatwiki.title') }}
          </h2>
          <Button variant="outline" size="sm" @click="goToChoose">
            {{ t('settings.chatwiki.addBinding') }}
          </Button>
        </div>
        <p class="text-sm text-muted-foreground">
          {{ t('settings.chatwiki.description') }}
        </p>
        <p class="text-sm text-muted-foreground">
          {{ t('settings.chatwiki.notBound') }}
        </p>
      </div>
      <div class="flex flex-col gap-4">
        <div>
          <h3 class="mb-1 text-sm font-medium text-foreground">
            {{ t('settings.chatwiki.applications') }}
          </h3>
          <p class="text-sm text-muted-foreground">
            {{ t('settings.chatwiki.notAuthorized') }}
          </p>
        </div>
        <div>
          <h3 class="mb-1 text-sm font-medium text-foreground">
            {{ t('settings.chatwiki.knowledgeBases') }}
          </h3>
          <p class="text-sm text-muted-foreground">
            {{ t('settings.chatwiki.notAuthorized') }}
          </p>
        </div>
      </div>
    </div>
  </template>

  <!-- Binding flow (sub-pages under 绑定ChatWiki): choose / binding / success / failure -->
  <div
    v-else
    class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
  >
    <!-- Header: brand + description -->
    <div class="flex flex-col gap-2">
      <h1 class="text-xl font-semibold text-foreground">
        {{ t('settings.chatwiki.title') }}
      </h1>
      <p class="text-sm text-muted-foreground">
        {{ t('settings.chatwiki.description') }}
      </p>
    </div>

    <!-- Choose: cloud or open-source -->
    <div v-if="view === 'choose'" class="flex flex-col gap-4">
      <div class="flex flex-col gap-3">
        <Button class="w-full" size="lg" @click="handleLoginCloud">
          {{ t('settings.chatwiki.loginCloud') }}
        </Button>
        <button
          type="button"
          class="text-left text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
          @click="showOpenSourceInputStep"
        >
          {{ t('settings.chatwiki.connectOpenSource') }}
        </button>
      </div>
      <div v-if="showOpenSourceInput" class="space-y-2">
        <Label for="chatwiki-open-source-url">
          {{ t('settings.chatwiki.openSourceUrlLabel') }}
        </Label>
        <Input
          id="chatwiki-open-source-url"
          v-model="openSourceUrl"
          type="url"
          autocomplete="url"
          :placeholder="t('settings.chatwiki.openSourceUrlPlaceholder')"
        />
        <Button
          class="w-full"
          :disabled="!isOpenSourceUrlValid"
          @click="handleGoToAuth"
        >
          {{ t('settings.chatwiki.goToAuth') }}
        </Button>
      </div>
      <Button variant="ghost" class="w-full" @click="backToList">
        {{ t('settings.chatwiki.back') }}
      </Button>
    </div>

    <!-- Binding: loading + countdown + cancel -->
    <div v-else-if="view === 'binding'" class="flex flex-col gap-4">
      <div
        class="flex w-full items-center gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
      >
        <Loader2 class="size-5 shrink-0 animate-spin text-muted-foreground" />
        <span class="text-sm text-foreground">{{ t('settings.chatwiki.loggingIn') }}</span>
      </div>
      <p class="text-sm text-foreground">
        {{ t('settings.chatwiki.waitingAuth') }}
      </p>
      <div class="flex w-full items-center justify-between text-sm">
        <span class="text-muted-foreground">{{ remainingTimeText }}</span>
        <button
          type="button"
          class="text-muted-foreground underline underline-offset-2 hover:text-foreground"
          @click="cancelBinding"
        >
          {{ t('settings.chatwiki.cancel') }}
        </button>
      </div>
    </div>

    <!-- Failure: message + retry -->
    <div v-else-if="view === 'failure'" class="flex flex-col gap-4">
      <div
        class="flex w-full items-start gap-3 rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
      >
        <AlertTriangle class="size-5 shrink-0 text-amber-600 dark:text-amber-500" />
        <div class="min-w-0">
          <p class="text-sm font-medium text-foreground">
            {{ t('settings.chatwiki.authFailed') }}
          </p>
          <p class="text-sm text-muted-foreground">
            {{ t('settings.chatwiki.timeoutReason') }}
          </p>
        </div>
      </div>
      <Button class="w-full" size="lg" @click="retry">
        <RotateCcw class="size-4 shrink-0" />
        {{ t('settings.chatwiki.retry') }}
      </Button>
    </div>

    <!-- Success: message + account + start -->
    <div v-else-if="view === 'success'" class="flex flex-col gap-4">
      <div
        class="flex w-full items-center gap-3 rounded-lg border border-border bg-emerald-500/10 px-4 py-3 dark:border-emerald-500/20 dark:bg-emerald-500/5"
      >
        <CheckCircle2 class="size-5 shrink-0 text-emerald-600 dark:text-emerald-500" />
        <div class="min-w-0">
          <p class="text-sm font-medium text-foreground">
            {{ t('settings.chatwiki.authSuccess') }}
          </p>
          <p class="text-xs text-muted-foreground">
            {{ t('settings.chatwiki.startUsingHint') }}
          </p>
        </div>
      </div>
      <div
        class="flex w-full items-center justify-between rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex size-9 items-center justify-center rounded-full bg-muted text-sm font-medium text-foreground"
          >
            89
          </div>
          <span class="text-sm text-foreground">892438476</span>
        </div>
        <span
          class="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground"
        >
          {{ t('settings.chatwiki.freeVersion') }}
        </span>
      </div>
      <Button class="w-full" size="lg" @click="finishSuccess">
        {{ t('settings.chatwiki.startUsing') }}
      </Button>
      <button
        type="button"
        class="text-sm text-muted-foreground underline underline-offset-2 hover:text-foreground"
        @click="goToChoose"
      >
        {{ t('settings.chatwiki.connectOpenSource') }}
      </button>
    </div>
  </div>
</template>
