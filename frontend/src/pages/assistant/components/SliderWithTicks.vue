<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { cn } from '@/lib/utils'

type Tick = { value: number; label: string }

const props = withDefaults(
  defineProps<{
    modelValue: number
    min: number
    max: number
    step?: number
    ticks?: Tick[]
    disabled?: boolean
    formatValue?: (v: number) => string
  }>(),
  {
    step: 1,
    ticks: () => [],
    disabled: false,
    formatValue: undefined,
  }
)

const emit = defineEmits<{
  'update:modelValue': [value: number]
}>()

const trackEl = ref<any>(null)
const dragging = ref(false)

const percent = computed(() => {
  if (props.max === props.min) return 0
  const clamped = Math.min(props.max, Math.max(props.min, props.modelValue))
  return ((clamped - props.min) / (props.max - props.min)) * 100
})

const displayValue = computed(() => {
  const v = props.modelValue
  return props.formatValue ? props.formatValue(v) : String(v)
})

function roundToStep(v: number) {
  const step = props.step || 1
  const snapped = Math.round((v - props.min) / step) * step + props.min
  const fixed = Number(snapped.toFixed(6))
  return Math.min(props.max, Math.max(props.min, fixed))
}

function valueFromClientX(clientX: number) {
  const el = trackEl.value
  if (!el) return props.modelValue
  const rect = el.getBoundingClientRect()
  const x = Math.min(rect.right, Math.max(rect.left, clientX))
  const ratio = rect.width <= 0 ? 0 : (x - rect.left) / rect.width
  const v = props.min + ratio * (props.max - props.min)
  return roundToStep(v)
}

function setValue(v: number) {
  emit('update:modelValue', v)
}

function onPointerDown(e: any) {
  if (props.disabled) return
  dragging.value = true
  ;(e.currentTarget as any).setPointerCapture(e.pointerId)
  setValue(valueFromClientX(e.clientX))
}

function onPointerMove(e: any) {
  if (!dragging.value || props.disabled) return
  setValue(valueFromClientX(e.clientX))
}

function onPointerUp() {
  dragging.value = false
}

function onKeyDown(e: KeyboardEvent) {
  if (props.disabled) return
  const step = props.step || 1
  if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
    e.preventDefault()
    setValue(roundToStep(props.modelValue - step))
  }
  if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
    e.preventDefault()
    setValue(roundToStep(props.modelValue + step))
  }
}

onBeforeUnmount(() => {
  dragging.value = false
})
</script>

<template>
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between">
      <div class="text-sm text-muted-foreground">{{ displayValue }}</div>
    </div>

    <div
      ref="trackEl"
      class="relative h-2 w-full rounded-full bg-muted"
      :class="disabled && 'opacity-50'"
      role="slider"
      :aria-valuemin="min"
      :aria-valuemax="max"
      :aria-valuenow="modelValue"
      tabindex="0"
      @pointerdown="onPointerDown"
      @pointermove="onPointerMove"
      @pointerup="onPointerUp"
      @pointercancel="onPointerUp"
      @keydown="onKeyDown"
    >
      <!-- filled -->
      <div
        class="absolute left-0 top-0 h-2 rounded-full bg-foreground"
        :style="{ width: `${percent}%` }"
      />
      <!-- thumb -->
      <div
        class="absolute top-1/2 size-4 -translate-x-1/2 -translate-y-1/2 rounded-full bg-background ring-1 ring-border"
        :style="{ left: `${percent}%` }"
      />
    </div>

    <div v-if="ticks.length" class="relative h-4 text-xs text-muted-foreground">
      <div
        v-for="t in ticks"
        :key="t.value"
        class="absolute -translate-x-1/2"
        :style="{ left: `${((t.value - min) / (max - min)) * 100}%` }"
      >
        <span :class="cn(t.value === modelValue && 'text-foreground')">{{ t.label }}</span>
      </div>
    </div>
  </div>
</template>
