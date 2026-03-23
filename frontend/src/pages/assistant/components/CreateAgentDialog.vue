<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useThemeLogo } from '@/composables/useLogo'
import { Dialogs } from '@wailsio/runtime'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import { defaultAvatars } from '@/assets/avatars'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

const props = defineProps<{
  open: boolean
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  create: [data: { name: string; prompt: string; icon: string }]
}>()

const { t } = useI18n()
const { logoSrc } = useThemeLogo()
const hidePrompt = ref(false)

const name = ref('')
const prompt = ref('')
const icon = ref<string>('') // data URL

const isValid = computed(() => name.value.trim() !== '')

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    name.value = ''
    prompt.value = ''
    icon.value = ''
    if (!hidePrompt.value) {
      try {
        prompt.value = await AgentsService.GetDefaultPrompt()
      } catch {
        // Fallback: leave prompt empty if the backend call fails
      }
    }
  }
)

const handleClose = () => emit('update:open', false)

const handleSelectDefaultAvatar = (src: string) => {
  icon.value = src
}

const handlePickIcon = async () => {
  if (props.loading) return
  try {
    const path = await Dialogs.OpenFile({
      CanChooseFiles: true,
      CanChooseDirectories: false,
      AllowsMultipleSelection: false,
      Title: t('assistant.icon.pickTitle'),
      Filters: [
        {
          DisplayName: t('assistant.icon.filterImages'),
          Pattern: '*.png;*.jpg;*.jpeg;*.gif;*.webp;*.svg',
        },
      ],
    })
    if (!path) return
    icon.value = await AgentsService.ReadIconFile(path)
  } catch (error) {
    // User cancelled the file dialog — not an error
    if (String(error).includes('cancelled by user')) return
    console.error('Failed to pick icon:', error)
  }
}

const handleCreate = () => {
  if (!isValid.value || props.loading) return
  emit('create', { name: name.value.trim(), prompt: prompt.value.trim(), icon: icon.value })
}
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent size="lg">
      <DialogHeader>
        <DialogTitle>{{ t('assistant.create.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <div class="flex flex-col items-center gap-2">
          <button
            class="flex size-icon-box items-center justify-center rounded-icon-box border border-border bg-white text-foreground dark:border-white/15 dark:bg-white/5"
            type="button"
            @click="handlePickIcon"
          >
            <img v-if="icon" :src="icon" class="size-icon-lg rounded-md object-contain" />
            <img v-else :src="logoSrc" class="size-icon-lg" alt="ChatClaw logo" />
          </button>
          <div class="text-xs text-muted-foreground">
            {{ t('assistant.icon.hint') }}
          </div>
        </div>

        <div class="flex flex-col gap-2">
          <div class="text-xs text-muted-foreground">
            {{ t('assistant.icon.defaultAvatars') }}
          </div>
          <div class="flex flex-wrap gap-3">
            <button
              v-for="avatar in defaultAvatars"
              :key="avatar.id"
              type="button"
              class="relative flex size-12 items-center justify-center rounded-xl border transition-colors"
              :class="
                icon === avatar.src
                  ? 'border-primary bg-primary/10'
                  : 'border-border bg-background hover:border-foreground/40 hover:bg-muted/60 dark:border-white/10'
              "
              @click="handleSelectDefaultAvatar(avatar.src)"
            >
              <img :src="avatar.src" class="size-10 rounded-lg object-cover" />
              <div
                v-if="icon === avatar.src"
                class="absolute inset-0 flex items-center justify-center rounded-xl bg-black/40"
              >
                <span
                  class="flex size-6 items-center justify-center rounded-full bg-primary text-primary-foreground shadow-sm"
                >
                  ✓
                </span>
              </div>
            </button>
          </div>
        </div>

        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('assistant.fields.name') }}
            <span class="text-destructive">*</span>
          </label>
          <Input
            v-model="name"
            :placeholder="t('assistant.fields.namePlaceholder')"
            maxlength="100"
          />
        </div>

        <div v-if="!hidePrompt" class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('assistant.fields.prompt') }}
          </label>
          <textarea
            v-model="prompt"
            :placeholder="t('assistant.fields.promptPlaceholder')"
            maxlength="1000"
            class="min-h-[110px] w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="loading" @click="handleClose">
          {{ t('assistant.actions.cancel') }}
        </Button>
        <Button :disabled="!isValid || loading" @click="handleCreate">
          {{ t('assistant.actions.create') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
