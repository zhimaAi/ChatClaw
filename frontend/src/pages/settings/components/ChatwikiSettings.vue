<script setup lang="ts">
/**
 * ChatWiki binding: list view + inline flow (choose → binding → success/failure).
 * On "binding" state, opens browser for auth and starts a 2-min countdown.
 * The Go backend emits "chatwiki:auth-callback" via deep link (chatclaw://auth/callback).
 */
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, CheckCircle2, AlertTriangle, RotateCcw, RefreshCw } from 'lucide-vue-next'
import { Call } from '@wailsio/runtime'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { ChatWikiService, type Binding } from '@bindings/chatclaw/internal/services/chatwiki'
import { ProvidersService } from '@bindings/chatclaw/internal/services/providers'
import {
  getBinding as getBindingCached,
  getRobotListAll as getRobotListAllCached,
  getLibraryList as getLibraryListCached,
  clearAll as clearChatwikiCache,
} from '@/lib/chatwikiCache'
import { buildChatWikiLoginUrl, openChatWikiCloudLogin } from '@/lib/chatwikiAuth'
import { notifyChatwikiBindingChanged } from '@/lib/chatwikiBindingState'
import { useSettingsStore } from '@/stores/settings'
import { Events } from '@wailsio/runtime'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import { isChatWikiAuthExpiredError } from '@/composables/useErrorMessage'
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
const settingsStore = useSettingsStore()

const BINDING_TIMEOUT_SEC = 120
/** Cloud URL loaded from backend on mount (respects dev/prod build config) */
const cloudAuthUrl = ref('')

type View = 'list' | 'choose' | 'binding' | 'success' | 'failure'

interface AuthCallbackData {
  server_url: string
  token: string
  ttl: string
  exp: string
  user_id: string
  user_name: string
  chatwiki_version: string
}

type LoginSource = '' | 'cloud' | 'open-source'

interface RobotItem {
  id: string
  robot_key: string
  name: string
  intro: string
  type: string
  icon: string
  chat_claw_switch_status: number
  application_type: string
  enabled: boolean
}

interface LibraryItem {
  id: string
  name: string
  intro: string
  type: string
  type_name: string
  chat_claw_switch_status: number
  enabled: boolean
}

const view = ref<View>('list')
const openSourceUrl = ref('')
const showOpenSourceInput = ref(false)
/** True when we entered binding view via re-auth (so cancel returns to list, not choose) */
const isReauthFlow = ref(false)
const remainingSeconds = ref(BINDING_TIMEOUT_SEC)
const authUser = ref<AuthCallbackData | null>(null)
const currentBinding = ref<Binding | null>(null)
const pendingLoginSource = ref<LoginSource>('')
let countdownTimer: ReturnType<typeof setInterval> | null = null
let unbindAuthCallback: (() => void) | null = null

const isBound = computed(() => !!currentBinding.value)
/** exp is Unix timestamp in seconds; binding is expired when exp <= now */
const bindingExpired = computed(() => {
  const b = currentBinding.value
  if (!b || b.exp == null) return false
  const exp = Number(b.exp)
  return exp <= Math.floor(Date.now() / 1000)
})
const showUnbindConfirm = ref(false)

const robots = ref<RobotItem[]>([])
const robotsLoading = ref(false)

const libraryTab = ref('0')
const libraries = ref<LibraryItem[]>([])
const librariesLoading = ref(false)

const syncingRobots = ref(false)
const syncingLibraries = ref(false)
/** Loading when user clicks "开始使用" to sync and go back to list */
const finishSuccessLoading = ref(false)

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
    const binding = await getBindingCached()
    currentBinding.value = binding ?? null
  } catch (error) {
    console.error('Failed to load chatwiki binding:', error)
    currentBinding.value = null
  }
}

async function loadRobots() {
  if (!isBound.value) return
  robotsLoading.value = true
  try {
    console.log('[ChatWiki] Loading robot list...')
    const list = await getRobotListAllCached()
    console.log('[ChatWiki] Robot list response:', list)
    robots.value = (list ?? []).map((r: any) => ({
      ...r,
      enabled: Number(r?.chat_claw_switch_status) === 1,
    }))
  } catch (error) {
    console.error('[ChatWiki] Failed to load robots:', error)
    if (isChatWikiAuthExpiredError(error)) {
      clearChatwikiCache()
      await loadBinding()
    }
    robots.value = []
    throw error
  } finally {
    robotsLoading.value = false
  }
}

