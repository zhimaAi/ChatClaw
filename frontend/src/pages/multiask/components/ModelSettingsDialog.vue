<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { ProviderIcon } from '@/components/ui/provider-icon'
import { GripVertical } from 'lucide-vue-next'

interface AIModel {
  id: string
  name: string
  icon: string
  displayName?: string
  url?: string
}

const props = defineProps<{
  open: boolean
  models: AIModel[]
  enabledIds: string[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  save: [models: AIModel[], enabledIds: string[]]
}>()

const { t } = useI18n()

const localModels = ref<AIModel[]>([])
const localEnabledIds = ref<string[]>([])

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      localModels.value = [...props.models]
      localEnabledIds.value = [...props.enabledIds]
    }
  }
)

const handleSave = () => {
  emit('save', localModels.value, localEnabledIds.value)
  emit('update:open', false)
}

const setEnabled = (id: string, checked: boolean) => {
  if (checked) {
    if (!localEnabledIds.value.includes(id)) {
      localEnabledIds.value.push(id)
    }
  } else {
    localEnabledIds.value = localEnabledIds.value.filter((i) => i !== id)
  }
}

// Pointer-based drag and drop for visual element-follows-cursor effect
const draggedIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
const dragDirection = ref<'up' | 'down' | null>(null)
const listRef = ref<HTMLElement | null>(null)

let dragClone: HTMLElement | null = null
let dragStartX = 0
let dragStartY = 0
let cloneOriginX = 0
let cloneOriginY = 0

const onRowPointerDown = (index: number, event: PointerEvent) => {
  const target = event.target as HTMLElement
  if (target.closest('[data-no-drag]')) return

  const li = target.closest('[data-drag-item]') as HTMLElement
  if (!li) return

  event.preventDefault()

  const rect = li.getBoundingClientRect()
  dragStartX = event.clientX
  dragStartY = event.clientY
  cloneOriginX = rect.left
  cloneOriginY = rect.top

  const clone = li.cloneNode(true) as HTMLElement
  Object.assign(clone.style, {
    position: 'fixed',
    left: `${rect.left}px`,
    top: `${rect.top}px`,
    width: `${rect.width}px`,
    height: `${rect.height}px`,
    zIndex: '9999',
    pointerEvents: 'none',
    opacity: '0.95',
    borderRadius: '0.375rem',
    boxShadow: '0 8px 24px rgba(0,0,0,0.18)',
    background: 'var(--popover)',
    cursor: 'grabbing',
    transform: 'translate3d(0,0,0)',
    transition: 'none',
    willChange: 'transform',
  })
  document.body.appendChild(clone)
  dragClone = clone

  draggedIndex.value = index

  document.addEventListener('pointermove', onPointerMove)
  document.addEventListener('pointerup', onPointerUp)
}

const onPointerMove = (event: PointerEvent) => {
  if (draggedIndex.value === null || !dragClone) return

  const dx = event.clientX - dragStartX
  const dy = event.clientY - dragStartY
  dragClone.style.transform = `translate3d(${dx}px,${dy}px,0)`

  const items = listRef.value?.querySelectorAll<HTMLElement>('[data-drag-item]')
  if (!items) return

  let found = false
  items.forEach((item, idx) => {
    if (idx === draggedIndex.value) return
    const r = item.getBoundingClientRect()
    if (event.clientY >= r.top && event.clientY <= r.bottom) {
      dragOverIndex.value = idx
      dragDirection.value = event.clientY - r.top < r.height / 2 ? 'up' : 'down'
      found = true
    }
  })
  if (!found) {
    dragOverIndex.value = null
    dragDirection.value = null
  }
}

const onPointerUp = () => {
  if (
    draggedIndex.value !== null &&
    dragOverIndex.value !== null &&
    draggedIndex.value !== dragOverIndex.value
  ) {
    const item = localModels.value[draggedIndex.value]
    let insertIndex = dragOverIndex.value
    if (dragDirection.value === 'down') insertIndex++
    if (draggedIndex.value < insertIndex) insertIndex--
    localModels.value.splice(draggedIndex.value, 1)
    localModels.value.splice(insertIndex, 0, item)
  }

  cleanupDrag()
}

const cleanupDrag = () => {
  if (dragClone) {
    document.body.removeChild(dragClone)
    dragClone = null
  }
  draggedIndex.value = null
  dragOverIndex.value = null
  dragDirection.value = null
  document.removeEventListener('pointermove', onPointerMove)
  document.removeEventListener('pointerup', onPointerUp)
}

onUnmounted(cleanupDrag)
</script>

<template>
  <Dialog :open="open" @update:open="emit('update:open', $event)">
    <DialogContent class="max-h-[85vh] flex max-w-md flex-col">
      <DialogHeader class="text-left">
        <div class="flex items-center gap-2">
          <DialogTitle>{{ t('multiask.modelSettings') }}</DialogTitle>
          <span class="text-xs text-muted-foreground font-normal">{{
            t('multiask.dragToReorder')
          }}</span>
        </div>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto py-4">
        <div class="flex items-center justify-between px-3 pb-2 text-xs text-muted-foreground">
          <span>{{ t('multiask.modelName') }}</span>
          <span>{{ t('multiask.hideOrShow') }}</span>
        </div>
        <ul ref="listRef" class="flex flex-col px-1 pb-1">
          <li
            v-for="(model, index) in localModels"
            :key="model.id"
            data-drag-item
            :class="[
              'group relative flex items-center justify-between p-2 transition-colors rounded-md cursor-grab active:cursor-grabbing touch-none',
              draggedIndex === index ? 'opacity-30 bg-muted/60' : 'hover:bg-muted/50',
            ]"
            @pointerdown="onRowPointerDown(index, $event)"
          >
            <!-- Drop indicator line -->
            <div
              v-if="dragOverIndex === index"
              :class="[
                'absolute left-0 right-0 h-0.5 bg-primary z-10 pointer-events-none rounded-full',
                dragDirection === 'up' ? '-top-[1px]' : '-bottom-[1px]',
              ]"
            ></div>
            <div class="flex items-center gap-2 pointer-events-none">
              <div
                class="text-muted-foreground/30 transition-colors group-hover:text-muted-foreground"
              >
                <GripVertical class="h-4 w-4" />
              </div>
              <div
                class="flex size-8 items-center justify-center rounded-md border border-border bg-background"
              >
                <ProviderIcon :icon="model.icon" :size="24" />
              </div>
              <span class="text-sm font-medium">{{ model.displayName || model.name }}</span>
            </div>
            <Switch
              data-no-drag
              class="cursor-pointer"
              :model-value="localEnabledIds.includes(model.id)"
              @update:model-value="setEnabled(model.id, $event)"
            />
          </li>
        </ul>
      </div>

      <DialogFooter>
        <Button variant="outline" @click="emit('update:open', false)">{{
          t('common.cancel')
        }}</Button>
        <Button @click="handleSave">{{ t('common.save') }}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
