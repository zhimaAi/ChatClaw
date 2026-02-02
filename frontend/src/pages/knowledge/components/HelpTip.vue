<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { CircleHelp } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = defineProps<{
  content: string
  class?: string
}>()

const open = ref(false)
const rootEl = ref<globalThis.HTMLElement | null>(null)

const close = () => {
  open.value = false
}

const toggle = () => {
  open.value = !open.value
}

const onDocPointerDown = (e: globalThis.PointerEvent) => {
  if (!open.value) return
  const target = e.target as globalThis.Node | null
  if (!target) return
  if (rootEl.value && !rootEl.value.contains(target)) {
    close()
  }
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocPointerDown)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocPointerDown)
})
</script>

<template>
  <span
    ref="rootEl"
    class="relative inline-flex"
    @mouseenter="open = true"
    @mouseleave="open = false"
  >
    <button
      type="button"
      :class="
        cn(
          'inline-flex items-center justify-center text-muted-foreground hover:text-foreground',
          props.class
        )
      "
      :aria-label="content"
      @click.stop="toggle"
    >
      <CircleHelp class="size-4" />
    </button>

    <div
      v-if="open"
      class="absolute left-full top-1/2 z-50 ml-2 w-[260px] -translate-y-1/2 rounded-md border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md"
    >
      {{ content }}
    </div>
  </span>
</template>
