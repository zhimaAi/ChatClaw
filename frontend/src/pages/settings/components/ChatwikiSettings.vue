<script setup lang="ts">
/**
 * ChatWiki binding: list view + inline flow (choose → binding → success/failure).
 * On "binding" state, opens browser for auth and starts a 2-min countdown.
 * The Go backend emits "chatwiki:auth-callback" via deep link (chatclaw://auth/callback).
 */
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, CheckCircle2, AlertTriangle, RotateCcw } from 'lucide-vue-next'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { ChatWikiService, type Binding } from '@bindings/chatclaw/internal/services/chatwiki'
import { Events } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

const { t } = useI18n()

const CHATWIKI_CLOUD_AUTH_URL = 'http://dev19.zhima_chat_ai.applnk.cn/'
const BINDING_TIMEOUT_SEC = 120

type View = 'list' | 'choose' | 'binding' | 'success' | 'failure'

interface AuthCallbackData {
  token: string
  ttl: string
  exp: string
  user_id: string
  user_name: string
}

const view = ref<View>('list')
const openSourceUrl = ref('')
const showOpenSourceInput = ref(false)
const remainingSeconds = ref(BINDING_TIMEOUT_SEC)
const authUser = ref<AuthCallbackData | null>(null)
const currentBinding = ref<Binding | null>(null)
let countdownTimer: ReturnType<typeof setInterval> | null = null
let unbindAuthCallback: (() => void) | null = null

const isBound = computed(() => !!currentBinding.value)
const showUnbindConfirm = ref(false)

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

async function loadBinding() {
  try {
    const binding = await ChatWikiService.GetBinding()
    currentBinding.value = binding ?? null
  } catch (error) {
    console.error('Failed to load chatwiki binding:', error)
    currentBinding.value = null
  }
}

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
  listenAuthCallback()
}

function startCountdown() {
  stopCountdown()
  countdownTimer = setInterval(() => {
    remainingSeconds.value -= 1
    if (remainingSeconds.value <= 0) {
      stopCountdown()
      cleanupListeners()
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

function cleanupListeners() {
  unbindAuthCallback?.()
  unbindAuthCallback = null
}

function listenAuthCallback() {
  cleanupListeners()
  unbindAuthCallback = Events.On('chatwiki:auth-callback', (event: any) => {
    stopCountdown()
    cleanupListeners()
    const data: AuthCallbackData = event?.data?.[0] ?? event?.data ?? event
    authUser.value = data
    void loadBinding()
    view.value = 'success'
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
  const authUrl = `${base}/#/chatclaw/login`
  await startBinding(authUrl)
}

function cancelBinding() {
  stopCountdown()
  cleanupListeners()
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

function requestUnbind() {
  showUnbindConfirm.value = true
}

async function confirmUnbind() {
  showUnbindConfirm.value = false
  try {
    await ChatWikiService.DeleteBinding()
    currentBinding.value = null
    authUser.value = null
  } catch (error) {
    console.error('Failed to delete chatwiki binding:', error)
  }
}

onMounted(() => {
  void loadBinding()
})

onUnmounted(() => {
  stopCountdown()
  cleanupListeners()
})
</script>

<template>
  <!-- List view -->
  <template v-if="view === 'list'">
    <div
      class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
    >
      <div class="flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold tracking-tight text-foreground">
            {{ t('settings.chatwiki.title') }}
          </h2>
          <div class="flex items-center gap-2">
            <Button v-if="isBound" variant="outline" size="sm" @click="requestUnbind">
              {{ t('settings.chatwiki.unbind') }}
            </Button>
            <Button v-if="!isBound" variant="outline" size="sm" @click="goToChoose">
              {{ t('settings.chatwiki.addBinding') }}
            </Button>
          </div>
        </div>
        <p class="text-sm text-muted-foreground">
          {{ t('settings.chatwiki.description') }}
        </p>
      </div>

      <!-- Bound: show user info + applications + knowledge bases -->
      <div v-if="isBound && currentBinding" class="flex flex-col gap-4">
        <div
          class="flex items-center justify-between rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
        >
          <div class="flex items-center gap-3">
            <div
              class="flex size-9 items-center justify-center rounded-full bg-muted text-sm font-medium text-foreground"
            >
              {{ currentBinding.user_name?.charAt(0)?.toUpperCase() || '?' }}
            </div>
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-foreground">{{ currentBinding.user_name }}</p>
              <p class="truncate text-xs text-muted-foreground">ID: {{ currentBinding.user_id }}</p>
            </div>
          </div>
          <span class="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
            {{ t('settings.chatwiki.bound') }}
          </span>
        </div>
        <div>
          <h3 class="mb-1 text-sm font-medium text-foreground">
            {{ t('settings.chatwiki.applications') }}
          </h3>
          <div
            class="rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
          >
            <p class="text-sm text-muted-foreground">
              {{ t('settings.chatwiki.notAuthorized') }}
            </p>
          </div>
        </div>
        <div>
          <h3 class="mb-1 text-sm font-medium text-foreground">
            {{ t('settings.chatwiki.knowledgeBases') }}
          </h3>
          <div
            class="rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
          >
            <p class="text-sm text-muted-foreground">
              {{ t('settings.chatwiki.notAuthorized') }}
            </p>
          </div>
        </div>
      </div>

      <!-- Not bound: show placeholder -->
      <div v-else class="flex flex-col gap-4">
        <p class="text-sm text-muted-foreground">
          {{ t('settings.chatwiki.notBound') }}
        </p>
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

  <!-- Binding flow: choose / binding / success / failure -->
  <div
    v-else
    class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
  >
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
        v-if="authUser"
        class="flex w-full items-center justify-between rounded-lg border border-border bg-muted/30 px-4 py-3 dark:border-white/10 dark:bg-white/5"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex size-9 items-center justify-center rounded-full bg-muted text-sm font-medium text-foreground"
          >
            {{ authUser.user_id }}
          </div>
          <div class="min-w-0">
            <p class="truncate text-sm font-medium text-foreground">{{ authUser.user_name }}</p>
            <p class="truncate text-xs text-muted-foreground">{{ authUser.user_id }}</p>
          </div>
        </div>
        <span class="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
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

  <!-- Unbind confirmation dialog -->
  <AlertDialog :open="showUnbindConfirm" @update:open="showUnbindConfirm = $event">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ t('settings.chatwiki.unbindConfirmTitle') }}</AlertDialogTitle>
        <AlertDialogDescription>{{ t('settings.chatwiki.unbindConfirmDesc') }}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel @click="showUnbindConfirm = false">{{ t('common.cancel') }}</AlertDialogCancel>
        <AlertDialogAction
          class="bg-foreground text-background hover:bg-foreground/90"
          @click="confirmUnbind"
        >
          {{ t('common.confirm') }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