async function loadLibraries(type: number = 0) {
  if (!isBound.value) return
  librariesLoading.value = true
  try {
    console.log('[ChatWiki] Loading library list, type:', type)
    const list = await getLibraryListCached(type)
    console.log('[ChatWiki] Library list response:', list)
    libraries.value = (list ?? []).map((l: any) => ({
      ...l,
      enabled: Number(l?.chat_claw_switch_status) === 1,
    }))
  } catch (error) {
    console.error('[ChatWiki] Failed to load libraries:', error)
    if (isChatWikiAuthExpiredError(error)) {
      clearChatwikiCache()
      await loadBinding()
    }
    libraries.value = []
    throw error
  } finally {
    librariesLoading.value = false
  }
}

async function syncRobots(options?: { silent?: boolean }) {
  syncingRobots.value = true
  try {
    clearChatwikiCache()
    await loadRobots()
    if (!options?.silent) toast.success(t('settings.chatwiki.syncSuccess'))
  } catch (error) {
    if (!options?.silent) {
      if (isChatWikiAuthExpiredError(error)) {
        toast.error(t('settings.chatwiki.authExpiredPleaseReauth'))
      } else {
        toast.error(t('settings.chatwiki.syncFailed'))
      }
    }
    throw error
  } finally {
    syncingRobots.value = false
  }
}

async function syncLibraries(options?: { silent?: boolean }) {
  syncingLibraries.value = true
  try {
    clearChatwikiCache()
    await loadLibraries(Number(libraryTab.value))
    if (!options?.silent) toast.success(t('settings.chatwiki.syncSuccess'))
  } catch (error) {
    if (!options?.silent) {
      if (isChatWikiAuthExpiredError(error)) {
        toast.error(t('settings.chatwiki.authExpiredPleaseReauth'))
      } else {
        toast.error(t('settings.chatwiki.syncFailed'))
      }
    }
    throw error
  } finally {
    syncingLibraries.value = false
  }
}

watch(libraryTab, (newType) => {
  void loadLibraries(Number(newType))
})

function getRobotTypeLabel(type: string): string {
  const typeMap: Record<string, string> = {
    chat: t('settings.chatwiki.robotType.chat'),
    workflow: t('settings.chatwiki.robotType.workflow'),
  }
  return typeMap[type] || type
}

function onRobotAvatarError(robot: RobotItem, event: Event) {
  console.error('[ChatWiki] Robot avatar failed to load', {
    robotId: robot.id,
    robotName: robot.name,
    icon: robot.icon,
    event,
  })
}

async function onRobotSwitchChange(robot: RobotItem, checked: boolean | 'indeterminate') {
  const nextEnabled = checked === true
  const previousEnabled = robot.enabled
  const previousStatus = robot.chat_claw_switch_status
  robot.enabled = nextEnabled
  robot.chat_claw_switch_status = nextEnabled ? 1 : 0
  try {
    await ChatWikiService.UpdateRobotSwitchStatus(robot.id, robot.chat_claw_switch_status)
    clearChatwikiCache()
  } catch (error) {
    robot.enabled = previousEnabled
    robot.chat_claw_switch_status = previousStatus
    console.error('[ChatWiki] Failed to update robot switch status:', error)
    toast.error(t('settings.chatwiki.switchUpdateFailed'))
  }
}

async function onLibrarySwitchChange(lib: LibraryItem, checked: boolean | 'indeterminate') {
  const nextEnabled = checked === true
  const previousEnabled = lib.enabled
  const previousStatus = lib.chat_claw_switch_status
  lib.enabled = nextEnabled
  lib.chat_claw_switch_status = nextEnabled ? 1 : 0
  try {
    await ChatWikiService.UpdateLibrarySwitchStatus(lib.id, lib.chat_claw_switch_status)
    clearChatwikiCache()
  } catch (error) {
    lib.enabled = previousEnabled
    lib.chat_claw_switch_status = previousStatus
    console.error('[ChatWiki] Failed to update library switch status:', error)
    toast.error(t('settings.chatwiki.switchUpdateFailed'))
  }
}

