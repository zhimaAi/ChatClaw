<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { EmojiPicker } from '@/components/ui/emoji-picker'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export interface CreateAgentData {
  name: string
  icon: string
  identityEmoji: string
}

const props = defineProps<{
  open: boolean
  loading?: boolean
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  create: [data: CreateAgentData]
}>()

const { t } = useI18n()

const name = ref('')
const identityEmoji = ref('')

const isValid = computed(() => name.value.trim() !== '')

watch(
  () => props.open,
  (open) => {
    if (!open) return
    name.value = ''
    identityEmoji.value = ''
  }
)

const handleClose = () => emit('update:open', false)

const handleCreate = () => {
  if (!isValid.value || props.loading) return
  emit('create', {
    name: name.value.trim(),
    icon: '',
    identityEmoji: identityEmoji.value,
  })
}
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent size="lg">
      <DialogHeader>
        <DialogTitle>{{ t('assistant.create.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
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

        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">
            {{ t('assistant.fields.identityEmoji') }}
          </label>
          <EmojiPicker v-model="identityEmoji" />
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
