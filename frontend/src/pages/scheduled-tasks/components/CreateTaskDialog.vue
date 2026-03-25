<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, ChevronDown, Clock3 } from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
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

const { t } = useI18n()

const scheduleTypeOptions = [
  { value: 'preset', labelKey: 'scheduledTasks.dialog.scheduleType.preset' },
  { value: 'custom', labelKey: 'scheduledTasks.dialog.scheduleType.custom' },
  { value: 'cron', labelKey: 'scheduledTasks.dialog.scheduleType.cron' },
] as const

const customModeOptions = [
  { value: 'interval', labelKey: 'scheduledTasks.dialog.customMode.interval' },
  { value: 'daily', labelKey: 'scheduledTasks.dialog.customMode.daily' },
  { value: 'weekly', labelKey: 'scheduledTasks.dialog.customMode.weekly' },
  { value: 'monthly', labelKey: 'scheduledTasks.dialog.customMode.monthly' },
] as const

const dialogSubtitle = computed(() =>
  props.form.id
    ? t('scheduledTasks.dialog.subtitleEdit')
    : t('scheduledTasks.dialog.subtitleCreate')
)
const monthlyOptions = Array.from({ length: 31 }, (_, index) => index + 1)
const submitLock = createSubmitLock()
const submitLocked = ref(false)

const canSubmit = computed(() => {
  if (!props.form.name.trim()) return false
  if (!props.form.prompt.trim()) return false
  if (props.form.agentId == null || props.form.agentId <= 0) return false

  if (props.form.scheduleType === 'cron' && !props.form.cronExpr.trim()) return false

  if (props.form.scheduleType === 'custom' && props.form.customMode === 'interval') {
    const n = Number(props.form.customIntervalMinutes)
    if (!Number.isFinite(n) || n < 1 || n > 59) return false
  }

  if (
    props.form.scheduleType === 'custom' &&
    props.form.customMode === 'weekly' &&
    !props.form.customWeekdays?.length
  ) {
    return false
  }

  return true
})

