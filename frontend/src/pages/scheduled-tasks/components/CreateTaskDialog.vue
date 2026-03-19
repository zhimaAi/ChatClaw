<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { Agent, Channel, ScheduledTaskFormState } from '../types'
import { createSubmitLock } from './submitLock'
import TaskFormContent from './TaskFormContent.vue'

const props = defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: ScheduledTaskFormState
  agents: Agent[]
  channels: Channel[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()

const dialogSubtitle = computed(() =>
  props.form.id ? '更新自动化 AI 任务' : '安排自动化 AI 任务'
)
const submitLock = createSubmitLock()
const submitLocked = ref(false)
const submitDisabled = computed(() => props.saving || submitLocked.value)

function closeDialog() {
  submitLock.reset()
  submitLocked.value = submitLock.isLocked()
  emit('update:open', false)
}

function syncSubmitLockedState() {
  submitLocked.value = submitLock.isLocked()
}

function handleSubmit() {
  if (!submitLock.acquire(props.saving)) {
    syncSubmitLockedState()
    return
  }

  syncSubmitLockedState()
  emit('submit')
}

watch(
  () => props.saving,
  (saving) => {
    submitLock.syncSaving(saving)
    syncSubmitLockedState()
  }
)

watch(
  () => props.open,
  (open) => {
    if (!open) {
      submitLock.reset()
      syncSubmitLockedState()
    }
  }
)
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      :show-close-button="true"
      class="flex max-h-[90vh] flex-col gap-0 overflow-hidden border-[#e5e7eb] bg-white p-0 shadow-[0_24px_80px_rgba(15,23,42,0.14)] sm:!w-auto sm:min-w-[780px] sm:!max-w-[780px]"
    >
      <DialogHeader
        class="flex shrink-0 flex-row items-baseline gap-3 border-b border-[#eef2f6] px-7 py-6"
      >
        <DialogTitle class="text-lg font-semibold text-[#111827]">
          {{ title }}
        </DialogTitle>
        <p class="text-sm font-medium text-[#94a3b8]">
          {{ dialogSubtitle }}
        </p>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto px-7">
        <TaskFormContent :form="form" :agents="agents" :channels="channels" />
      </div>

      <DialogFooter class="shrink-0 border-t border-[#eef2f6] px-7 py-5 sm:justify-end">
        <button
          type="button"
          class="inline-flex h-11 min-w-[92px] items-center justify-center rounded-xl border border-[#dbe3ec] bg-white px-5 text-sm font-medium text-[#475569] transition-colors hover:bg-[#f8fafc]"
          @click="closeDialog"
        >
          取消
        </button>
        <button
          type="button"
          :disabled="submitDisabled"
          class="inline-flex h-11 min-w-[116px] items-center justify-center rounded-xl bg-[#111827] px-5 text-sm font-semibold text-white transition-colors hover:bg-[#0f172a] disabled:cursor-not-allowed disabled:bg-[#94a3b8]"
          @click="handleSubmit"
        >
          {{ submitDisabled ? '提交中...' : '确定' }}
        </button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