function goToChoose() {
  view.value = 'choose'
  openSourceUrl.value = ''
  showOpenSourceInput.value = false
}

/** Re-auth: open current binding server_url login page directly (no choose step) */
async function startReauthBinding() {
  const b = currentBinding.value
  const base = (b?.server_url ?? '').trim().replace(/\/+$/, '')
  if (!base) {
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }
  isReauthFlow.value = true
  pendingLoginSource.value = b?.chatwiki_version === 'yun' ? 'cloud' : 'open-source'
  const authUrl = buildChatWikiLoginUrl(base, await BrowserService.GetLoginParams().catch(() => undefined))
  await startBinding(authUrl)
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
    pendingLoginSource.value = ''
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
  unbindAuthCallback = Events.On('chatwiki:auth-callback', async (event: any) => {
    stopCountdown()
    cleanupListeners()
    const data: AuthCallbackData = event?.data?.[0] ?? event?.data ?? event
    try {
      await saveBindingFromCallback(data)
      authUser.value = data
      await loadBinding()
      view.value = 'success'
    } catch (error) {
      console.error('Failed to save chatwiki binding from auth callback:', error)
      toast.error(t('settings.chatwiki.authFailed'))
      view.value = 'failure'
    } finally {
      pendingLoginSource.value = ''
    }
  })
}

async function handleLoginCloud() {
  isReauthFlow.value = false
  pendingLoginSource.value = 'cloud'
  const base = cloudAuthUrl.value.replace(/\/+$/, '')
  if (!base) {
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }
  try {
    await openChatWikiCloudLogin(base)
  } catch (error) {
    console.error('Failed to open auth URL:', error)
    pendingLoginSource.value = ''
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }
  view.value = 'binding'
  remainingSeconds.value = BINDING_TIMEOUT_SEC
  startCountdown()
  listenAuthCallback()
}

async function saveBindingFromCallback(data: AuthCallbackData) {
  const methodNames = [
    'chatclaw/internal/services/chatwiki.ChatWikiService.SaveBindingFromCallback',
    'ChatWikiService.SaveBindingFromCallback',
    'SaveBindingFromCallback',
  ]
  let lastError: unknown = null

  for (const methodName of methodNames) {
    try {
      await Call.ByName(
        methodName,
        data.server_url,
        data.token,
        data.ttl,
        data.exp,
        data.user_id,
        data.user_name,
        pendingLoginSource.value
      )
      return
    } catch (error) {
      lastError = error
    }
  }

  throw lastError
}

async function handleGoToAuth() {
  if (!isOpenSourceUrlValid.value) {
    toast.error(t('settings.chatwiki.invalidUrl'))
    return
  }
  isReauthFlow.value = false
  pendingLoginSource.value = 'open-source'
  const base = openSourceUrl.value.trim().replace(/\/+$/, '')
  const authUrl = buildChatWikiLoginUrl(base, await BrowserService.GetLoginParams().catch(() => undefined))
  await startBinding(authUrl)
}

function cancelBinding() {
  stopCountdown()
  cleanupListeners()
  pendingLoginSource.value = ''
  // Re-auth flow: go back to list; new binding flow: go back to choose
  view.value = isReauthFlow.value ? 'list' : 'choose'
  isReauthFlow.value = false
}

function retry() {
  isReauthFlow.value = false
  pendingLoginSource.value = ''
  view.value = 'choose'
  openSourceUrl.value = ''
  showOpenSourceInput.value = false
}

async function finishSuccess() {
  finishSuccessLoading.value = true
  try {
    // Sync apps and knowledge bases after (re-)binding so list view has fresh data
    await Promise.all([syncRobots({ silent: true }), syncLibraries({ silent: true })])
    await ProvidersService.GetProviderWithModels('chatwiki')
    // Reload binding so list view shows latest auth status (bound/expired, user info)
    await loadBinding()
    notifyChatwikiBindingChanged(currentBinding.value)
    Events.Emit('models:changed')
    toast.success(t('settings.chatwiki.syncSuccess'))
    view.value = 'list'
  } catch (error) {
    if (isChatWikiAuthExpiredError(error)) {
      toast.error(t('settings.chatwiki.authExpiredPleaseReauth'))
    } else {
      toast.error(t('settings.chatwiki.syncFailed'))
    }
  } finally {
    finishSuccessLoading.value = false
  }
}

