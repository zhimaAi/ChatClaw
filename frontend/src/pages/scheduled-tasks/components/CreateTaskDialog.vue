<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Check, ChevronDown, Clock3 } from 'lucide-vue-next'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { SCHEDULE_PRESETS, WEEKDAY_OPTIONS } from '../constants'
import type { Agent, ScheduledTaskFormState } from '../types'
import { createSubmitLock } from './submitLock'

const props = defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: ScheduledTaskFormState
  agents: Agent[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()

const scheduleTypeOptions = [
  { value: 'preset', label: '快捷设置' },
  { value: 'custom', label: '自定义时间' },
  { value: 'cron', label: 'Linux Crontab 代码' },
] as const

const customModeOptions = [
  { value: 'interval', label: '每间隔' },
  { value: 'daily', label: '每天执行' },
  { value: 'weekly', label: '每周执行' },
  { value: 'monthly', label: '每月执行' },
] as const

const dialogSubtitle = computed(() => (props.form.id ? '更新自动化的 AI 任务' : '安排自动化的 AI 任务'))
const monthlyOptions = Array.from({ length: 31 }, (_, index) => index + 1)
const submitLock = createSubmitLock()
const submitLocked = ref(false)
const submitDisabled = computed(() => props.saving || submitLocked.value)

const customTimeValue = computed({
  get() {
    return `${String(props.form.customHour).padStart(2, '0')}:${String(props.form.customMinute).padStart(2, '0')}`
  },
  set(value: string) {
    const [hour, minute] = value.split(':')
    props.form.customHour = Number(hour || 0)
    props.form.customMinute = Number(minute || 0)
  },
})

const selectedWeeklyDay = computed({
  get() {
    return props.form.customWeekdays[0] ?? 1
  },
  set(value: number) {
    props.form.customWeekdays = [value]
  },
})

function closeDialog() {
  submitLock.reset()
  submitLocked.value = submitLock.isLocked()
  emit('update:open', false)
}

function syncSubmitLockedState() {
  submitLocked.value = submitLock.isLocked()
}

function selectScheduleType(value: ScheduledTaskFormState['scheduleType']) {
  props.form.scheduleType = value
  if (value === 'custom' && props.form.customMode === 'weekly' && props.form.customWeekdays.length === 0) {
    props.form.customWeekdays = [1]
  }
}

function selectCustomMode(value: ScheduledTaskFormState['customMode']) {
  props.form.customMode = value
  if (value === 'weekly') {
    selectedWeeklyDay.value = props.form.customWeekdays[0] ?? 1
  }
}

function selectWeeklyDay(value: number) {
  selectedWeeklyDay.value = value
}

function selectMonthlyDay(value: number) {
  props.form.customDayOfMonth = value
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
  },
)

