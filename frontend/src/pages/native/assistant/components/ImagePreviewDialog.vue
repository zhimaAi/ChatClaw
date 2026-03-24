<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, ZoomIn, ZoomOut, RotateCw, X } from 'lucide-vue-next'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface ImagePayload {
  id?: string
  kind: string
  source: string
  mime_type: string
  base64: string
  data_url?: string
  file_name?: string
  size?: number
}

const props = defineProps<{
  open: boolean
  images: ImagePayload[]
  initialIndex?: number
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
}>()

const currentIndex = ref(props.initialIndex ?? 0)
const scale = ref(1)
const rotation = ref(0)
const imageRef = ref<HTMLImageElement | null>(null)
const containerRef = ref<HTMLDivElement | null>(null)
const isDragging = ref(false)
const dragStart = ref({ x: 0, y: 0 })
const translate = ref({ x: 0, y: 0 })

const currentImage = computed(() => props.images[currentIndex.value])
const hasMultipleImages = computed(() => props.images.length > 1)
const canGoPrev = computed(() => hasMultipleImages.value && currentIndex.value > 0)
const canGoNext = computed(
  () => hasMultipleImages.value && currentIndex.value < props.images.length - 1
)

const close = () => {
  emit('update:open', false)
}

const resetTransform = () => {
  scale.value = 1
  rotation.value = 0
  translate.value = { x: 0, y: 0 }
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      currentIndex.value = props.initialIndex ?? 0
      resetTransform()
    }
  }
)

watch(
  () => props.initialIndex,
  (index) => {
    if (index !== undefined && props.open) {
      currentIndex.value = index
      resetTransform()
    }
  }
)

const goPrev = () => {
  if (canGoPrev.value) {
    currentIndex.value--
    resetTransform()
  }
}

const goNext = () => {
  if (canGoNext.value) {
    currentIndex.value++
    resetTransform()
  }
}

const zoomIn = () => {
  scale.value = Math.min(scale.value + 0.25, 5)
}

const zoomOut = () => {
  scale.value = Math.max(scale.value - 0.25, 0.5)
}

const rotate = () => {
  rotation.value = (rotation.value + 90) % 360
}

const handleWheel = (e: WheelEvent) => {
  if (!props.open) return
  e.preventDefault()
  const delta = e.deltaY > 0 ? -0.1 : 0.1
  scale.value = Math.max(0.5, Math.min(5, scale.value + delta))
}

const handleMouseDown = (e: MouseEvent) => {
  if (scale.value <= 1) return
  isDragging.value = true
  dragStart.value = { x: e.clientX - translate.value.x, y: e.clientY - translate.value.y }
}

const handleMouseMove = (e: MouseEvent) => {
  if (!isDragging.value || scale.value <= 1) return
  translate.value = {
    x: e.clientX - dragStart.value.x,
    y: e.clientY - dragStart.value.y,
  }
}

const handleMouseUp = () => {
  isDragging.value = false
}

const handleKeyDown = (e: KeyboardEvent) => {
  if (!props.open) return
  switch (e.key) {
    case 'ArrowLeft':
      e.preventDefault()
      goPrev()
      break
    case 'ArrowRight':
      e.preventDefault()
      goNext()
      break
    case 'Escape':
      e.preventDefault()
      close()
      break
    case '+':
    case '=':
      e.preventDefault()
      zoomIn()
      break
    case '-':
      e.preventDefault()
      zoomOut()
      break
    case 'r':
    case 'R':
      e.preventDefault()
      rotate()
      break
  }
}

onMounted(() => {
  window.addEventListener('wheel', handleWheel, { passive: false })
  window.addEventListener('mousemove', handleMouseMove)
  window.addEventListener('mouseup', handleMouseUp)
  window.addEventListener('keydown', handleKeyDown)
})

onUnmounted(() => {
  window.removeEventListener('wheel', handleWheel)
  window.removeEventListener('mousemove', handleMouseMove)
  window.removeEventListener('mouseup', handleMouseUp)
  window.removeEventListener('keydown', handleKeyDown)
})

