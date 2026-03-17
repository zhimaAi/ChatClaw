<script setup lang="ts">
import { computed, ref, watch, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, X, Image as ImageIcon, File as FileIcon } from 'lucide-vue-next'
import { toast } from '@/components/ui/toast'
import { Button } from '@/components/ui/button'

import type { ImagePayload } from '@bindings/chatclaw/internal/services/chat'

const MAX_IMAGES = 4
const MAX_IMAGE_SIZE = 2 * 1024 * 1024
const MAX_TOTAL_SIZE = 8 * 1024 * 1024
const MAX_FILES = 4
const MAX_FILE_SIZE = 20 * 1024 * 1024

const ALLOWED_FILE_EXTENSIONS = [
  '.pdf',
  '.doc',
  '.docx',
  '.xls',
  '.xlsx',
  '.ppt',
  '.pptx',
  '.txt',
  '.csv',
  '.md',
  '.json',
  '.xml',
  '.html',
  '.rtf',
  '.log',
]

type PendingImage = {
  id: string
  mimeType: string
  base64: string
  dataUrl: string
  fileName: string
  size: number
}

type PendingFile = {
  id: string
  mimeType: string
  base64: string
  fileName: string
  originalName: string
  filePath: string
  size: number
  isExisting: boolean
}

const props = defineProps<{
  initialContent: string
  initialImages?: ImagePayload[]
  initialFiles?: ImagePayload[]
}>()

const emit = defineEmits<{
  save: [payload: { newContent: string; images: ImagePayload[] }]
  cancel: []
}>()

const { t } = useI18n()

const editContent = ref(props.initialContent)
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
const docFileInputRef = ref<HTMLInputElement | null>(null)

const pendingImages = ref<PendingImage[]>([])
const pendingFiles = ref<PendingFile[]>([])
const totalImages = computed(() => pendingImages.value.length)
const totalFiles = computed(() => pendingFiles.value.length)
const totalSize = computed(() => pendingImages.value.reduce((sum, img) => sum + img.size, 0))

const normalizePayload = (img: ImagePayload): PendingImage => ({
  id: img.id ?? `${Date.now()}-${Math.random()}`,
  mimeType: img.mime_type,
  base64: img.base64,
  dataUrl: img.data_url ?? `data:${img.mime_type};base64,${img.base64}`,
  fileName: img.file_name ?? 'image',
  size: img.size ?? Math.ceil((img.base64.length * 3) / 4),
})

const normalizeFilePayload = (f: ImagePayload): PendingFile => ({
  id: f.id ?? `${Date.now()}-${Math.random()}`,
  mimeType: f.mime_type,
  base64: f.base64 ?? '',
  fileName: f.file_name ?? 'file',
  originalName: f.original_name ?? f.file_name ?? 'file',
  filePath: f.file_path ?? '',
  size: f.size ?? 0,
  isExisting: true,
})

