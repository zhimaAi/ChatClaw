<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { onClickOutside, useEventListener } from '@vueuse/core'
import { CalendarDays, ChevronDown, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import CustomScheduleBuilder from '@/components/schedule/CustomScheduleBuilder.vue'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import type { OpenClawCronAgentOption } from '@bindings/chatclaw/internal/openclaw/cron'
import {
  addExpirationMonths,
  buildExpirationMonthOptions,
  buildExpirationYearOptions,
  setVisibleMonthYear,
  startOfExpirationMonth,
} from '@/pages/scheduled-tasks/components/taskFormExpirationCalendar'
import type {
  OpenClawCronEveryUnit,
  OpenClawCronFormState,
  OpenClawCronScheduleKind,
} from './utils'

const props = defineProps<{
  open: boolean
  saving: boolean
  title: string
  form: OpenClawCronFormState
  agents: OpenClawCronAgentOption[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()

const { t } = useI18n()
const SHOW_ADVANCED_SETTINGS = false
const LEGACY_CUSTOM_SCHEDULE_KIND = 'custom'
const SAFE_DATE_HOUR = 12
const SAFE_DATE_MINUTE = 0
const SAFE_DATE_SECOND = 0
const SAFE_DATE_MILLISECOND = 0
const ONE_TIME_HOUR_OPTIONS = Array.from({ length: 24 }, (_, index) => index)
const ONE_TIME_MINUTE_OPTIONS = Array.from({ length: 60 }, (_, index) => index)
const ONE_TIME_SECOND_OPTIONS = Array.from({ length: 60 }, (_, index) => index)
const FALLBACK_TIMEZONE = 'Asia/Shanghai'
const COMMON_TIMEZONE_OPTIONS = [
  'Asia/Shanghai',
  'Asia/Tokyo',
  'Asia/Singapore',
  'Europe/London',
  'Europe/Berlin',
  'America/New_York',
  'America/Los_Angeles',
  'UTC',
] as const

// Keep schedule option keys centralized so UI labels always come from i18n.
const BASE_SCHEDULE_KIND_OPTIONS = [
  { value: 'cron', labelKey: 'openclawCron.dialog.scheduleKinds.cron' },
  { value: 'every', labelKey: 'openclawCron.dialog.scheduleKinds.every' },
  { value: 'at', labelKey: 'openclawCron.dialog.scheduleKinds.at' },
] as const

const LEGACY_CUSTOM_SCHEDULE_KIND_OPTION = {
  value: LEGACY_CUSTOM_SCHEDULE_KIND,
  labelKey: 'openclawCron.dialog.scheduleKinds.custom',
} as const

const EVERY_UNIT_OPTIONS: Array<{ value: OpenClawCronEveryUnit; labelKey: string }> = [
  { value: 'seconds', labelKey: 'openclawCron.dialog.everyUnits.seconds' },
  { value: 'minutes', labelKey: 'openclawCron.dialog.everyUnits.minutes' },
  { value: 'hours', labelKey: 'openclawCron.dialog.everyUnits.hours' },
  { value: 'days', labelKey: 'openclawCron.dialog.everyUnits.days' },
]

const CUSTOM_MODE_OPTIONS = [
  { value: 'daily' as const, labelKey: 'scheduledTasks.dialog.customMode.daily' },
  { value: 'weekly' as const, labelKey: 'scheduledTasks.dialog.customMode.weekly' },
  { value: 'monthly' as const, labelKey: 'scheduledTasks.dialog.customMode.monthly' },
]

const agentOptions = computed(() => {
  const seen = new Set<string>()
  return props.agents.filter((agent) => {
    const agentID = String(agent.openclaw_agent_id || '').trim()
    if (!agentID || seen.has(agentID)) return false
    seen.add(agentID)
    return true
  })
})

const scheduleKindOptions = computed(() => {
  if (props.form.scheduleKind === LEGACY_CUSTOM_SCHEDULE_KIND) {
    return [...BASE_SCHEDULE_KIND_OPTIONS, LEGACY_CUSTOM_SCHEDULE_KIND_OPTION]
  }
  return BASE_SCHEDULE_KIND_OPTIONS
})

const canSubmit = computed(() => {
  if (!props.form.name.trim()) return false
  if (!props.form.message.trim() && !props.form.systemEvent.trim()) return false
  if (props.form.scheduleKind === 'cron' && !props.form.cronExpr.trim()) return false
  if (props.form.scheduleKind === 'every' && (!Number.isFinite(props.form.everyValue) || props.form.everyValue < 1)) return false
  if (props.form.scheduleKind === 'at' && !props.form.oneTimeDate.trim()) return false
  if (props.form.scheduleKind === 'custom' && props.form.customMode === 'weekly' && !props.form.customWeekdays.length) return false
  return true
})

function handleSubmit() {
  if (!canSubmit.value || props.saving) return
  emit('submit')
}

function selectScheduleKind(value: OpenClawCronScheduleKind) {
  props.form.scheduleKind = value
}

type CalendarDay = {
  key: string
  isoDate: string
  label: number
  inCurrentMonth: boolean
  isSelected: boolean
  isToday: boolean
}

function createSafeDate(year: number, month: number, day: number) {
  return new Date(
    year,
    month,
    day,
    SAFE_DATE_HOUR,
    SAFE_DATE_MINUTE,
    SAFE_DATE_SECOND,
    SAFE_DATE_MILLISECOND
  )
}

function padDatePart(value: number) {
  return String(value).padStart(2, '0')
}

function formatDateKey(date: Date) {
  return `${date.getFullYear()}-${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())}`
}

function parseDateKey(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) return null
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (!year || month < 1 || month > 12 || day < 1 || day > 31) return null
  const date = createSafeDate(year, month - 1, day)
  if (date.getFullYear() !== year || date.getMonth() !== month - 1 || date.getDate() !== day) return null
  return date
}

function buildCalendarDays(monthAnchor: Date, selectedDateKey: string): CalendarDay[] {
  const monthStart = startOfExpirationMonth(monthAnchor)
  const gridStart = createSafeDate(monthStart.getFullYear(), monthStart.getMonth(), 1 - monthStart.getDay())
  const todayKey = formatDateKey(new Date())

  return Array.from({ length: 42 }, (_, index) => {
    const current = createSafeDate(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + index)
    const isoDate = formatDateKey(current)
    return {
      key: isoDate,
      isoDate,
      label: current.getDate(),
      inCurrentMonth: current.getMonth() === monthStart.getMonth(),
      isSelected: isoDate === selectedDateKey,
      isToday: isoDate === todayKey,
    }
  })
}

const calendarWeekdayLabels = computed(() => ['日', '一', '二', '三', '四', '五', '六'])
const oneTimePickerOpen = ref(false)
const oneTimePickerRef = ref<HTMLElement | null>(null)
const visibleOneTimeMonth = ref(
  startOfExpirationMonth(parseDateKey(props.form.oneTimeDate) ?? new Date())
)
const oneTimeMonthOptions = buildExpirationMonthOptions()
const oneTimeYearOptions = computed(() => buildExpirationYearOptions(visibleOneTimeMonth.value))
const visibleOneTimeYear = computed(() => visibleOneTimeMonth.value.getFullYear())
const visibleOneTimeMonthValue = computed(() => visibleOneTimeMonth.value.getMonth() + 1)
const oneTimeCalendarWeeks = computed(() => {
  const days = buildCalendarDays(visibleOneTimeMonth.value, props.form.oneTimeDate)
  return Array.from({ length: 6 }, (_, index) => days.slice(index * 7, index * 7 + 7))
})
const oneTimeDisplayValue = computed(() => {
  if (!props.form.oneTimeDate) return ''
  return `${props.form.oneTimeDate} ${padDatePart(props.form.oneTimeHour)}:${padDatePart(props.form.oneTimeMinute)}:${padDatePart(props.form.oneTimeSecond)}`
})
const systemTimezone = resolveSystemTimezone()
const timezoneOptions = computed(() => {
  const allValues = typeof Intl.supportedValuesOf === 'function'
    ? Intl.supportedValuesOf('timeZone')
    : []
  const merged = new Set<string>([systemTimezone, ...COMMON_TIMEZONE_OPTIONS, ...allValues, props.form.timezone])
  return Array.from(merged).filter(Boolean)
})

function syncVisibleOneTimeMonth(value: string) {
  visibleOneTimeMonth.value = startOfExpirationMonth(parseDateKey(value) ?? new Date())
}

function openOneTimePicker() {
  syncVisibleOneTimeMonth(props.form.oneTimeDate)
  oneTimePickerOpen.value = true
}

function closeOneTimePicker() {
  oneTimePickerOpen.value = false
}

function toggleOneTimePicker() {
  if (oneTimePickerOpen.value) {
    closeOneTimePicker()
    return
  }
  openOneTimePicker()
}

function goToPreviousOneTimeMonth() {
  visibleOneTimeMonth.value = addExpirationMonths(visibleOneTimeMonth.value, -1)
}

function goToNextOneTimeMonth() {
  visibleOneTimeMonth.value = addExpirationMonths(visibleOneTimeMonth.value, 1)
}

function handleOneTimeYearChange(value: string) {
  const year = Number(value)
  if (!Number.isInteger(year)) return
  visibleOneTimeMonth.value = setVisibleMonthYear(visibleOneTimeMonth.value, year)
}

function handleOneTimeMonthChange(value: string) {
  const month = Number(value)
  if (!Number.isInteger(month)) return
  visibleOneTimeMonth.value = setVisibleMonthYear(visibleOneTimeMonth.value, undefined, month)
}

function selectOneTimeDate(value: string) {
  props.form.oneTimeDate = value
  syncVisibleOneTimeMonth(value)
  closeOneTimePicker()
}

function clearOneTimeDate() {
  props.form.oneTimeDate = ''
  syncVisibleOneTimeMonth('')
  closeOneTimePicker()
}

function resolveSystemTimezone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || FALLBACK_TIMEZONE
  } catch {
    return FALLBACK_TIMEZONE
  }
}