function requestUnbind() {
  showUnbindConfirm.value = true
}

async function confirmUnbind() {
  showUnbindConfirm.value = false
  try {
    await ChatWikiService.TokenForceOffline()
    await ChatWikiService.DeleteBinding()
    clearChatwikiCache()
    currentBinding.value = null
    authUser.value = null
    robots.value = []
    libraries.value = []
    notifyChatwikiBindingChanged(null)
    Events.Emit('models:changed')
  } catch (error) {
    console.error('Failed to delete chatwiki binding:', error)
  }
}

watch(isBound, (bound) => {
  if (bound) {
    void loadRobots().catch((err) => {
      if (isChatWikiAuthExpiredError(err)) {
        toast.error(t('settings.chatwiki.authExpiredPleaseReauth'))
      }
    })
    void loadLibraries(Number(libraryTab.value)).catch((err) => {
      if (isChatWikiAuthExpiredError(err)) {
        toast.error(t('settings.chatwiki.authExpiredPleaseReauth'))
      }
    })
  }
})

onMounted(() => {
  clearChatwikiCache()
  void loadBinding()
  void ChatWikiService.GetCloudURL().then((url) => {
    cloudAuthUrl.value = url ?? ''
    if (settingsStore.consumePendingChatwikiAction() === 'cloudLogin') {
      void handleLoginCloud()
    }
  })
})

onUnmounted(() => {
  stopCountdown()
  cleanupListeners()
})
</script>

