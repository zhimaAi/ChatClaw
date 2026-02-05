<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { Document } from './DocumentCard.vue'

const props = defineProps<{
  open: boolean
  document: Document | null
}>()

const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
  (e: 'confirm', doc: Document, newName: string): void
}>()

const { t } = useI18n()

const newName = ref('')
const isSubmitting = ref(false)

// 当对话框打开时，初始化名称
watch(
  () => props.open,
  (open) => {
    if (open && props.document) {
      // 移除扩展名，只显示文件名
      const name = props.document.name
      const lastDot = name.lastIndexOf('.')
      newName.value = lastDot > 0 ? name.substring(0, lastDot) : name
    }
  },
)

const handleConfirm = async () => {
  if (!props.document || !newName.value.trim()) return

  isSubmitting.value = true
  try {
    emit('confirm', props.document, newName.value.trim())
  } finally {
    isSubmitting.value = false
  }
}

const handleClose = () => {
  emit('update:open', false)
}
</script>

<template>
  <Dialog :open="open" @update:open="handleClose">
    <DialogContent class="sm:max-w-[400px]">
      <DialogHeader>
        <DialogTitle>{{ t('knowledge.content.rename.title') }}</DialogTitle>
        <DialogDescription>
          {{ t('knowledge.content.rename.desc') }}
        </DialogDescription>
      </DialogHeader>

      <div class="py-4">
        <Input
          v-model="newName"
          :placeholder="t('knowledge.content.rename.placeholder')"
          class="w-full"
          @keyup.enter="handleConfirm"
        />
      </div>

      <DialogFooter>
        <Button variant="outline" @click="handleClose">
          {{ t('knowledge.content.rename.cancel') }}
        </Button>
        <Button :disabled="!newName.trim() || isSubmitting" @click="handleConfirm">
          {{ t('knowledge.content.rename.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