const submitActionDisabled = computed(() => !canSubmit.value || props.saving || submitLocked.value)

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
  if (
    value === 'custom' &&
    props.form.customMode === 'weekly' &&
    props.form.customWeekdays.length === 0
  ) {
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
  if (!canSubmit.value || props.saving) return

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
      size="xl"
      :show-close-button="true"
      class="flex max-h-[90vh] flex-col gap-0 overflow-hidden p-0 !max-w-[780px] shadow-lg dark:shadow-none dark:ring-1 dark:ring-white/10"
    >
      <DialogHeader
        class="flex shrink-0 flex-row flex-wrap items-baseline gap-2 border-b border-border px-6 pb-3 pt-4"
      >
        <DialogTitle class="text-xl font-semibold text-[#0a0a0a] dark:text-foreground">
          {{ title }}
        </DialogTitle>
        <p class="text-sm text-muted-foreground">
          {{ dialogSubtitle }}
        </p>
      </DialogHeader>

      <div class="flex-1 overflow-y-auto px-6">
        <div class="space-y-6 py-4">
          <section class="space-y-4">
            <div class="space-y-1.5">
              <Label
                for="scheduled-task-name"
                class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
              >
                <span class="text-destructive" aria-hidden="true">*</span>
                {{ t('scheduledTasks.dialog.nameLabel') }}
              </Label>
              <Input
                id="scheduled-task-name"
                v-model="form.name"
                :placeholder="t('scheduledTasks.dialog.namePlaceholder')"
                class="h-10"
              />
            </div>

            <div class="space-y-1.5">
              <Label
                for="scheduled-task-prompt"
                class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
              >
                <span class="text-destructive" aria-hidden="true">*</span>
                {{ t('scheduledTasks.dialog.promptLabel') }}
              </Label>
              <textarea
                id="scheduled-task-prompt"
                v-model="form.prompt"
                class="min-h-[118px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm leading-6 text-foreground shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                :placeholder="t('scheduledTasks.dialog.promptPlaceholder')"
              />
            </div>

            <div class="space-y-1.5">
              <Label
                for="scheduled-task-agent"
                class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
              >
                <span class="text-destructive" aria-hidden="true">*</span>
                {{ t('scheduledTasks.dialog.agentLabel') }}
              </Label>
              <div class="relative">
                <select
                  id="scheduled-task-agent"
                  v-model.number="form.agentId"
                  class="h-10 w-full appearance-none rounded-md border border-input bg-transparent px-3 pr-10 text-sm text-foreground shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                >
                  <option :value="null">{{ t('scheduledTasks.dialog.selectAgent') }}</option>
                  <option v-for="agent in agents" :key="agent.id" :value="agent.id">
                    {{ agent.name }}
                  </option>
                </select>
                <ChevronDown
                  class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
                />
              </div>
            </div>
          </section>

          <section class="space-y-4">
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[#0a0a0a] dark:text-foreground">
                {{ t('scheduledTasks.dialog.scheduleTitle') }}
              </h3>
              <p class="text-sm text-muted-foreground">
                {{ t('scheduledTasks.dialog.scheduleHint') }}
              </p>
            </div>

            <div class="flex flex-wrap gap-2">
              <button
                v-for="option in scheduleTypeOptions"
                :key="option.value"
                type="button"
                class="inline-flex h-9 items-center rounded-md border px-3.5 text-sm font-medium transition-colors"
                :class="
                  form.scheduleType === option.value
                    ? 'border-border bg-muted text-foreground dark:border-white/10'
                    : 'border-border bg-background text-muted-foreground hover:bg-muted/50 hover:text-foreground dark:border-white/10'
                "
                @click="selectScheduleType(option.value)"
              >
                {{ t(option.labelKey) }}
              </button>
            </div>

            <div
              class="rounded-lg border border-border bg-muted/20 p-4 dark:border-white/10 dark:bg-white/5"
            >
              <div v-if="form.scheduleType === 'preset'" class="space-y-2">
                <span class="block text-sm font-medium text-foreground">
                  {{ t('scheduledTasks.dialog.presetLabel') }}
                </span>
                <div class="grid gap-3 md:grid-cols-2">
                  <button
                    v-for="item in SCHEDULE_PRESETS"
                    :key="item.value"
                    type="button"
                    class="flex min-h-11 items-center gap-3 rounded-lg border bg-card px-4 py-3 text-left text-sm transition-colors dark:border-white/10"
                    :class="
                      form.schedulePreset === item.value
                        ? 'border-foreground/25 bg-muted/40 text-foreground'
                        : 'border-border text-foreground/90 hover:bg-muted/30 dark:border-white/10'
                    "
                    @click="form.schedulePreset = item.value"
                  >
                    <Clock3 class="size-4 shrink-0 text-muted-foreground" />
                    <span class="flex-1 font-medium">{{ t(item.labelKey) }}</span>
                    <Check
                      v-if="form.schedulePreset === item.value"
                      class="size-4 shrink-0 text-primary"
                    />
                  </button>
                </div>
              </div>

              <div v-else-if="form.scheduleType === 'custom'" class="space-y-4">
                <div class="flex flex-wrap items-start gap-4">
                  <div
                    class="inline-flex overflow-hidden rounded-lg border border-border bg-card dark:border-white/10"
                  >
                    <div class="w-[140px] shrink-0">
                      <button
                        v-for="item in customModeOptions"
                        :key="item.value"
                        type="button"
                        class="flex h-12 w-full items-center justify-between px-4 text-sm font-medium transition-colors"
                        :class="
                          form.customMode === item.value
                            ? 'bg-muted text-foreground'
                            : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                        "
                        @click="selectCustomMode(item.value)"
                      >
                        <span>{{ t(item.labelKey) }}</span>
                        <ChevronDown
                          v-if="item.value !== 'daily' && item.value !== 'interval'"
                          class="size-4 shrink-0 rotate-[-90deg] text-muted-foreground"
                          :class="form.customMode === item.value ? 'opacity-100' : 'opacity-40'"
                        />
                      </button>
                    </div>

                    <div
                      v-if="form.customMode === 'weekly'"
                      class="max-h-[336px] w-[140px] shrink-0 overflow-y-auto border-l border-border dark:border-white/10"
                    >
                      <button
                        v-for="item in WEEKDAY_OPTIONS"
                        :key="item.value"
                        type="button"
                        class="flex h-12 w-full items-center justify-between px-4 text-sm transition-colors"
                        :class="
                          selectedWeeklyDay === item.value
                            ? 'bg-muted font-medium text-foreground'
                            : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                        "
                        @click="selectWeeklyDay(item.value)"
                      >
                        <span>{{ t(item.labelKey) }}</span>
                        <Check
                          v-if="selectedWeeklyDay === item.value"
                          class="size-4 shrink-0 text-primary"
                        />
                      </button>
                    </div>

                    <div
                      v-else-if="form.customMode === 'monthly'"
                      class="max-h-[336px] w-[140px] shrink-0 overflow-y-auto border-l border-border dark:border-white/10"
                    >
                      <button
                        v-for="day in monthlyOptions"
                        :key="day"
                        type="button"
                        class="flex h-12 w-full items-center px-4 text-sm transition-colors"
                        :class="
                          form.customDayOfMonth === day
                            ? 'bg-muted font-medium text-foreground'
                            : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                        "
                        @click="selectMonthlyDay(day)"
                      >
                        {{ t('scheduledTasks.dialog.monthlyDay', { day }) }}
                      </button>
                    </div>
                  </div>

                  <div v-if="form.customMode === 'interval'" class="flex min-h-12 items-center">
                    <Input
                      v-model.number="form.customIntervalMinutes"
                      type="number"
                      min="1"
                      max="59"
                      step="1"
                      class="h-9 w-[120px]"
                    />
                    <span class="ml-3 text-sm text-muted-foreground">
                      {{ t('scheduledTasks.dialog.minutes') }}</span
                    >
                  </div>

                  <Input
                    v-else-if="form.customMode === 'daily'"
                    v-model="customTimeValue"
                    type="time"
                    class="h-10 w-[156px]"
                  />
                </div>

                <Input
                  v-if="form.customMode === 'weekly' || form.customMode === 'monthly'"
                  v-model="customTimeValue"
                  type="time"
                  class="h-10 w-[156px]"
                />
              </div>

              <div v-else class="space-y-2">
                <Label
                  for="scheduled-task-cron"
                  class="flex items-center gap-1 text-sm font-medium text-[#0a0a0a] dark:text-foreground"
                >
                  <span class="text-destructive" aria-hidden="true">*</span>
                  {{ t('scheduledTasks.dialog.cronLabel') }}
                </Label>
                <textarea
                  id="scheduled-task-cron"
                  v-model="form.cronExpr"
                  class="min-h-[108px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 font-mono text-sm leading-6 text-foreground shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 dark:bg-input/30"
                  :placeholder="t('scheduledTasks.dialog.cronPlaceholder')"
                />
              </div>
            </div>
          </section>

          <section
            class="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-4 dark:border-white/10"
          >
            <div class="space-y-1">
              <h3 class="text-sm font-semibold text-[#0a0a0a] dark:text-foreground">
                {{ t('scheduledTasks.dialog.enableNowTitle') }}
              </h3>
              <p class="text-sm text-muted-foreground">
                {{ t('scheduledTasks.dialog.enableNowHint') }}
              </p>
            </div>
            <Switch
              :model-value="form.enabled"
              @update:model-value="(value) => (form.enabled = !!value)"
            />
          </section>
        </div>
      </div>

      <DialogFooter class="shrink-0 gap-2 border-t border-border px-6 py-4 sm:justify-end">
        <Button type="button" variant="outline" @click="closeDialog">
          {{ t('common.cancel') }}
        </Button>
        <Button type="button" :disabled="submitActionDisabled" @click="handleSubmit">
          {{ saving || submitLocked ? t('scheduledTasks.dialog.submitting') : t('common.confirm') }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
