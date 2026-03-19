<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle, ShieldCheck } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { Dialogs } from '@wailsio/runtime'
import { BrowserService } from '@bindings/chatclaw/internal/services/browser'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ChannelService, UpdateChannelInput } from '@bindings/chatclaw/internal/services/channels'
import type { Channel, PlatformMeta } from '@bindings/chatclaw/internal/services/channels'
import { platformIconMap } from '@/assets/icons/snap/platformIcons'
import { getPlatformDocsUrl } from '@/pages/channels/platformDocs'

const props = defineProps<{
  platform?: PlatformMeta | null
  channel?: Channel | null
}>()

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ saved: [channel: Channel, isEdit: boolean] }>()

const { t } = useI18n()

const name = ref('')
const avatar = ref('')
const appId = ref('')
const appSecret = ref('')
const token = ref('')
const saving = ref(false)
const verifying = ref(false)

watch(open, (val) => {
  if (val) {
    if (props.channel) {
      name.value = props.channel.name
      avatar.value = props.channel.avatar
      try {
        const config = JSON.parse(props.channel.extra_config)
        appId.value = config.app_id || config.token || ''
        appSecret.value = config.app_secret || ''
      } catch {
        appId.value = ''
        appSecret.value = ''
      }
    } else {
      name.value = ''
      avatar.value = ''
      appId.value = ''
      appSecret.value = ''
      token.value = ''
    }
  }
})

const currentPlatformId = computed(() => props.platform?.id || props.channel?.platform)

const isWeCom = computed(() => currentPlatformId.value === 'wecom')

const PLATFORM_TIP_ENTRIES: Record<
  string,
  { platformUrl: string }
> = {
  feishu: { platformUrl: 'https://open.feishu.cn/' },
  dingtalk: { platformUrl: 'https://open.dingtalk.com/' },
  qq: { platformUrl: 'https://q.qq.com/#/' },
  wecom: { platformUrl: 'https://work.weixin.qq.com/' },
}

const platformTipConfig = computed(() => {
  const pid = currentPlatformId.value
  const entry = PLATFORM_TIP_ENTRIES[pid]
  if (!entry) return null
  return {
    platformUrl: entry.platformUrl,
    docsUrl: getPlatformDocsUrl(pid),
    prefix: t(`channels.config.${pid}TipPrefix`),
    platformLink: t(`channels.config.${pid}PlatformLink`),
    middle: t(`channels.config.${pid}TipMiddle`),
    guideLink: t(`channels.config.${pid}GuideLink`),
    suffix: t(`channels.config.${pid}TipSuffix`),
  }
})
const appIdLabel = computed(() =>
  isWeCom.value ? t('channels.config.wecomBotId') : t('channels.config.appId')
)
const appSecretLabel = computed(() =>
  isWeCom.value ? t('channels.config.wecomSecret') : t('channels.config.appSecret')
)
const appIdPlaceholder = computed(() =>
  isWeCom.value ? t('channels.config.wecomAppIdPlaceholder') : t('channels.config.appIdPlaceholder')
)
const appSecretPlaceholder = computed(() =>
  isWeCom.value
    ? t('channels.config.wecomAppSecretPlaceholder')
    : t('channels.config.appSecretPlaceholder')
)

const dialogTitle = computed(() => {
  const pid = currentPlatformId.value
  if (!pid) return ''
  const botName = t(`channels.meta.${pid}.botName`, pid)
  if (props.channel) {
    return t('channels.config.editTitle', { platform: botName })
  }
  return t('channels.config.title', { platform: botName })
})

const isFormValid = computed(() => {
  if (!name.value.trim()) return false
  return !!(appId.value.trim() && appSecret.value.trim())
})

const defaultAvatarSrc = computed(() => {
  const pid = currentPlatformId.value
  if (!pid) return null
  return platformIconMap[pid] || null
})

const handlePickIcon = async () => {
  if (saving.value) return
  try {
    const path = await Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: false,
      Title: t('channels.config.pickAvatar'),
      Filters: [
        {
          DisplayName: t('channels.config.filterImages'),
          Pattern: '*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg',
        },
      ],
    })
    if (!path) return
    avatar.value = await AgentsService.ReadIconFile(path)
  } catch (error) {
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to pick icon:', error)
  }
}

