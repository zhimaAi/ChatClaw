<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  initialContent: string
}>()

const emit = defineEmits<{
  save: [newContent: string]
  cancel: []
}>()

const { t } = useI18n()

const editContent = ref(props.initialContent)
const textareaRef = ref<HTMLTextAreaElement | null>(null)

const handleSave = () => {
  const trimmed = editContent.value.trim()
  if (trimmed && trimmed !== props.initialContent) {
    emit('save', trimmed)
  } else {
    emit('cancel')
  }
}

const handleCancel = () => {
  emit('cancel')
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Enter' && !event.shiftKey) {
    // Do not submit while IME is composing.

    const anyEvent = event as any
    if (anyEvent?.isComposing || anyEvent?.keyCode === 229) return
    event.preventDefault()
    handleSave()
  } else if (event.key === 'Escape') {
    handleCancel()
  }
}

onMounted(() => {
  nextTick(() => {
    if (textareaRef.value) {
      textareaRef.value.focus()
      textareaRef.value.select()
    }
  })
})
</script>

<template>
  <div class="flex flex-col gap-2">
    <textarea
      ref="textareaRef"
      v-model="editContent"
      class="min-h-[60px] w-full resize-none rounded-lg border border-border bg-background p-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
      rows="3"
      @keydown="handleKeydown"
    />
    <div class="flex items-center justify-end gap-2">
      <Button size="sm" variant="ghost" class="h-7 px-2 text-xs" @click="handleCancel">
        <X class="mr-1 size-3" />
        {{ t('common.cancel') }}
      </Button>
      <Button size="sm" class="h-7 px-2 text-xs" @click="handleSave">
        <Check class="mr-1 size-3" />
        {{ t('assistant.chat.resend') }}
      </Button>
    </div>
  </div>
</template>