const formatFileSize = (bytes: number): string => {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

watch(
  () => props.initialImages,
  (value) => {
    pendingImages.value = (value ?? []).map(normalizePayload)
  },
  { immediate: true }
)

watch(
  () => props.initialFiles,
  (value) => {
    pendingFiles.value = (value ?? []).map(normalizeFilePayload)
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

const handleSelectFilesClick = () => {
  if (totalFiles.value >= MAX_FILES) {
    toast.error(t('assistant.errors.tooManyFiles', { max: MAX_FILES }))
    return
  }
  docFileInputRef.value?.click()
}

const handleDocFilesSelected = async (event: Event) => {
  const target = event.target as HTMLInputElement
  const files = target.files
  if (!files || files.length === 0) return

  if (totalFiles.value + files.length > MAX_FILES) {
    toast.error(t('assistant.errors.tooManyFiles', { max: MAX_FILES }))
    return
  }

  for (const file of Array.from(files)) {
    const ext = '.' + file.name.split('.').pop()?.toLowerCase()
    if (!ALLOWED_FILE_EXTENSIONS.includes(ext)) {
      toast.error(t('assistant.errors.invalidFileType'))
      continue
    }
    if (file.size > MAX_FILE_SIZE) {
      toast.error(t('assistant.errors.fileTooLarge', { max: '20MB' }))
      continue
    }

    try {
      const dataUrl = await readFileAsDataUrl(file)
      const base64Match = dataUrl.match(/^data:[^;]+;base64,(.+)$/)
      if (!base64Match) throw new Error('Invalid file data')

      pendingFiles.value.push({
        id: `${Date.now()}-${Math.random()}`,
        mimeType: file.type || 'application/octet-stream',
        base64: base64Match[1],
        fileName: file.name,
        originalName: file.name,
        filePath: '',
        size: file.size,
        isExisting: false,
      })
    } catch (error) {
      console.error('Failed to read file for editing:', error)
      toast.error(t('assistant.errors.fileReadFailed'))
    }
  }

  if (docFileInputRef.value) {
    docFileInputRef.value.value = ''
  }
}

const handleRemoveFile = (id: string) => {
  pendingFiles.value = pendingFiles.value.filter((f) => f.id !== id)
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

const toFilePayload = (f: PendingFile): ImagePayload => ({
  id: f.id,
  kind: 'file',
  source: f.isExisting ? 'local_file' : 'inline_base64',
  mime_type: f.mimeType,
  base64: f.base64,
  file_name: f.fileName,
  file_path: f.filePath,
  original_name: f.originalName,
  size: f.size,
})

const handleSave = () => {
  const trimmed = editContent.value.trim()
  if (!trimmed && pendingImages.value.length === 0 && pendingFiles.value.length === 0) {
    toast.error(t('assistant.placeholders.enterToSend'))
    return
  }

  const imagePayloads = pendingImages.value.map(toImagePayload)
  const filePayloads = pendingFiles.value.map(toFilePayload)
  emit('save', { newContent: trimmed, images: [...imagePayloads, ...filePayloads] })
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
    <div v-if="pendingImages.length > 0" class="flex flex-wrap gap-2">
      <div
        v-for="img in pendingImages"
        :key="img.id"
        class="group relative h-24 w-24 rounded-md border border-border bg-muted/30"
      >
        <img :src="img.dataUrl" class="h-full w-full object-cover" :alt="img.fileName" />
        <button
          class="absolute right-0 top-0 flex size-4 items-center justify-center rounded-bl-md bg-destructive/80 text-destructive-foreground opacity-0 transition-opacity group-hover:opacity-100 active:bg-destructive"
          type="button"
          @click="handleRemoveImage(img.id)"
        >
          <X class="size-3" />
        </button>
      </div>
    </div>
    <!-- File cards -->
    <div v-if="pendingFiles.length > 0" class="flex flex-wrap gap-2">
      <div
        v-for="f in pendingFiles"
        :key="f.id"
        class="group relative flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2"
      >
        <FileIcon class="size-4 shrink-0 text-muted-foreground" />
        <div class="flex min-w-0 flex-col">
          <span class="truncate text-xs font-medium text-foreground" :title="f.originalName">{{
            f.originalName
          }}</span>
          <span v-if="f.size" class="text-[10px] text-muted-foreground">{{
            formatFileSize(f.size)
          }}</span>
        </div>
        <button
          type="button"
          class="ml-1 flex size-4 shrink-0 items-center justify-center rounded-full bg-destructive/80 text-destructive-foreground opacity-0 transition-opacity group-hover:opacity-100 active:bg-destructive"
          @click="handleRemoveFile(f.id)"
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
      <input
        ref="docFileInputRef"
        type="file"
        accept=".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt,.csv,.md,.json,.xml,.html,.rtf,.log"
        multiple
        class="hidden"
        @change="handleDocFilesSelected"
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
      <Button
        size="icon"
        variant="ghost"
        class="size-8 rounded-full border border-border bg-background hover:bg-muted/40"
        :disabled="totalFiles >= MAX_FILES"
        @click="handleSelectFilesClick"
      >
        <FileIcon class="size-4 text-muted-foreground" />
      </Button>
      <span class="text-xs text-muted-foreground">
        {{ totalImages }}/{{ MAX_IMAGES }} {{ t('assistant.chat.selectImages') }} ·
        {{ totalFiles }}/{{ MAX_FILES }} {{ t('assistant.chat.uploadFile') }}
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