<template>
  <!-- List view: main ChatWiki card + Applications card + Knowledge bases card -->
  <template v-if="view === 'list'">
    <div class="flex flex-col gap-6">
      <!-- Main ChatWiki card: binding info only -->
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

        <!-- Bound: user info only -->
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
                <p class="truncate text-sm font-medium text-foreground">
                  {{ currentBinding.user_name }}
                </p>
                <p class="truncate text-xs text-muted-foreground">
                  ID: {{ currentBinding.user_id }}
                </p>
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <span
                v-if="bindingExpired"
                class="rounded-md border border-destructive/50 bg-destructive/10 px-2 py-1 text-xs text-destructive"
              >
                {{ t('settings.chatwiki.bindingExpired') }}
              </span>
              <span v-else class="rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
                {{ t('settings.chatwiki.bound') }}
              </span>
              <Button v-if="bindingExpired" size="sm" @click="startReauthBinding">
                {{ t('settings.chatwiki.reauthBind') }}
              </Button>
            </div>
          </div>
        </div>

        <!-- Not bound -->
        <div v-else>
          <p class="text-sm text-muted-foreground">
            {{ t('settings.chatwiki.notBound') }}
          </p>
        </div>
      </div>

      <!-- Applications card -->
      <div
        class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
      >
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold tracking-tight text-foreground">
            {{ t('settings.chatwiki.applications') }}
          </h2>
          <Button
            v-if="isBound"
            variant="outline"
            size="sm"
            class="gap-1"
            :disabled="syncingRobots"
            @click="syncRobots"
          >
            <RefreshCw class="size-3.5 shrink-0" :class="{ 'animate-spin': syncingRobots }" />
            {{ syncingRobots ? t('settings.chatwiki.syncing') : t('settings.chatwiki.sync') }}
          </Button>
        </div>
        <div v-if="!isBound" class="text-sm text-muted-foreground">
          {{ t('settings.chatwiki.notAuthorized') }}
        </div>
        <template v-else>
          <div v-if="robotsLoading" class="flex items-center justify-center py-6">
            <Loader2 class="size-5 animate-spin text-muted-foreground" />
          </div>
          <div v-else-if="robots.length === 0" class="text-sm text-muted-foreground">
            {{ t('settings.chatwiki.emptyRobots') }}
          </div>
          <div
            v-else
            class="flex flex-col rounded-lg border border-border bg-muted/30 dark:border-white/10 dark:bg-white/5"
          >
            <div
              v-for="robot in robots"
              :key="robot.id"
              class="flex items-center justify-between border-t border-border px-4 py-3 first:border-t-0 dark:border-white/10"
            >
              <div class="flex items-center gap-3 overflow-hidden">
                <div
                  class="flex size-8 shrink-0 items-center justify-center overflow-hidden rounded bg-muted"
                >
                  <img
                    v-if="robot.icon"
                    :src="robot.icon"
                    :alt="robot.name"
                    class="size-full object-cover"
                    @error="onRobotAvatarError(robot, $event)"
                  />
                  <span v-else class="text-xs text-muted-foreground">{{
                    robot.name?.charAt(0) || '?'
                  }}</span>
                </div>
                <div class="min-w-0">
                  <p class="truncate text-sm text-foreground">{{ robot.name }}</p>
                </div>
                <span
                  class="shrink-0 rounded border border-border px-1.5 py-0.5 text-xs text-muted-foreground"
                >
                  {{ getRobotTypeLabel(robot.type) }}
                </span>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <Switch
                  :model-value="robot.enabled"
                  @update:model-value="(checked) => onRobotSwitchChange(robot, checked)"
                />
              </div>
            </div>
          </div>
        </template>
      </div>

      <!-- Knowledge bases card -->
      <div
        class="flex w-settings-card flex-col gap-6 rounded-2xl border border-border bg-card p-8 shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
      >
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold tracking-tight text-foreground">
            {{ t('settings.chatwiki.knowledgeBases') }}
          </h2>
          <Button
            v-if="isBound"
            variant="outline"
            size="sm"
            class="gap-1"
            :disabled="syncingLibraries"
            @click="syncLibraries"
          >
            <RefreshCw class="size-3.5 shrink-0" :class="{ 'animate-spin': syncingLibraries }" />
            {{ syncingLibraries ? t('settings.chatwiki.syncing') : t('settings.chatwiki.sync') }}
          </Button>
        </div>
        <div v-if="!isBound" class="text-sm text-muted-foreground">
          {{ t('settings.chatwiki.notAuthorized') }}
        </div>
        <template v-else>
          <Tabs v-model="libraryTab" class="w-full">
            <TabsList class="w-auto">
              <TabsTrigger value="0">{{ t('settings.chatwiki.libraryType.normal') }}</TabsTrigger>
              <TabsTrigger value="2">{{ t('settings.chatwiki.libraryType.qa') }}</TabsTrigger>
              <TabsTrigger value="3">{{ t('settings.chatwiki.libraryType.wechat') }}</TabsTrigger>
            </TabsList>
          </Tabs>
          <div v-if="librariesLoading" class="flex items-center justify-center py-6">
            <Loader2 class="size-5 animate-spin text-muted-foreground" />
          </div>
          <div v-else-if="libraries.length === 0" class="text-sm text-muted-foreground">
            {{ t('settings.chatwiki.emptyLibraries') }}
          </div>
          <div
            v-else
            class="flex flex-col rounded-lg border border-border bg-muted/30 dark:border-white/10 dark:bg-white/5"
          >
            <div
              v-for="lib in libraries"
              :key="lib.id"
              class="flex items-center justify-between border-t border-border px-4 py-3 first:border-t-0 dark:border-white/10"
            >
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm text-foreground">{{ lib.name }}</p>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <Switch
                  :model-value="lib.enabled"
                  @update:model-value="(checked) => onLibrarySwitchChange(lib, checked)"
                />
              </div>
            </div>
          </div>
        </template>
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
        <Button class="w-full" :disabled="!isOpenSourceUrlValid" @click="handleGoToAuth">
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
      <Button class="w-full" size="lg" :disabled="finishSuccessLoading" @click="finishSuccess">
        <Loader2 v-if="finishSuccessLoading" class="size-4 shrink-0 animate-spin" />
        {{
          finishSuccessLoading ? t('settings.chatwiki.syncing') : t('settings.chatwiki.startUsing')
        }}
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
        <AlertDialogDescription>{{
          t('settings.chatwiki.unbindConfirmDesc')
        }}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel @click="showUnbindConfirm = false">{{
          t('common.cancel')
        }}</AlertDialogCancel>
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
