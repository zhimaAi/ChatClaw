<script setup lang="ts">
import { computed } from 'vue'
import { cn } from '@/lib/utils'
import { Slider } from '@/components/ui/slider'

type Mark = {
  value: number
  label: string
  emphasize?: boolean
}

const props = defineProps<{
  modelValue: number[]
  min: number
  max: number
  step?: number
  disabled?: boolean
  marks: Mark[]
  class?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const positions = computed(() =>
  props.marks.map((m) => {
    const clamped = Math.min(props.max, Math.max(props.min, m.value))
    const pct =
      props.max === props.min ? 0 : ((clamped - props.min) / (props.max - props.min)) * 100
    return { ...m, pct }
  })
)
</script>

<template>
  <div :class="cn('w-full', props.class)">
    <Slider
      :model-value="modelValue"
      :min="min"
      :max="max"
      :step="step ?? 1"
      :disabled="disabled"
      @update:model-value="emit('update:modelValue', $event as number[])"
    />

    <div class="relative mt-2 h-5 select-none text-xs text-foreground">
      <div
        v-for="m in positions"
        :key="`${m.value}-${m.label}`"
        class="absolute top-0 -translate-x-1/2 whitespace-nowrap"
        :style="{ left: `${m.pct}%` }"
      >
        <span :class="m.emphasize && 'font-medium'">{{ m.label }}</span>
      </div>
    </div>
  </div>
</template>