async function handleVerify() {
  const pid = currentPlatformId.value
  if (!pid) return
  if (!isFormValid.value) {
    toast.error(t('channels.inline.fillRequired'))
    return
  }
  const extraConfig = JSON.stringify({
    app_id: appId.value.trim(),
    app_secret: appSecret.value.trim(),
  })
  verifying.value = true
  try {
    await ChannelService.VerifyChannelConfig(pid, extraConfig)
    toast.success(t('channels.inline.verifySuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('channels.inline.verifyFailed'))
  } finally {
    verifying.value = false
  }
}

async function handleSave() {
  const pid = currentPlatformId.value
  if (!pid) return
  if (!name.value.trim()) return

  saving.value = true
  try {
    let extraConfig = JSON.stringify({
      app_id: appId.value.trim(),
      app_secret: appSecret.value.trim(),
    })

    let channel: Channel | null = null
    const isEdit = !!props.channel
    if (isEdit) {
      channel = await ChannelService.UpdateChannel(
        props.channel!.id,
        new UpdateChannelInput({
          name: name.value.trim(),
          avatar: avatar.value,
          extra_config: extraConfig,
        })
      )
      toast.success(t('channels.config.editSuccess'))
    } else {
      channel = await ChannelService.CreateChannel({
        platform: pid,
        name: name.value.trim(),
        avatar: avatar.value,
        connection_type: 'gateway',
        extra_config: extraConfig,
      })
      toast.success(t('channels.config.success'))
    }

    open.value = false
    if (channel) emit('saved', channel, isEdit)
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}

async function handleOpenExternalLink(url: string) {
  try {
    await BrowserService.OpenURL(url)
  } catch (error) {
    console.error('Failed to open external link:', error)
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-[480px] gap-0 p-0">
      <DialogHeader class="p-4 pb-0">
        <DialogTitle class="text-xl font-semibold text-[#0a0a0a] dark:text-foreground">
          {{ dialogTitle }}
        </DialogTitle>
      </DialogHeader>

      <form class="px-6 pb-4" @submit.prevent="handleSave">
        <!-- Platform tip card (Feishu / DingTalk / QQ / WeCom) -->
        <div
          v-if="platformTipConfig"
          class="mt-4 rounded-lg border border-border bg-card px-4 py-3"
        >
          <p class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
            {{ platformTipConfig.prefix }}
            <a
              :href="platformTipConfig.platformUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="underline hover:text-primary"
              @click.prevent="handleOpenExternalLink(platformTipConfig.platformUrl)"
              >{{ platformTipConfig.platformLink }}</a
            >
            {{ platformTipConfig.middle }}
            <a
              :href="platformTipConfig.docsUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="underline hover:text-primary"
              @click.prevent="handleOpenExternalLink(platformTipConfig.docsUrl)"
              >{{ platformTipConfig.guideLink }}</a
            >
            {{ platformTipConfig.suffix }}
          </p>
        </div>

        <!-- Avatar upload area -->
        <div class="flex flex-col items-center gap-2 py-6">
          <button
            class="flex h-[62px] w-[62px] items-center justify-center rounded-lg overflow-hidden transition-opacity hover:opacity-80 bg-[#f5f5f5] dark:bg-white/5"
            type="button"
            @click="handlePickIcon"
          >
            <img v-if="avatar" :src="avatar" class="h-full w-full object-cover" />
            <img
              v-else-if="defaultAvatarSrc"
              :src="defaultAvatarSrc"
              class="h-8 w-8 object-contain text-[#0a0a0a] dark:text-foreground"
            />
            <div v-else class="flex h-full w-full items-center justify-center">
              <span class="text-2xl text-[#8c8c8c] dark:text-muted-foreground">+</span>
            </div>
          </button>
          <p class="text-sm text-[#8c8c8c] dark:text-muted-foreground">
            {{ t('channels.config.avatarHint') }}
          </p>
        </div>

        <!-- Name field -->
        <div class="space-y-1">
          <Label
            for="channel-name"
            class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
          >
            <span class="text-[#0a0a0a] dark:text-foreground">*</span>
            {{ t('channels.config.name') }}
          </Label>
          <Input
            id="channel-name"
            v-model="name"
            :placeholder="t('channels.config.namePlaceholder')"
            maxlength="60"
          />
        </div>

        <div class="mt-4 space-y-1">
          <Label
            for="app-id"
            class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
          >
            <span class="text-[#0a0a0a] dark:text-foreground">*</span>
            {{ appIdLabel }}
          </Label>
          <Input id="app-id" v-model="appId" :placeholder="appIdPlaceholder" maxlength="60" />
        </div>
        <div class="mt-4 space-y-1">
          <Label
            for="app-secret"
            class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
          >
            <span class="text-[#0a0a0a] dark:text-foreground">*</span>
            {{ appSecretLabel }}
          </Label>
          <Input
            id="app-secret"
            v-model="appSecret"
            type="password"
            :placeholder="appSecretPlaceholder"
            maxlength="200"
          />
        </div>

        <DialogFooter class="mt-6 pt-4">
          <Button
            type="button"
            variant="outline"
            class="gap-2 bg-[#f5f5f5] text-[#171717] hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
            :disabled="saving || verifying || !isFormValid"
            @click="handleVerify"
          >
            <LoaderCircle v-if="verifying" class="size-4 shrink-0 animate-spin" />
            <ShieldCheck v-else class="size-4 shrink-0" />
            {{ verifying ? t('channels.inline.verifying') : t('channels.inline.verifyConfig') }}
          </Button>
          <Button variant="outline" type="button" @click="open = false">
            {{ t('channels.config.cancel') }}
          </Button>
          <Button type="submit" :disabled="saving || verifying || !isFormValid">
            {{ t('channels.config.save') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
