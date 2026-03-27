<script setup lang="ts">
import { computed } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Input } from '@/components/ui/input'
import { WEEKDAY_OPTIONS } from '@/pages/scheduled-tasks/constants'

type CustomScheduleMode = 'interval' | 'daily' | 'weekly' | 'monthly'

interface CustomScheduleFormLike {
  customMode: CustomScheduleMode
  customHour: number
  customMinute: number
  customIntervalMinutes: number
  customWeekdays: number[]
  customDayOfMonth: number
}

const DEFAULT_MODE_OPTIONS = [
  { value: 'interval' as const, labelKey: 'scheduledTasks.dialog.customMode.interval' },
  { value: 'daily' as const, labelKey: 'scheduledTasks.dialog.customMode.daily' },
  { value: 'weekly' as const, labelKey: 'scheduledTasks.dialog.customMode.weekly' },
  { value: 'monthly' as const, labelKey: 'scheduledTasks.dialog.customMode.monthly' },
] as const

const props = withDefaults(
  defineProps<{
    form: CustomScheduleFormLike
    readonly?: boolean
    modeOptions?: ReadonlyArray<{ value: CustomScheduleMode; labelKey: string }>
    showIntervalInput?: boolean
  }>(),
  {
    readonly: false,
    showIntervalInput: true,
  }
)

const { t } = useI18n()
const monthlyOptions = Array.from({ length: 31 }, (_, index) => index + 1)
const resolvedModeOptions = computed(() => props.modeOptions ?? DEFAULT_MODE_OPTIONS)

const customTimeValue = computed({
  get() {
    return `${String(props.form.customHour).padStart(2, '0')}:${String(props.form.customMinute).padStart(2, '0')}`
  },
  set(value: string) {
    if (props.readonly) return
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
    if (props.readonly) return
    props.form.customWeekdays = [value]
  },
})

function selectCustomMode(value: CustomScheduleMode) {
  if (props.readonly) return
  props.form.customMode = value
  if (value === 'weekly' && props.form.customWeekdays.length === 0) {
    props.form.customWeekdays = [1]
  }
}

function selectWeeklyDay(value: number) {
  if (props.readonly) return
  selectedWeeklyDay.value = value
}

function selectMonthlyDay(value: number) {
  if (props.readonly) return
  props.form.customDayOfMonth = value
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-start gap-5">
      <div class="inline-flex overflow-hidden rounded-2xl border border-[#dbe3ec] bg-white">
        <div class="w-[140px] shrink-0">
          <button
            v-for="item in resolvedModeOptions"
            :key="item.value"
            type="button"
            :disabled="readonly"
            class="flex h-12 w-full items-center justify-between px-4 text-sm font-medium transition-colors disabled:cursor-default"
            :class="
              form.customMode === item.value ? 'bg-[#f3f4f6] text-[#111827]' : 'text-[#475569]'
            "
            @click="selectCustomMode(item.value)"
          >
            <span>{{ t(item.labelKey) }}</span>
            <ChevronDown
              v-if="item.value !== 'daily' && item.value !== 'interval'"
              class="size-4 shrink-0 rotate-[-90deg] text-[#94a3b8]"
              :class="form.customMode === item.value ? 'opacity-100' : 'opacity-40'"
            />
          </button>
        </div>

        <div
          v-if="form.customMode === 'weekly'"
          class="max-h-[336px] w-[140px] shrink-0 overflow-y-auto border-l border-[#dbe3ec]"
        >
          <button
            v-for="item in WEEKDAY_OPTIONS"
            :key="item.value"
            type="button"
            :disabled="readonly"
            class="flex h-12 w-full items-center justify-between px-4 text-sm transition-colors disabled:cursor-default"
            :class="
              selectedWeeklyDay === item.value
                ? 'bg-[#f3f4f6] font-medium text-[#111827]'
                : 'text-[#475569]'
            "
            @click="selectWeeklyDay(item.value)"
          >
            <span>{{ t(item.labelKey) }}</span>
            <Check
              v-if="selectedWeeklyDay === item.value"
              class="size-4 shrink-0 text-[#2563eb]"
            />
          </button>
        </div>

        <div
          v-else-if="form.customMode === 'monthly'"
          class="max-h-[336px] w-[140px] shrink-0 overflow-y-auto border-l border-[#dbe3ec]"
        >
          <button
            v-for="day in monthlyOptions"
            :key="day"
            type="button"
            :disabled="readonly"
            class="flex h-12 w-full items-center px-4 text-sm transition-colors disabled:cursor-default"
            :class="
              form.customDayOfMonth === day
                ? 'bg-[#f3f4f6] font-medium text-[#111827]'
                : 'text-[#475569]'
            "
            @click="selectMonthlyDay(day)"
          >
            {{ t('scheduledTasks.dialog.monthlyDay', { day }) }}
          </button>
        </div>
      </div>

      <div
        v-if="form.customMode === 'interval' && showIntervalInput"
        class="flex min-h-12 items-center"
      >
        <Input
          v-model.number="form.customIntervalMinutes"
          :readonly="readonly"
          type="number"
          min="1"
          max="59"
          step="1"
          class="h-9 w-[120px] rounded-md border-[#dbe3ec] bg-white text-sm shadow-none read-only:bg-[#f8fafc] read-only:text-[#475569]"
        />
        <span class="ml-3 text-sm text-[#64748b]">
          {{ t('scheduledTasks.dialog.minutes') }}
        </span>
      </div>

      <Input
        v-else-if="form.customMode === 'daily'"
        v-model="customTimeValue"
        :readonly="readonly"
        type="time"
        class="h-11 w-[156px] rounded-xl border-[#dbe3ec] bg-white px-4 text-sm read-only:bg-[#f8fafc] read-only:text-[#475569]"
      />
    </div>

    <Input
      v-if="form.customMode === 'weekly' || form.customMode === 'monthly'"
      v-model="customTimeValue"
      :readonly="readonly"
      type="time"
      class="h-11 w-[156px] rounded-xl border-[#dbe3ec] bg-white px-4 text-sm read-only:bg-[#f8fafc] read-only:text-[#475569]"
    />
  </div>
</template>
