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
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ChannelService } from '@bindings/chatclaw/internal/services/channels'
import type { Channel, PlatformMeta } from '@bindings/chatclaw/internal/services/channels'
import { platformIconMap } from '@/assets/icons/snap/platformIcons'

const props = defineProps<{
  platform: PlatformMeta | null
}>()

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ saved: [channel: Channel] }>()

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
    name.value = ''
    avatar.value = ''
    appId.value = ''
    appSecret.value = ''
    token.value = ''
  }
})

const isFeishu = computed(() => props.platform?.id === 'feishu')
const isWeCom = computed(() => props.platform?.id === 'wecom')
const appIdLabel = computed(() => (isWeCom.value ? 'Bot ID' : t('channels.config.appId')))
const appSecretLabel = computed(() => (isWeCom.value ? 'Secret' : t('channels.config.appSecret')))
const appIdPlaceholder = computed(() => (
  isWeCom.value
    ? t('channels.config.wecomAppIdPlaceholder')
    : t('channels.config.appIdPlaceholder')
))
const appSecretPlaceholder = computed(() => (
  isWeCom.value
    ? t('channels.config.wecomAppSecretPlaceholder')
    : t('channels.config.appSecretPlaceholder')
))

const dialogTitle = computed(() => {
  if (!props.platform) return ''
  const botName = t(`channels.meta.${props.platform.id}.botName`, props.platform.id)
  return t('channels.config.title', { platform: botName })
})

const isFormValid = computed(() => {
  if (!name.value.trim()) return false
  return !!(appId.value.trim() && appSecret.value.trim())
})

const defaultAvatarSrc = computed(() => {
  if (!props.platform) return null
  return platformIconMap[props.platform.id] || null
})

const handlePickIcon = async () => {
  if (saving.value) return
  try {
    const path = await Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: false,
      Title: t('channels.config.pickAvatar', '选择头像'),
      Filters: [
        {
          DisplayName: t('channels.config.filterImages', '图片文件'),
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
  if (!props.platform) return
  if (!isFormValid.value) {
    toast.error(t('channels.inline.fillRequired', '请先填写必填项'))
    return
  }
  const extraConfig = JSON.stringify({
    app_id: appId.value.trim(),
    app_secret: appSecret.value.trim(),
  })
  verifying.value = true
  try {
    await ChannelService.VerifyChannelConfig(props.platform.id, extraConfig)
    toast.success(t('channels.inline.verifySuccess', '验证通过'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('channels.inline.verifyFailed', '验证失败'))
  } finally {
    verifying.value = false
  }
}

async function handleSave() {
  if (!props.platform) return
  if (!name.value.trim()) return

  saving.value = true
  try {
    let extraConfig = JSON.stringify({
      app_id: appId.value.trim(),
      app_secret: appSecret.value.trim(),
    })

    const channel = await ChannelService.CreateChannel({
      platform: props.platform.id,
      name: name.value.trim(),
      avatar: avatar.value,
      connection_type: 'gateway',
      extra_config: extraConfig,
    })

    toast.success(t('channels.config.success'))
    open.value = false
    if (channel) emit('saved', channel)
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    saving.value = false
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
        <!-- Feishu tip card -->
        <div
          v-if="isFeishu"
          class="mt-4 rounded-lg border border-border bg-card px-4 py-3"
        >
          <p class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
            {{ t('channels.config.feishuTipPrefix') }}
            <a
              href="https://open.feishu.cn/"
              target="_blank"
              class="underline hover:text-primary"
            >{{ t('channels.config.feishuPlatformLink') }}</a>
            {{ t('channels.config.feishuTipMiddle') }}
            <a
              href="https://www.feishu.cn/hc/zh-CN/articles/360024984973"
              target="_blank"
              class="underline hover:text-primary"
            >{{ t('channels.config.feishuGuideLink') }}</a>
            {{ t('channels.config.feishuTipSuffix') }}
          </p>
        </div>

        <!-- Avatar upload area -->
        <div class="flex flex-col items-center gap-2 py-6">
          <button
            class="flex h-[62px] w-[62px] items-center justify-center rounded-lg overflow-hidden transition-opacity hover:opacity-80 bg-[#f5f5f5] dark:bg-white/5"
            type="button"
            @click="handlePickIcon"
          >
            <img
              v-if="avatar"
              :src="avatar"
              class="h-full w-full object-cover"
            />
            <img
              v-else-if="defaultAvatarSrc"
              :src="defaultAvatarSrc"
              class="h-8 w-8 object-contain text-[#0a0a0a] dark:text-foreground"
            />
            <div
              v-else
              class="flex h-full w-full items-center justify-center"
            >
              <span class="text-2xl text-[#8c8c8c] dark:text-muted-foreground">+</span>
            </div>
          </button>
          <p class="text-sm text-[#8c8c8c] dark:text-muted-foreground">
            {{ t('channels.config.avatarHint') }}
          </p>
        </div>

        <!-- Name field -->
        <div class="space-y-1">
          <Label for="channel-name" class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground">
            <span class="text-[#0a0a0a] dark:text-foreground">*</span>
            {{ t('channels.config.name') }}
          </Label>
          <Input
            id="channel-name"
            v-model="name"
            :placeholder="t('channels.config.namePlaceholder')"
          />
        </div>

        <div class="mt-4 space-y-1">
          <Label for="app-id" class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground">
            <span class="text-[#0a0a0a] dark:text-foreground">*</span>
            {{ appIdLabel }}
          </Label>
          <Input
            id="app-id"
            v-model="appId"
            :placeholder="appIdPlaceholder"
          />
        </div>
        <div class="mt-4 space-y-1">
          <Label for="app-secret" class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground">
            <span class="text-[#0a0a0a] dark:text-foreground">*</span>
            {{ appSecretLabel }}
          </Label>
          <Input
            id="app-secret"
            v-model="appSecret"
            type="password"
            :placeholder="appSecretPlaceholder"
          />
        </div>

        <DialogFooter class="mt-6 pt-4">
          <Button
            type="button"
            variant="outline"
            class="bg-[#f5f5f5] text-[#171717] hover:bg-[#e5e5e5] dark:bg-muted dark:text-foreground dark:hover:bg-muted/80"
            :disabled="saving || verifying || !isFormValid"
            @click="handleVerify"
          >
            <LoaderCircle v-if="verifying" class="mr-2 size-4 animate-spin" />
            <ShieldCheck v-else class="mr-2 size-4" />
            {{ verifying ? t('channels.inline.verifying', '验证中…') : t('channels.inline.verifyConfig', '验证配置') }}
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
