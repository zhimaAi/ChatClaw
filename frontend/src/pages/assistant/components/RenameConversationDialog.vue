<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { LoaderCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

import type { Conversation } from '@bindings/willchat/internal/services/conversations'
import {
  ConversationsService,
  UpdateConversationInput,
} from '@bindings/willchat/internal/services/conversations'

const props = defineProps<{
  open: boolean
  conversation: Conversation | null
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  updated: [conversation: Conversation]
}>()

const { t } = useI18n()
const name = ref('')
const saving = ref(false)
const NAME_MAX_LEN = 100

const setOpen = (value: boolean) => emit('update:open', value)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    name.value = props.conversation?.name || ''
    saving.value = false
  }
)

const isValid = computed(() => {
  if (!props.conversation) return false
  const n = name.value.trim()
  return n.length > 0 && n.length <= NAME_MAX_LEN
})

const handleSave = async () => {
  if (!props.conversation || !isValid.value || saving.value) return
  saving.value = true
  try {
    const updated = await ConversationsService.UpdateConversation(
      props.conversation.id,
      new UpdateConversationInput({
        name: name.value.trim(),
      })
    )
    if (!updated) throw new Error(t('assistant.errors.updateConversationFailed'))
    emit('updated', updated)
    toast.success(t('assistant.conversation.rename.success'))
    setOpen(false)
  } catch (error) {
    console.error('Failed to rename conversation:', error)
    toast.error(getErrorMessage(error) || t('assistant.errors.updateConversationFailed'))
  } finally {
    saving.value = false
  }
}

const handleEnter = (event: KeyboardEvent) => {
  // Avoid submitting while IME is composing.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const anyEvent = event as any
  if (anyEvent?.isComposing || anyEvent?.keyCode === 229) return
  event.preventDefault()
  void handleSave()
}
</script>

<template>
  <Dialog :open="open" @update:open="setOpen">
    <DialogContent size="md">
      <DialogHeader>
        <DialogTitle>{{ t('assistant.conversation.rename.title') }}</DialogTitle>
      </DialogHeader>

      <div class="flex flex-col gap-4 py-4">
        <div class="flex flex-col gap-1.5">
          <label class="text-sm font-medium text-foreground">{{ t('assistant.fields.name') }}</label>
          <Input
            v-model="name"
            :placeholder="t('assistant.conversation.rename.placeholder')"
            :maxlength="NAME_MAX_LEN"
            :disabled="saving"
            @keydown.enter="handleEnter"
          />
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" :disabled="saving" @click="setOpen(false)">
          {{ t('assistant.actions.cancel') }}
        </Button>
        <Button :disabled="!isValid || saving" @click="handleSave">
          <LoaderCircle v-if="saving" class="mr-2 size-4 animate-spin" />
          {{ t('assistant.conversation.rename.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
