<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RefreshCw, Plus, Trash2, Wifi, WifiOff, Radio, AlertCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
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
import AddChannelDialog from './components/AddChannelDialog.vue'
import ConfigChannelDialog from './components/ConfigChannelDialog.vue'
import { ChannelService } from '@bindings/chatclaw/internal/services/channels'
import type { Channel, ChannelStats, PlatformMeta } from '@bindings/chatclaw/internal/services/channels'

defineProps<{ tabId: string }>()

const { t } = useI18n()

const channels = ref<Channel[]>([])
const stats = ref<ChannelStats>({ total: 0, connected: 0, disconnected: 0 })
const platforms = ref<PlatformMeta[]>([])
const loading = ref(false)
const addDialogOpen = ref(false)
const configDialogOpen = ref(false)
const selectedPlatform = ref<PlatformMeta | null>(null)
const deleteDialogOpen = ref(false)
const channelToDelete = ref<Channel | null>(null)

const configuredChannels = computed(() => channels.value)

const platformIconMap: Record<string, string> = {
  feishu: '🐦',
  telegram: '✈️',
  discord: '🎮',
  whatsapp: '📱',
  dingtalk: '💬',
}

async function loadData() {
  loading.value = true
  try {
    const [channelList, channelStats, platformList] = await Promise.all([
      ChannelService.ListChannels(),
      ChannelService.GetChannelStats(),
      ChannelService.GetSupportedPlatforms(),
    ])
    channels.value = channelList || []
    stats.value = channelStats || { total: 0, connected: 0, disconnected: 0 }
    platforms.value = platformList || []
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function handleRefresh() {
  try {
    await ChannelService.RefreshChannels()
  } catch {
    // ignore
  }
  await loadData()
}

function handleAddChannel() {
  addDialogOpen.value = true
}

function handleSelectPlatform(platform: PlatformMeta) {
  selectedPlatform.value = platform
  addDialogOpen.value = false
  configDialogOpen.value = true
}

function handleConfigSaved() {
  configDialogOpen.value = false
  selectedPlatform.value = null
  loadData()
}

function confirmDelete(channel: Channel) {
  channelToDelete.value = channel
  deleteDialogOpen.value = true
}

async function handleDelete() {
  if (!channelToDelete.value) return
  try {
    await ChannelService.DeleteChannel(channelToDelete.value.id)
    toast.success(t('channels.delete.success'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    deleteDialogOpen.value = false
    channelToDelete.value = null
  }
}

async function handleConnect(channel: Channel) {
  try {
    await ChannelService.ConnectChannel(channel.id)
    toast.success(t('channels.connect.success'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

async function handleDisconnect(channel: Channel) {
  try {
    await ChannelService.DisconnectChannel(channel.id)
    toast.success(t('channels.disconnect.success'))
    loadData()
  } catch (error) {
    toast.error(getErrorMessage(error))
  }
}

function getPlatformIcon(platformId: string): string {
  return platformIconMap[platformId] || '🤖'
}

function getPlatformName(platformId: string): string {
  const key = `channels.meta.${platformId}.name`
  const translated = t(key)
  return translated !== key ? translated : platformId
}

function isConfigured(platformId: string): boolean {
  return channels.value.some((ch) => ch.platform === platformId)
}

onMounted(loadData)
</script>

<template>
  <div class="flex h-full flex-col overflow-y-auto">
    <div class="mx-auto w-full max-w-5xl px-8 py-6">
      <!-- Header -->
      <div class="mb-6 flex items-start justify-between">
        <div>
          <h1 class="text-2xl font-bold text-foreground">{{ t('channels.title') }}</h1>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('channels.subtitle') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="sm" :disabled="loading" @click="handleRefresh">
            <RefreshCw :class="['mr-1.5 size-3.5', loading && 'animate-spin']" />
            {{ t('channels.refresh') }}
          </Button>
          <Button size="sm" @click="handleAddChannel">
            <Plus class="mr-1.5 size-3.5" />
            {{ t('channels.addChannel') }}
          </Button>
        </div>
      </div>

      <!-- Stats Cards -->
      <div class="mb-8 grid grid-cols-3 gap-4">
        <div class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10">
          <div class="flex items-center gap-3">
            <div class="flex size-10 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Radio class="size-5" />
            </div>
            <div>
              <p class="text-2xl font-bold text-foreground">{{ stats.total }}</p>
              <p class="text-xs text-muted-foreground">{{ t('channels.stats.total') }}</p>
            </div>
          </div>
        </div>
        <div class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10">
          <div class="flex items-center gap-3">
            <div class="flex size-10 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-500">
              <Wifi class="size-5" />
            </div>
            <div>
              <p class="text-2xl font-bold text-foreground">{{ stats.connected }}</p>
              <p class="text-xs text-muted-foreground">{{ t('channels.stats.connected') }}</p>
            </div>
          </div>
        </div>
        <div class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10">
          <div class="flex items-center gap-3">
            <div class="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
              <WifiOff class="size-5" />
            </div>
            <div>
              <p class="text-2xl font-bold text-foreground">{{ stats.disconnected }}</p>
              <p class="text-xs text-muted-foreground">{{ t('channels.stats.disconnected') }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Configured Channels Section -->
      <div v-if="configuredChannels.length > 0" class="mb-8">
        <div class="mb-3">
          <h2 class="text-lg font-semibold text-foreground">{{ t('channels.configured.title') }}</h2>
          <p class="text-xs text-muted-foreground">{{ t('channels.configured.desc') }}</p>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <div
            v-for="channel in configuredChannels"
            :key="channel.id"
            class="group relative rounded-lg border border-border bg-card p-4 shadow-sm transition-colors hover:border-primary/30 dark:shadow-none dark:ring-1 dark:ring-white/10"
          >
            <div class="flex items-center gap-3">
              <span class="text-2xl">{{ getPlatformIcon(channel.platform) }}</span>
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium text-foreground">{{ channel.name }}</p>
                <p class="text-xs text-muted-foreground">{{ getPlatformName(channel.platform) }}</p>
              </div>
              <Badge
                v-if="channel.status === 'online'"
                variant="outline"
                class="shrink-0 border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
              >
                <span class="mr-1 inline-block size-1.5 rounded-full bg-emerald-500" />
                {{ t('channels.configured.connected') }}
              </Badge>
              <Badge
                v-else-if="channel.status === 'error'"
                variant="outline"
                class="shrink-0 border-destructive/30 bg-destructive/10 text-destructive"
              >
                <AlertCircle class="mr-1 size-3" />
                {{ t('channels.configured.error') }}
              </Badge>
              <Badge
                v-else
                variant="outline"
                class="shrink-0 text-muted-foreground"
              >
                {{ t('channels.configured.disconnected') }}
              </Badge>
            </div>
            <div class="mt-3 flex items-center gap-2">
              <Button
                v-if="channel.status !== 'online'"
                variant="outline"
                size="sm"
                class="h-7 text-xs"
                @click="handleConnect(channel)"
              >
                <Wifi class="mr-1 size-3" />
                {{ t('channels.configured.connected') }}
              </Button>
              <Button
                v-else
                variant="outline"
                size="sm"
                class="h-7 text-xs"
                @click="handleDisconnect(channel)"
              >
                <WifiOff class="mr-1 size-3" />
                {{ t('channels.configured.disconnected') }}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                class="h-7 text-xs text-destructive hover:bg-destructive/10 hover:text-destructive"
                @click="confirmDelete(channel)"
              >
                <Trash2 class="size-3.5" />
              </Button>
            </div>
          </div>
        </div>
      </div>

      <!-- Available Channels Section -->
      <div>
        <div class="mb-3">
          <h2 class="text-lg font-semibold text-foreground">{{ t('channels.available.title') }}</h2>
          <p class="text-xs text-muted-foreground">{{ t('channels.available.desc') }}</p>
        </div>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          <button
            v-for="platform in platforms"
            :key="platform.id"
            class="group flex flex-col items-start gap-2 rounded-lg border border-border bg-card p-4 text-left shadow-sm transition-all hover:border-primary/40 hover:shadow-md dark:shadow-none dark:ring-1 dark:ring-white/10 dark:hover:ring-primary/40"
            :class="isConfigured(platform.id) && 'border-primary/30 ring-1 ring-primary/20'"
            @click="handleSelectPlatform(platform)"
          >
            <span class="text-2xl">{{ getPlatformIcon(platform.id) }}</span>
            <div>
              <p class="text-sm font-medium text-foreground">{{ getPlatformName(platform.id) }}</p>
              <p class="text-xs text-muted-foreground">
                {{ t(`channels.meta.${platform.id}.description`) }}
              </p>
            </div>
            <Badge
              v-if="isConfigured(platform.id)"
              variant="outline"
              class="border-primary/30 bg-primary/10 text-primary"
            >
              {{ t('channels.configured.title') }}
            </Badge>
          </button>
        </div>
      </div>
    </div>

    <!-- Add Channel Dialog (platform selection) -->
    <AddChannelDialog
      v-model:open="addDialogOpen"
      :platforms="platforms"
      @select="handleSelectPlatform"
    />

    <!-- Config Channel Dialog (platform-specific config form) -->
    <ConfigChannelDialog
      v-model:open="configDialogOpen"
      :platform="selectedPlatform"
      @saved="handleConfigSaved"
    />

    <!-- Delete Confirmation -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('channels.delete.title') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('channels.delete.desc', { name: channelToDelete?.name }) }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            @click="handleDelete"
          >
            {{ t('common.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