const imageStyle = computed(() => {
  return {
    transform: `scale(${scale.value}) rotate(${rotation.value}deg) translate(${translate.value.x}px, ${translate.value.y}px)`,
    transition: isDragging.value ? 'none' : 'transform 0.2s ease-out',
  }
})
</script>

<template>
  <Dialog :open="open" @update:open="(v) => emit('update:open', v)">
    <DialogContent
      :size="'full'"
      class="max-h-[calc(100vh-2rem)] p-0"
      @pointer-down-outside="close"
    >
      <div ref="containerRef" class="relative flex h-full flex-col bg-background">
        <!-- Header toolbar -->
        <div
          class="absolute top-0 left-0 right-0 z-10 flex items-center justify-between border-b border-border bg-background/95 px-4 py-2 backdrop-blur-sm"
        >
          <div class="flex items-center gap-2">
            <span class="text-sm text-muted-foreground">
              {{ currentIndex + 1 }} / {{ images.length }}
            </span>
            <span v-if="currentImage?.file_name" class="text-sm font-medium text-foreground">
              {{ currentImage.file_name }}
            </span>
          </div>
          <div class="flex items-center gap-1">
            <Button
              size="icon"
              variant="ghost"
              class="size-8"
              :disabled="scale <= 0.5"
              @click="zoomOut"
            >
              <ZoomOut class="size-4 text-muted-foreground" />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              class="size-8"
              :disabled="scale >= 5"
              @click="zoomIn"
            >
              <ZoomIn class="size-4 text-muted-foreground" />
            </Button>
            <Button size="icon" variant="ghost" class="size-8" @click="rotate">
              <RotateCw class="size-4 text-muted-foreground" />
            </Button>
            <Button size="icon" variant="ghost" class="size-8" @click="close">
              <X class="size-4 text-muted-foreground" />
            </Button>
          </div>
        </div>

        <!-- Image container -->
        <div
          class="relative flex flex-1 items-center justify-center overflow-hidden p-4 pt-16"
          @mousedown="handleMouseDown"
        >
          <img
            v-if="currentImage"
            ref="imageRef"
            :src="
              currentImage.data_url ||
              `data:${currentImage.mime_type};base64,${currentImage.base64}`
            "
            :alt="currentImage.file_name || 'Image'"
            :style="imageStyle"
            class="max-h-full max-w-full select-none object-contain"
            draggable="false"
          />

          <!-- Navigation buttons -->
          <Button
            v-if="hasMultipleImages && canGoPrev"
            size="icon"
            variant="ghost"
            class="absolute left-4 size-10 bg-background/80 hover:bg-background"
            @click="goPrev"
          >
            <ChevronLeft class="size-5 text-foreground" />
          </Button>
          <Button
            v-if="hasMultipleImages && canGoNext"
            size="icon"
            variant="ghost"
            class="absolute right-4 size-10 bg-background/80 hover:bg-background"
            @click="goNext"
          >
            <ChevronRight class="size-5 text-foreground" />
          </Button>
        </div>

        <!-- Thumbnail strip (if multiple images) -->
        <div
          v-if="hasMultipleImages"
          class="flex gap-2 overflow-x-auto border-t border-border bg-background/95 p-2 backdrop-blur-sm"
        >
          <button
            v-for="(img, idx) in images"
            :key="img.id || img.file_name || idx"
            :class="
              cn(
                'relative h-16 w-16 shrink-0 overflow-hidden rounded-md border-2 transition-all',
                idx === currentIndex
                  ? 'border-primary ring-2 ring-primary/20'
                  : 'border-border opacity-60 hover:opacity-100'
              )
            "
            @click="currentIndex = idx"
          >
            <img
              :src="img.data_url || `data:${img.mime_type};base64,${img.base64}`"
              class="h-full w-full object-cover"
              :alt="img.file_name || `Thumbnail ${idx + 1}`"
            />
          </button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
