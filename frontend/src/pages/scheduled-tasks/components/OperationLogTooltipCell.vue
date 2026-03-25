<script setup lang="ts">
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

interface Props {
  value: string
}

const props = defineProps<Props>()

// Keep long operation-log values on one line and reveal the full text via tooltip.
const OPERATION_LOG_TOOLTIP_DELAY_DURATION = 200
const OPERATION_LOG_TOOLTIP_CONTAINER_CLASS = 'min-w-0 w-full overflow-hidden'
const OPERATION_LOG_TOOLTIP_TRIGGER_CLASS =
  'block w-full max-w-full overflow-hidden text-ellipsis whitespace-nowrap'
const OPERATION_LOG_TOOLTIP_CONTENT_CLASS =
  'max-w-[480px] whitespace-pre-wrap break-all text-xs leading-5'
</script>

<template>
  <div :class="OPERATION_LOG_TOOLTIP_CONTAINER_CLASS">
    <TooltipProvider :delay-duration="OPERATION_LOG_TOOLTIP_DELAY_DURATION">
      <Tooltip>
        <TooltipTrigger as-child>
          <div :class="OPERATION_LOG_TOOLTIP_TRIGGER_CLASS">
            {{ props.value }}
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" :class="OPERATION_LOG_TOOLTIP_CONTENT_CLASS">
          {{ props.value }}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </div>
</template>
