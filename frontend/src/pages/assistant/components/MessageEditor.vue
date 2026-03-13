<script setup lang="ts">
import { computed, ref, watch, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, X, Image as ImageIcon } from 'lucide-vue-next'
import { toast } from '@/components/ui/toast'
import { Button } from '@/components/ui/button'

import type { ImagePayload } from '@bindings/chatclaw/internal/services/chat'

const MAX_IMAGES = 4
const MAX_IMAGE_SIZE = 2 * 1024 * 1024
const MAX_TOTAL_SIZE = 8 * 1024 * 1024

type PendingImage = {
  id: string
  mimeType: string
  base64: string
  dataUrl: string
  fileName: string
  size: number
}

const props = defineProps<{
  initialContent: string
  initialImages?: ImagePayload[]
}>()

const emit = defineEmits<{
  save: [payload: { newContent: string; images: ImagePayload[] }]
  cancel: []
}>()

const { t } = useI18n()

const editContent = ref(props.initialContent)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)

const pendingImages = ref<PendingImage[]>([])
const totalImages = computed(() => pendingImages.value.length)
const totalSize = computed(() => pendingImages.value.reduce((sum, img) => sum + img.size, 0))

const normalizePayload = (img: ImagePayload): PendingImage => ({
  id: img.id ?? `${Date.now()}-${Math.random()}`,
  mimeType: img.mime_type,
  base64: img.base64,
  dataUrl: img.data_url ?? `data:${img.mime_type};base64,${img.base64}`,
  fileName: img.file_name ?? 'image',
  size: img.size ?? Math.ceil((img.base64.length * 3) / 4),
})

watch(
  () => props.initialImages,
  (value) => {
    pendingImages.value = (value ?? []).map(normalizePayload)
  },
  { immediate: true }
)

watch(
  () => props.initialContent,
  (value) => {
    editContent.value = value
  }
)

const readFileAsDataUrl = (file: File): Promise<string> =>
  new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result
      if (typeof result === 'string') {
        resolve(result)
      } else {
        reject(new Error('Invalid data URL'))
      }
    }
    reader.onerror = reject
    reader.readAsDataURL(file)
  })

const createPendingImageFromFile = async (file: File): Promise<PendingImage | null> => {
  try {
    const dataUrl = await readFileAsDataUrl(file)
    const base64Match = dataUrl.match(/^data:[^;]+;base64,(.+)$/)
    if (!base64Match) {
      throw new Error('Invalid image data')
    }
    return {
      id: `${Date.now()}-${Math.random()}`,
      mimeType: file.type,
      base64: base64Match[1],
      dataUrl,
      fileName: file.name,
      size: file.size,
    }
  } catch (error) {
    toast.error(t('assistant.errors.imageReadFailed'))
    console.error('Failed to read image for editing:', error)
    return null
  }
}

const handleSelectImagesClick = () => {
  if (totalImages.value >= MAX_IMAGES) {
    toast.error(t('assistant.errors.tooManyImages', { max: MAX_IMAGES }))
    return
  }
  fileInputRef.value?.click()
}

const processImageFiles = async (files: FileList | File[]) => {
  const fileArray = Array.from(files).filter((file) => file.type.startsWith('image/'))
  if (fileArray.length === 0) {
    toast.error(t('assistant.errors.invalidImageType'))
    return
  }

  if (totalImages.value + fileArray.length > MAX_IMAGES) {
    toast.error(t('assistant.errors.tooManyImages', { max: MAX_IMAGES }))
    return
  }

  let traversedSize = totalSize.value
  for (const file of fileArray) {
    if (file.size > MAX_IMAGE_SIZE) {
      toast.error(t('assistant.errors.imageTooLarge', { max: '2MB' }))
      return
    }
    traversedSize += file.size
  }

  if (traversedSize > MAX_TOTAL_SIZE) {
    toast.error(t('assistant.errors.imagesTotalTooLarge', { max: '8MB' }))
    return
  }

  const newImages: PendingImage[] = []
  for (const file of fileArray) {
    const pending = await createPendingImageFromFile(file)
    if (pending) {
      newImages.push(pending)
    }
  }

  pendingImages.value = [...pendingImages.value, ...newImages]
}

const handleFilesSelected = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (files && files.length > 0) {
    await processImageFiles(files)
  }
  if (fileInputRef.value) {
    fileInputRef.value.value = ''
  }
}

const handleRemoveImage = (id: string) => {
  pendingImages.value = pendingImages.value.filter((img) => img.id !== id)
}

const toImagePayload = (img: PendingImage): ImagePayload => ({
  id: img.id,
  kind: 'image',
  source: 'inline_base64',
  mime_type: img.mimeType,
  base64: img.base64,
  data_url: img.dataUrl,
  file_name: img.fileName,
  size: img.size,
})

const handleSave = () => {
  const trimmed = editContent.value.trim()
  if (!trimmed && pendingImages.value.length === 0) {
    toast.error(t('assistant.placeholders.enterToSend'))
    return
  }

  const imagePayloads = pendingImages.value.map(toImagePayload)
  emit('save', { newContent: trimmed, images: imagePayloads })
}

const handleCancel = () => {
  emit('cancel')
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Enter' && !event.shiftKey) {
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
    <div
      v-if="pendingImages.length > 0"
      class="flex flex-wrap gap-2"
    >
      <div
        v-for="img in pendingImages"
        :key="img.id"
        class="group relative h-24 w-24 rounded-md border border-border bg-muted/30"
      >
        <img :src="img.dataUrl" class="h-full w-full object-cover" :alt="img.fileName" />
        <button
          class="absolute right-0 top-0 flex size-4 items-center justify-center rounded-bl-md bg-destructive/80 text-destructive-foreground opacity-0 transition-opacity group-hover:opacity-100"
          @click="handleRemoveImage(img.id)"
          type="button"
        >
          <X class="size-3" />
        </button>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <input
        ref="fileInputRef"
        type="file"
        accept="image/*"
        multiple
        class="hidden"
        @change="handleFilesSelected"
      />
      <Button
        size="icon"
        variant="ghost"
        class="size-8 rounded-full border border-border bg-background hover:bg-muted/40"
        :disabled="totalImages >= MAX_IMAGES"
        @click="handleSelectImagesClick"
      >
        <ImageIcon class="size-4 text-muted-foreground" />
      </Button>
      <span class="text-xs text-muted-foreground">
        {{ totalImages }}/{{ MAX_IMAGES }} {{ t('assistant.chat.selectImages') }}
      </span>
    </div>
    <div class="flex items-center justify-end gap-2">
      <Button size="sm" variant="ghost" class="h-7 gap-1 px-2 text-xs" @click="handleCancel">
        <X class="size-3 shrink-0" />
        {{ t('common.cancel') }}
      </Button>
      <Button size="sm" class="h-7 gap-1 px-2 text-xs" @click="handleSave">
        <Check class="size-3 shrink-0" />
        {{ t('assistant.chat.resend') }}
      </Button>
    </div>
  </div>
</template>
