<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import {
  SelectRoot,
  SelectTrigger as SelectTriggerRaw,
  SelectPortal,
  SelectContent as SelectContentRaw,
  SelectViewport,
  SelectItem as SelectItemRaw,
  SelectItemIndicator,
  SelectItemText,
} from 'reka-ui'
import { Check } from 'lucide-vue-next'

const props = defineProps<{
  modelValue: string
  compact?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const { t } = useI18n()

const modes = [
  { id: 'chat', labelKey: 'assistant.chatMode.chat', descKey: 'assistant.chatMode.chatDesc' },
  { id: 'task', labelKey: 'assistant.chatMode.task', descKey: 'assistant.chatMode.taskDesc' },
] as const
</script>

<template>
  <SelectRoot
    :model-value="modelValue"
    @update:model-value="(v: any) => v && emit('update:modelValue', String(v))"
  >
    <SelectTriggerRaw as-child>
      <!-- Compact mode (snap window): icon only -->
      <button
        v-if="compact"
        class="flex size-8 shrink-0 items-center justify-center rounded-full border border-border bg-background text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40 focus:outline-none"
        :title="t(modes.find((m) => m.id === modelValue)?.labelKey ?? 'assistant.chatMode.chat')"
      >
        <!-- Chat mode icon: speech bubble -->
        <svg
          v-if="modelValue === 'chat'"
          class="size-4 text-muted-foreground"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M7.9 20A9 9 0 1 0 4 16.1L2 22z" />
        </svg>
        <!-- Task mode icon: crosshair/target -->
        <svg
          v-else
          class="size-4 text-muted-foreground"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M12 2v4" />
          <path d="M12 18v4" />
          <path d="M2 12h4" />
          <path d="M18 12h4" />
          <circle cx="12" cy="12" r="4" />
          <circle cx="12" cy="12" r="8" />
        </svg>
      </button>
      <!-- Full mode (assistant / knowledge pages): icon + label -->
      <button
        v-else
        class="flex h-8 shrink-0 items-center gap-1.5 rounded-full border border-border bg-background px-3 text-xs shadow-[0_1px_2px_rgba(0,0,0,0.04)] hover:bg-muted/40 focus:outline-none"
      >
        <!-- Chat mode icon: speech bubble -->
        <svg
          v-if="modelValue === 'chat'"
          class="size-3.5 shrink-0 text-muted-foreground"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M7.9 20A9 9 0 1 0 4 16.1L2 22z" />
        </svg>
        <!-- Task mode icon: crosshair/target -->
        <svg
          v-else
          class="size-3.5 shrink-0 text-muted-foreground"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M12 2v4" />
          <path d="M12 18v4" />
          <path d="M2 12h4" />
          <path d="M18 12h4" />
          <circle cx="12" cy="12" r="4" />
          <circle cx="12" cy="12" r="8" />
        </svg>
        <span class="text-muted-foreground">
          {{ t(modes.find((m) => m.id === modelValue)?.labelKey ?? 'assistant.chatMode.chat') }}
        </span>
      </button>
    </SelectTriggerRaw>
    <SelectPortal>
      <SelectContentRaw
        class="z-50 min-w-[220px] overflow-y-auto rounded-md border bg-popover p-1 text-popover-foreground shadow-md"
        position="popper"
        :side-offset="5"
      >
        <SelectViewport>
          <SelectItemRaw
            v-for="mode in modes"
            :key="mode.id"
            :value="mode.id"
            class="relative flex cursor-pointer select-none items-center rounded-sm py-2 pl-2 pr-8 text-sm outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-50"
          >
            <span class="absolute right-2 flex size-3.5 items-center justify-center">
              <SelectItemIndicator>
                <Check class="size-4" />
              </SelectItemIndicator>
            </span>
            <div class="flex items-start gap-2">
              <!-- Chat mode icon: speech bubble -->
              <svg
                v-if="mode.id === 'chat'"
                class="mt-0.5 size-3.5 shrink-0 text-muted-foreground"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M7.9 20A9 9 0 1 0 4 16.1L2 22z" />
              </svg>
              <!-- Task mode icon: crosshair/target -->
              <svg
                v-else
                class="mt-0.5 size-3.5 shrink-0 text-muted-foreground"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M12 2v4" />
                <path d="M12 18v4" />
                <path d="M2 12h4" />
                <path d="M18 12h4" />
                <circle cx="12" cy="12" r="4" />
                <circle cx="12" cy="12" r="8" />
              </svg>
              <div class="flex flex-col">
                <SelectItemText>
                  <span class="font-medium">{{ t(mode.labelKey) }}</span>
                </SelectItemText>
                <span class="mt-0.5 text-xs text-muted-foreground">{{ t(mode.descKey) }}</span>
              </div>
            </div>
          </SelectItemRaw>
        </SelectViewport>
      </SelectContentRaw>
    </SelectPortal>
  </SelectRoot>
</template>