watch(
  () => props.open,
  (open) => {
    if (!open) {
      submitLock.reset()
      syncSubmitLockedState()
    }
  },
)
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      :show-close-button="true"
      class="flex max-h-[90vh] flex-col gap-0 overflow-hidden border-[#e5e7eb] bg-white p-0 shadow-[0_24px_80px_rgba(15,23,42,0.14)] sm:!w-auto sm:min-w-[780px] sm:!max-w-[780px]"
    >
        <DialogHeader class="flex shrink-0 flex-row items-baseline gap-3 border-b border-[#eef2f6] px-7 py-6">
        <DialogTitle class="text-lg font-semibold text-[#111827]">
          {{ title }}
        </DialogTitle>
        <p class="text-sm font-medium text-[#94a3b8]">
          {{ dialogSubtitle }}
        </p>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto px-7">
        <div class="space-y-6 py-6">
          <section class="space-y-5">
          <div class="space-y-2">
            <label class="block text-[15px] font-semibold text-[#1f2937]">任务名称</label>
            <Input
              v-model="form.name"
              placeholder="例如：早间简报"
              class="h-11 rounded-xl border-[#dbe3ec] bg-white px-4 text-sm text-[#111827] placeholder:text-[#a0aec0] focus-visible:border-[#2563eb] focus-visible:ring-[#bfdbfe]"
            />
          </div>

          <div class="space-y-2">
            <label class="block text-[15px] font-semibold text-[#1f2937]">消息提示词</label>
            <textarea
              v-model="form.prompt"
              class="min-h-[118px] w-full rounded-xl border border-[#dbe3ec] bg-white px-4 py-3 text-sm leading-6 text-[#111827] outline-none transition-[border-color,box-shadow] placeholder:text-[#a0aec0] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
              placeholder="AI 应该做什么？例如：给我一份今天的新闻和天气摘要"
            />
          </div>

          <div class="space-y-2">
            <label class="block text-[15px] font-semibold text-[#1f2937]">关联 AI 助手</label>
            <div class="relative">
              <select
                v-model.number="form.agentId"
                class="h-11 w-full appearance-none rounded-xl border border-[#dbe3ec] bg-white px-4 pr-11 text-sm text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
              >
                <option :value="null">请选择助手</option>
                <option v-for="agent in agents" :key="agent.id" :value="agent.id">{{ agent.name }}</option>
              </select>
              <ChevronDown class="pointer-events-none absolute right-4 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]" />
            </div>
          </div>
        </section>

        <section class="space-y-4">
          <div class="space-y-1">
            <h3 class="text-[15px] font-semibold text-[#1f2937]">设置定时时间</h3>
            <p class="text-sm text-[#94a3b8]">选择快捷方案，或按你的节奏自定义执行时间。</p>
          </div>

          <div class="flex flex-wrap gap-2">
            <button
              v-for="option in scheduleTypeOptions"
              :key="option.value"
              type="button"
              class="inline-flex h-10 items-center rounded-xl border px-4 text-sm font-medium transition-colors"
              :class="
                form.scheduleType === option.value
                  ? 'border-[#2563eb] bg-[#eff6ff] text-[#2563eb]'
                  : 'border-[#dbe3ec] bg-white text-[#64748b] hover:border-[#cbd5e1] hover:text-[#0f172a]'
              "
              @click="selectScheduleType(option.value)"
            >
              {{ option.label }}
            </button>
          </div>

          <div class="rounded-2xl border border-[#e5e7eb] bg-[#f8fafc] p-4">
            <div v-if="form.scheduleType === 'preset'" class="space-y-2">
              <label class="block text-sm font-semibold text-[#334155]">快捷设置</label>
              <div class="grid gap-3 md:grid-cols-2">
                <button
                  v-for="item in SCHEDULE_PRESETS"
                  :key="item.value"
                  type="button"
                  class="flex min-h-11 items-center gap-3 rounded-xl border bg-white px-4 py-3 text-left text-sm transition-all"
                  :class="
                    form.schedulePreset === item.value
                      ? 'border-[#2563eb] bg-[#eff6ff] text-[#1d4ed8] shadow-[0_0_0_1px_rgba(37,99,235,0.08)]'
                      : 'border-[#dbe3ec] text-[#334155] hover:border-[#cbd5e1] hover:bg-[#f8fafc]'
                  "
                  @click="form.schedulePreset = item.value"
                >
                  <Clock3 class="size-4 shrink-0" />
                  <span class="flex-1 font-medium">{{ item.label }}</span>
                  <Check v-if="form.schedulePreset === item.value" class="size-4 shrink-0" />
                </button>
              </div>
            </div>

            <div v-else-if="form.scheduleType === 'custom'" class="space-y-4">
              <div class="flex items-start gap-5">
                <div class="inline-flex overflow-hidden rounded-2xl border border-[#dbe3ec] bg-white">
                  <div class="w-[140px] shrink-0">
                    <button
                      v-for="item in customModeOptions"
                      :key="item.value"
                      type="button"
                      class="flex h-12 w-full items-center justify-between px-4 text-sm font-medium transition-colors"
                      :class="
                        form.customMode === item.value
                          ? 'bg-[#f3f4f6] text-[#111827]'
                          : 'text-[#475569] hover:bg-[#f8fafc] hover:text-[#111827]'
                      "
                      @click="selectCustomMode(item.value)"
                    >
                      <span>{{ item.label }}</span>
                      <ChevronDown
                        v-if="item.value !== 'daily' && item.value !== 'interval'"
                        class="size-4 shrink-0 rotate-[-90deg] text-[#94a3b8]"
                        :class="form.customMode === item.value ? 'opacity-100' : 'opacity-40'"
                      />
                    </button>
                  </div>

                  <div
                    v-if="form.customMode === 'weekly'"
                    class="w-[140px] shrink-0 overflow-y-auto border-l border-[#dbe3ec] max-h-[336px]"
                  >
                    <button
                      v-for="item in WEEKDAY_OPTIONS"
                      :key="item.value"
                      type="button"
                      class="flex h-12 w-full items-center justify-between px-4 text-sm transition-colors"
                      :class="
                        selectedWeeklyDay === item.value
                          ? 'bg-[#f3f4f6] font-medium text-[#111827]'
                          : 'text-[#475569] hover:bg-[#f8fafc] hover:text-[#111827]'
                      "
                      @click="selectWeeklyDay(item.value)"
                    >
                      <span>{{ item.label }}</span>
                      <Check v-if="selectedWeeklyDay === item.value" class="size-4 shrink-0 text-[#2563eb]" />
                    </button>
                  </div>

                  <div
                    v-else-if="form.customMode === 'monthly'"
                    class="w-[140px] shrink-0 overflow-y-auto border-l border-[#dbe3ec] max-h-[336px]"
                  >
                    <button
                      v-for="day in monthlyOptions"
                      :key="day"
                      type="button"
                      class="flex h-12 w-full items-center px-4 text-sm transition-colors"
                      :class="
                        form.customDayOfMonth === day
                          ? 'bg-[#f3f4f6] font-medium text-[#111827]'
                          : 'text-[#475569] hover:bg-[#f8fafc] hover:text-[#111827]'
                      "
                      @click="selectMonthlyDay(day)"
                    >
                      {{ day }}号
                    </button>
                  </div>
                </div>

                <div
                  v-if="form.customMode === 'interval'"
                  class="flex min-h-12 items-center"
                >
                  <Input
                    v-model.number="form.customIntervalMinutes"
                    type="number"
                    min="1"
                    max="59"
                    step="1"
                    class="h-9 w-[120px] rounded-md border-[#dbe3ec] bg-white text-sm shadow-none"
                  />
                  <span class="ml-3 text-sm text-[#64748b]">分钟</span>
                </div>

                <Input
                  v-else-if="form.customMode === 'daily'"
                  v-model="customTimeValue"
                  type="time"
                  class="h-11 w-[156px] rounded-xl border-[#dbe3ec] bg-white px-4 text-sm"
                />
              </div>

              <Input
                v-if="form.customMode === 'weekly' || form.customMode === 'monthly'"
                v-model="customTimeValue"
                type="time"
                class="h-11 w-[156px] rounded-xl border-[#dbe3ec] bg-white px-4 text-sm"
              />
            </div>

            <div v-else class="space-y-2">
              <label class="block text-sm font-semibold text-[#334155]">Linux Crontab 代码</label>
              <textarea
                v-model="form.cronExpr"
                class="min-h-[108px] w-full rounded-xl border border-[#dbe3ec] bg-white px-4 py-3 font-mono text-sm leading-6 text-[#111827] outline-none transition-[border-color,box-shadow] placeholder:text-[#a0aec0] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
                placeholder="例如 0 9 * * *"
              />
            </div>
          </div>
        </section>

        <section class="flex items-center justify-between rounded-2xl border border-[#e5e7eb] bg-white px-4 py-4">
          <div class="space-y-1">
            <h3 class="text-[15px] font-semibold text-[#1f2937]">立即启用</h3>
            <p class="text-sm text-[#94a3b8]">创建后立即开始运行此任务</p>
          </div>
          <Switch
            :model-value="form.enabled"
            class="data-[state=checked]:bg-[#111827] data-[state=unchecked]:bg-[#cbd5e1]"
            @update:model-value="(value) => (form.enabled = !!value)"
          />
        </section>
        </div>
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
