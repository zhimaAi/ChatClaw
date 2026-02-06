<script setup lang="ts">
import { Label, type LabelProps } from 'reka-ui'
import { cn } from '@/lib/utils'
import { computed } from 'vue'

const props = defineProps<LabelProps & { class?: string }>()

const delegatedProps = computed(() => {
  const { class: _, ...rest } = props
  return rest
})
</script>

<template>
  <Label
    v-bind="delegatedProps"
    :class="
      cn(
        'text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70',
        props.class
      )
    "
  >
    <slot />
  </Label>
</template>
