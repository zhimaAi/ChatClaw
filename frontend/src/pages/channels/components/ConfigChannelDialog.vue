<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ChannelService } from '@bindings/chatclaw/internal/services/channels'
import type { PlatformMeta } from '@bindings/chatclaw/internal/services/channels'

const props = defineProps<{
  platform: PlatformMeta | null
}>()

const open = defineModel<boolean>('open', { required: true })
const emit = defineEmits<{ saved: [] }>()

const { t } = useI18n()

const name = ref('')
const appId = ref('')
const appSecret = ref('')
const token = ref('')
const saving = ref(false)

watch(open, (val) => {
  if (val) {
    name.value = ''
    appId.value = ''
    appSecret.value = ''
    token.value = ''
  }
})

const isFeishu = () => props.platform?.id === 'feishu'

async function handleSave() {
  if (!props.platform) return
  if (!name.value.trim()) return

  saving.value = true
  try {
    let extraConfig = '{}'

    if (isFeishu()) {
      extraConfig = JSON.stringify({
        app_id: appId.value.trim(),
        app_secret: appSecret.value.trim(),
      })
    } else {
      extraConfig = JSON.stringify({ token: token.value.trim() })
    }

    await ChannelService.CreateChannel({
      platform: props.platform.id,
      name: name.value.trim(),
      connection_type: 'gateway',
      extra_config: extraConfig,
    })

    toast.success(t('channels.config.success'))
    emit('saved')
    open.value = false
  } catch (error) {
    toast.error(getErrorMessage(error))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{{ t('channels.config.title') }}</DialogTitle>
        <DialogDescription v-if="platform">
          {{ t(`channels.meta.${platform.id}.name`) }}
        </DialogDescription>
      </DialogHeader>

      <form class="space-y-4 py-2" @submit.prevent="handleSave">
        <div class="space-y-2">
          <Label for="channel-name">{{ t('channels.config.name') }}</Label>
          <Input
            id="channel-name"
            v-model="name"
            :placeholder="t('channels.config.namePlaceholder')"
          />
        </div>

        <!-- Feishu-specific fields -->
        <template v-if="isFeishu()">
          <div class="space-y-2">
            <Label for="app-id">{{ t('channels.config.appId') }}</Label>
            <Input
              id="app-id"
              v-model="appId"
              :placeholder="t('channels.config.appIdPlaceholder')"
            />
          </div>
          <div class="space-y-2">
            <Label for="app-secret">{{ t('channels.config.appSecret') }}</Label>
            <Input
              id="app-secret"
              v-model="appSecret"
              type="password"
              :placeholder="t('channels.config.appSecretPlaceholder')"
            />
          </div>
        </template>

        <!-- Generic token-based platforms -->
        <template v-else>
          <div class="space-y-2">
            <Label for="channel-token">{{ t('channels.config.token') }}</Label>
            <Input
              id="channel-token"
              v-model="token"
              type="password"
              :placeholder="t('channels.config.tokenPlaceholder')"
            />
          </div>
        </template>

        <DialogFooter>
          <Button variant="outline" type="button" @click="open = false">
            {{ t('channels.config.cancel') }}
          </Button>
          <Button type="submit" :disabled="saving || !name.trim()">
            {{ t('channels.config.save') }}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>
</template>