watch(
  () => props.form.oneTimeDate,
  (value) => {
    if (oneTimePickerOpen.value) return
    syncVisibleOneTimeMonth(value)
  }
)

onClickOutside(oneTimePickerRef, () => {
  if (!oneTimePickerOpen.value) return
  closeOneTimePicker()
})

useEventListener(window, 'keydown', (event) => {
  if (event.key === 'Escape' && oneTimePickerOpen.value) {
    closeOneTimePicker()
  }
})
</script>

<template>
  <Dialog :open="open" @update:open="(value) => emit('update:open', value)">
    <DialogContent
      size="xl"
      :show-close-button="true"
      class="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 !max-w-[780px] shadow-lg dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <DialogHeader class="flex shrink-0 flex-row items-baseline gap-2 border-b border-border px-6 pb-3 pt-4">
        <DialogTitle class="text-xl font-semibold text-[#0a0a0a] dark:text-foreground">
          {{ title }}
        </DialogTitle>
        <p class="text-sm text-muted-foreground">
          {{ t('openclawCron.dialog.subtitle', '安排自动化的 AI 任务') }}
        </p>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto px-6 py-4">
        <div class="space-y-6">
          <section class="space-y-4">
            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.name', '任务名称') }}
              </Label>
              <Input v-model="form.name" :placeholder="t('openclawCron.dialog.namePlaceholder', '例如：日报总结')" class="h-10" />
            </div>

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.description', '描述') }}
              </Label>
              <Input v-model="form.description" :placeholder="t('openclawCron.dialog.descriptionPlaceholder', '可选，用于补充说明')" class="h-10" />
            </div>

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.message', '消息提示词') }}
              </Label>
              <textarea
                v-model="form.message"
                class="min-h-[118px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm leading-6 text-foreground shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                :placeholder="t('openclawCron.dialog.messagePlaceholder', 'AI 应该做什么？例如：给我一份今天的新闻和天气摘要')"
              />
            </div>

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.agent', '关联助手') }}
              </Label>
              <div class="relative">
                <select
                  v-model="form.agentId"
                  class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                >
                  <option value="">{{ t('openclawCron.dialog.useDefaultAgent', '未指定（使用默认助手）') }}</option>
                  <option
                    v-for="agent in agentOptions"
                    :key="agent.openclaw_agent_id"
                    :value="agent.openclaw_agent_id"
                  >
                    {{ agent.name }}
                  </option>
                </select>
                <ChevronDown class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              </div>
            </div>
          </section>

          <section class="space-y-4">
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.scheduleTitle', '时间配置') }}
              </h3>
              <p class="text-sm text-muted-foreground">
                {{ t('openclawCron.dialog.scheduleHint', '选择执行方式，并设置任务运行时间。') }}
              </p>
            </div>

            <div class="flex flex-wrap gap-2">
              <button
                v-for="item in scheduleKindOptions"
                :key="item.value"
                type="button"
                class="inline-flex h-9 items-center rounded-md border px-3.5 text-sm font-medium transition-colors"
                :class="
                  form.scheduleKind === item.value
                    ? 'border-border bg-muted text-foreground dark:border-white/10'
                    : 'border-border bg-background text-muted-foreground hover:bg-muted/50 hover:text-foreground dark:border-white/10'
                "
                @click="selectScheduleKind(item.value as OpenClawCronScheduleKind)"
              >
                {{ t(item.labelKey) }}
              </button>
            </div>

            <div class="rounded-lg border border-border bg-muted/20 p-4 dark:border-white/10 dark:bg-white/5">
              <div v-if="form.scheduleKind === 'cron'" class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.scheduleKinds.cron', 'Cron 表达式') }}
                </Label>
                <textarea
                  v-model="form.cronExpr"
                  class="min-h-[88px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 font-mono text-sm leading-6 text-foreground shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                  :placeholder="t('openclawCron.dialog.cronPlaceholder', '例如：0 9 * * *')"
                />
              </div>

              <div v-else-if="form.scheduleKind === 'every'" class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.scheduleKinds.every', '固定间隔') }}
                </Label>
                <div class="flex flex-wrap items-center gap-3">
                  <Input
                    v-model.number="form.everyValue"
                    type="number"
                    min="1"
                    step="1"
                    class="h-10 w-[140px]"
                  />
                  <div class="relative">
                    <select
                      v-model="form.everyUnit"
                      class="h-10 min-w-[120px] appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                    >
                      <option
                        v-for="item in EVERY_UNIT_OPTIONS"
                        :key="item.value"
                        :value="item.value"
                      >
                        {{ t(item.labelKey) }}
                      </option>
                    </select>
                    <ChevronDown
                      class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                    />
                  </div>
                </div>
                <p class="text-xs text-muted-foreground">
                  {{ t('openclawCron.dialog.everyHint', '例如：30 秒、10 分钟、2 小时、1 天') }}
                </p>
              </div>

              <div v-else-if="form.scheduleKind === 'custom'" class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.scheduleKinds.custom', '自定义时间') }}
                </Label>
                <CustomScheduleBuilder
                  :form="form"
                  :mode-options="CUSTOM_MODE_OPTIONS"
                  :show-interval-input="false"
                />
              </div>

              <div v-else-if="form.scheduleKind === 'at'" class="space-y-4">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.scheduleKinds.at', '一次性时间') }}
                </Label>
                <div ref="oneTimePickerRef" class="relative">
                  <button
                    type="button"
                    class="group flex h-11 w-full items-center justify-between rounded-xl border px-4 text-left text-sm outline-none transition-[border-color,box-shadow,background-color]"
                    :class="
                      oneTimePickerOpen
                        ? 'border-[#2563eb] bg-white shadow-[0_0_0_4px_rgba(191,219,254,0.9)]'
                        : 'border-[#dbe3ec] bg-white hover:border-[#cbd5e1]'
                    "
                    @click="toggleOneTimePicker"
                  >
                    <div class="flex min-w-0 flex-1 items-center gap-3">
                      <div
                        class="flex size-8 shrink-0 items-center justify-center rounded-lg border"
                        :class="
                          oneTimePickerOpen
                            ? 'border-[#bfdbfe] bg-[#eff6ff] text-[#2563eb]'
                            : 'border-[#e2e8f0] bg-[#f8fafc] text-[#64748b]'
                        "
                      >
                        <CalendarDays class="size-4" />
                      </div>
                      <div class="min-w-0">
                        <p class="truncate font-medium text-[#111827]">
                          {{ oneTimeDisplayValue || t('openclawCron.dialog.selectOneTime', '选择一次性时间') }}
                        </p>
                      </div>
                    </div>
                    <ChevronDown
                      class="size-4 shrink-0 text-[#94a3b8] transition-transform"
                      :class="oneTimePickerOpen ? 'rotate-180 text-[#2563eb]' : ''"
                    />
                  </button>

                  <div
                    v-if="oneTimePickerOpen"
                    class="absolute left-0 z-50 mt-3 w-full min-w-[320px] overflow-hidden rounded-[22px] border border-[#dbe3ec] bg-[linear-gradient(180deg,#ffffff_0%,#f8fbff_100%)] shadow-[0_22px_60px_rgba(15,23,42,0.18)] backdrop-blur"
                  >
                    <div class="border-b border-[#e5eef8] px-4 pb-4 pt-4">
                      <div class="flex items-center justify-between gap-3">
                        <div class="flex items-center gap-2">
                          <button
                            type="button"
                            class="inline-flex size-9 items-center justify-center rounded-full border border-[#dbe3ec] bg-white text-[#475569] transition-colors hover:border-[#bfdbfe] hover:bg-[#eff6ff] hover:text-[#2563eb]"
                            @click="goToPreviousOneTimeMonth"
                          >
                            <ChevronLeft class="size-4" />
                          </button>
                          <button
                            type="button"
                            class="inline-flex size-9 items-center justify-center rounded-full border border-[#dbe3ec] bg-white text-[#475569] transition-colors hover:border-[#bfdbfe] hover:bg-[#eff6ff] hover:text-[#2563eb]"
                            @click="goToNextOneTimeMonth"
                          >
                            <ChevronRight class="size-4" />
                          </button>
                        </div>
                        <div class="flex items-center gap-2">
                          <div class="relative">
                            <select
                              :value="visibleOneTimeYear"
                              class="h-9 appearance-none rounded-full border border-[#dbe3ec] bg-white pl-4 pr-9 text-sm font-medium text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
                              @change="handleOneTimeYearChange(($event.target as HTMLSelectElement).value)"
                            >
                              <option v-for="year in oneTimeYearOptions" :key="year" :value="year">
                                {{ year }} 年
                              </option>
                            </select>
                            <ChevronDown
                              class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]"
                            />
                          </div>
                          <div class="relative">
                            <select
                              :value="visibleOneTimeMonthValue"
                              class="h-9 appearance-none rounded-full border border-[#dbe3ec] bg-white pl-4 pr-9 text-sm font-medium text-[#111827] outline-none transition-[border-color,box-shadow] focus:border-[#2563eb] focus:ring-4 focus:ring-[#dbeafe]"
                              @change="handleOneTimeMonthChange(($event.target as HTMLSelectElement).value)"
                            >
                              <option v-for="month in oneTimeMonthOptions" :key="month" :value="month">
                                {{ month }} 月
                              </option>
                            </select>
                            <ChevronDown
                              class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-[#94a3b8]"
                            />
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="px-4 pb-4 pt-3">
                      <div class="mb-2 grid grid-cols-7 gap-1">
                        <span
                          v-for="weekday in calendarWeekdayLabels"
                          :key="weekday"
                          class="flex h-8 items-center justify-center text-xs font-semibold text-[#94a3b8]"
                        >
                          {{ weekday }}
                        </span>
                      </div>

                      <div class="space-y-1">
                        <div
                          v-for="(week, weekIndex) in oneTimeCalendarWeeks"
                          :key="'week-' + weekIndex"
                          class="grid grid-cols-7 gap-1"
                        >
                          <button
                            v-for="day in week"
                            :key="day.key"
                            type="button"
                            :class="
                              cn(
                                'flex h-10 items-center justify-center rounded-xl border text-sm font-medium transition-all',
                                day.isSelected
                                  ? 'border-[#111827] bg-[#111827] text-white shadow-[0_10px_22px_rgba(15,23,42,0.18)]'
                                  : day.isToday
                                    ? 'border-[#bfdbfe] bg-[#eff6ff] text-[#1d4ed8]'
                                    : day.inCurrentMonth
                                      ? 'border-transparent bg-white text-[#334155] hover:border-[#dbe3ec] hover:bg-[#f8fafc]'
                                      : 'border-transparent bg-transparent text-[#c0cad6] hover:bg-[#f8fafc]',
                              )
                            "
                            @click="selectOneTimeDate(day.isoDate)"
                          >
                            {{ day.label }}
                          </button>
                        </div>
                      </div>

                      <div class="mt-4 grid gap-3 md:grid-cols-3">
                        <div class="space-y-1.5">
                          <Label class="text-xs font-medium text-muted-foreground">
                            {{ t('openclawCron.dialog.hour', '小时') }}
                          </Label>
                          <div class="relative">
                            <select
                              v-model.number="form.oneTimeHour"
                              class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                            >
                              <option v-for="hour in ONE_TIME_HOUR_OPTIONS" :key="hour" :value="hour">
                                {{ padDatePart(hour) }}
                              </option>
                            </select>
                            <ChevronDown
                              class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                            />
                          </div>
                        </div>
                        <div class="space-y-1.5">
                          <Label class="text-xs font-medium text-muted-foreground">
                            {{ t('openclawCron.dialog.minute', '分钟') }}
                          </Label>
                          <div class="relative">
                            <select
                              v-model.number="form.oneTimeMinute"
                              class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                            >
                              <option
                                v-for="minute in ONE_TIME_MINUTE_OPTIONS"
                                :key="minute"
                                :value="minute"
                              >
                                {{ padDatePart(minute) }}
                              </option>
                            </select>
                            <ChevronDown
                              class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                            />
                          </div>
                        </div>
                        <div class="space-y-1.5">
                          <Label class="text-xs font-medium text-muted-foreground">
                            {{ t('openclawCron.dialog.second', '秒') }}
                          </Label>
                          <div class="relative">
                            <select
                              v-model.number="form.oneTimeSecond"
                              class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                            >
                              <option
                                v-for="second in ONE_TIME_SECOND_OPTIONS"
                                :key="second"
                                :value="second"
                              >
                                {{ padDatePart(second) }}
                              </option>
                            </select>
                            <ChevronDown
                              class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                            />
                          </div>
                        </div>
                      </div>
                    </div>

                    <div class="flex items-center justify-end border-t border-[#e5eef8] bg-white/80 px-4 py-3">
                      <button
                        type="button"
                        class="inline-flex items-center rounded-full px-3 py-1.5 text-sm font-medium text-[#64748b] transition-colors hover:bg-[#f1f5f9]"
                        @click="clearOneTimeDate"
                      >
                        {{ t('scheduledTasks.form.clear', '清空') }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <div v-else class="space-y-2">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.scheduleKinds.custom', '自定义时间') }}
                </Label>
                <CustomScheduleBuilder
                  :form="form"
                  :mode-options="CUSTOM_MODE_OPTIONS"
                  :show-interval-input="false"
                />
              </div>

              <div class="mt-4 grid gap-4 md:grid-cols-2">
                <div class="space-y-1.5">
                  <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                    {{ t('openclawCron.dialog.timezone', '时区') }}
                  </Label>
                  <div class="relative">
                    <select
                      v-model="form.timezone"
                      class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                    >
                      <option v-for="timezone in timezoneOptions" :key="timezone" :value="timezone">
                        {{ timezone }}{{ timezone === systemTimezone ? ` (${t('openclawCron.dialog.systemTimezone', '系统默认')})` : '' }}
                      </option>
                    </select>
                    <ChevronDown
                      class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                    />
                  </div>
                </div>
                <div class="flex items-end justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                  <div>
                    <div class="text-sm font-medium text-foreground">
                      {{ t('openclawCron.dialog.exact', '精确执行') }}
                    </div>
                    <div class="text-xs text-muted-foreground">
                      {{ t('openclawCron.dialog.exactHint', '启用 OpenClaw exact 模式，尽量按设定时刻触发') }}
                    </div>
                  </div>
                  <Switch :model-value="form.exact" @update:model-value="(value) => (form.exact = !!value)" />
                </div>
              </div>
            </div>
          </section>

          <section class="space-y-4">
            

            <div class="space-y-1.5">
              <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.systemEvent', '系统事件') }}
              </Label>
              <Input
                v-model="form.systemEvent"
                :placeholder="t('openclawCron.dialog.systemEventPlaceholder', '可选，用于传递 OpenClaw 系统事件载荷')"
                class="h-10"
              />
            </div>
          </section>

          <section
            class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-4 dark:border-white/10"
          >
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[#0a0a0a] dark:text-foreground">
                {{ t('openclawCron.dialog.enableNowTitle', '立即启用') }}
              </h3>
              <p class="text-sm text-muted-foreground">
                {{ t('openclawCron.dialog.enableNowHint', '创建后立即开始运行此任务') }}
              </p>
            </div>
            <Switch :model-value="form.enabled" @update:model-value="(value) => (form.enabled = !!value)" />
          </section>

          <section
            v-if="SHOW_ADVANCED_SETTINGS"
            class="rounded-lg border border-border bg-card dark:border-white/10"
          >
            <button
              type="button"
              class="flex w-full items-center justify-between px-4 py-3 text-left"
            >
              <div>
                <div class="text-sm font-semibold text-foreground">
                  {{ t('openclawCron.dialog.advanced', '高级设置') }}
                </div>
                <div class="text-xs text-muted-foreground">
                  {{ t('openclawCron.dialog.advancedHint', '模型、投递、会话、超时等额外配置') }}
                </div>
              </div>
              <ChevronDown class="size-4 text-muted-foreground" />
            </button>

            <div class="grid gap-4 border-t border-border px-4 py-4 md:grid-cols-2 dark:border-white/10">
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.thinking', '思考强度') }}
                </Label>
                <select
                  v-model="form.thinking"
                  class="h-10 w-full rounded-md border border-input bg-transparent px-3 text-sm text-foreground shadow-xs outline-none dark:bg-input/30"
                >
                  <option value="off">off</option>
                  <option value="minimal">minimal</option>
                  <option value="low">low</option>
                  <option value="medium">medium</option>
                  <option value="high">high</option>
                  <option value="xhigh">xhigh</option>
                </select>
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.sessionTarget', '会话目标') }}
                </Label>
                <select
                  v-model="form.sessionTarget"
                  class="h-10 w-full rounded-md border border-input bg-transparent px-3 text-sm text-foreground shadow-xs outline-none dark:bg-input/30"
                >
                  <option value="isolated">isolated</option>
                  <option value="main">main</option>
                </select>
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.sessionKey', '会话键') }}
                </Label>
                <Input
                  v-model="form.sessionKey"
                  :placeholder="t('openclawCron.dialog.sessionKeyPlaceholder', '例如：agent:main:my-session')"
                  class="h-10"
                />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.wakeMode', '唤醒模式') }}
                </Label>
                <select
                  v-model="form.wakeMode"
                  class="h-10 w-full rounded-md border border-input bg-transparent px-3 text-sm text-foreground shadow-xs outline-none dark:bg-input/30"
                >
                  <option value="now">now</option>
                  <option value="next-heartbeat">next-heartbeat</option>
                </select>
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.timeoutMs', '超时时间（毫秒）') }}
                </Label>
                <Input v-model.number="form.timeoutMs" type="number" min="1000" class="h-10" />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.deliveryChannel', '投递通道') }}
                </Label>
                <Input
                  v-model="form.deliveryChannel"
                  :placeholder="t('openclawCron.dialog.deliveryChannelPlaceholder', '例如：last')"
                  class="h-10"
                />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.deliveryTo', '投递目标') }}
                </Label>
                <Input
                  v-model="form.deliveryTo"
                  :placeholder="t('openclawCron.dialog.deliveryToPlaceholder', '目标会话或用户')"
                  class="h-10"
                />
              </div>
              <div class="space-y-1.5">
                <Label class="text-sm font-medium text-[#0a0a0a] dark:text-foreground">
                  {{ t('openclawCron.dialog.deliveryAccountId', '账号 ID') }}
                </Label>
                <Input
                  v-model="form.deliveryAccountId"
                  :placeholder="t('openclawCron.dialog.deliveryAccountIdPlaceholder', '多账号场景下可选')"
                  class="h-10"
                />
              </div>

              <div class="space-y-3 md:col-span-2">
                <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.announce', '发送公告') }}</span>
                    <Switch :model-value="form.announce" @update:model-value="(value) => (form.announce = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.expectFinal', '等待最终结果') }}</span>
                    <Switch :model-value="form.expectFinal" @update:model-value="(value) => (form.expectFinal = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.lightContext', '轻量上下文') }}</span>
                    <Switch :model-value="form.lightContext" @update:model-value="(value) => (form.lightContext = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.bestEffortDeliver', '尽力投递') }}</span>
                    <Switch :model-value="form.bestEffortDeliver" @update:model-value="(value) => (form.bestEffortDeliver = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.deleteAfterRun', '运行后删除') }}</span>
                    <Switch :model-value="form.deleteAfterRun" @update:model-value="(value) => (form.deleteAfterRun = !!value)" />
                  </label>
                  <label class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-3 dark:border-white/10">
                    <span class="text-sm text-foreground">{{ t('openclawCron.dialog.keepAfterRun', '运行后保留') }}</span>
                    <Switch :model-value="form.keepAfterRun" @update:model-value="(value) => (form.keepAfterRun = !!value)" />
                  </label>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>

      <DialogFooter class="shrink-0 gap-2 border-t border-border px-6 py-4 sm:justify-end">
        <Button type="button" variant="outline" @click="emit('update:open', false)">
          {{ t('common.cancel') }}
        </Button>
        <Button type="button" :disabled="!canSubmit || saving" @click="handleSubmit">
          {{ saving ? t('common.loading', '处理中...') : t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
