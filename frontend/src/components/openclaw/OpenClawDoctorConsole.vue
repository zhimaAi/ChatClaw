<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { CheckCircle2, Terminal, AlertCircle, Clock } from 'lucide-vue-next'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()

// 执行状态
const isRunning = ref(false)
const command = ref('openclaw doctor')
const workingDir = ref('')
const exitCode = ref<number | null>(null)
const duration = ref<number>(0)
const wasFixed = ref(false)
const stdout = ref('')
const stderr = ref('')

// 判断执行是否成功
const isSuccess = computed(() => exitCode.value === 0)

// 格式化耗时显示
const formatDuration = (ms: number): string => {
  return `${ms}ms`
}

// 运行 openclaw doctor
const runDoctor = async (fix: boolean = false) => {
  if (isRunning.value) return

  isRunning.value = true
  stdout.value = ''
  stderr.value = ''
  exitCode.value = null
  duration.value = 0
  wasFixed.value = false

  const startTime = Date.now()
  const cmd = fix ? 'openclaw doctor --fix --yes --non-interactive' : 'openclaw doctor'

  command.value = cmd

  try {
    // 调用后端接口执行 doctor 命令
    // 注意：这里需要后端提供对应的接口
    const result = await OpenClawRuntimeService.RunDoctorCommand(cmd, fix)

    const endTime = Date.now()
    duration.value = endTime - startTime

    if (!result) {
      exitCode.value = 1
      stderr.value = t('settings.openclawRuntime.doctor.failed')
      toast.error(t('settings.openclawRuntime.doctor.failed'))
      return
    }

    stdout.value = result.stdout || ''
    stderr.value = result.stderr || ''
    exitCode.value = result.exitCode
    wasFixed.value = result.fixed || false

    if (result.exitCode === 0) {
      toast.success(t('settings.openclawRuntime.doctor.success'))
    } else {
      toast.error(t('settings.openclawRuntime.doctor.failed'))
    }
  } catch (e: any) {
    const endTime = Date.now()
    duration.value = endTime - startTime
    stderr.value = getErrorMessage(e) || e.message || 'Unknown error'
    exitCode.value = 1
    toast.error(getErrorMessage(e) || t('settings.openclawRuntime.doctor.failed'))
  } finally {
    isRunning.value = false
  }
}

// 复制输出内容
const copyOutput = async () => {
  const content = `Command: ${command.value}\nExit Code: ${exitCode.value}\nDuration: ${formatDuration(duration.value)}\nWorking Directory: ${workingDir.value}\n\n--- STDOUT ---\n${stdout.value}\n\n--- STDERR ---\n${stderr.value}`

  try {
    await navigator.clipboard.writeText(content)
    toast.success(t('winsnap.toast.copied'))
  } catch (e) {
    toast.error(t('assistant.chat.copyFailed'))
  }
}

// 获取工作目录
const loadWorkingDir = async () => {
  try {
    const status = await OpenClawRuntimeService.GetStatus()
    workingDir.value = status.runtimePath || '-'
  } catch (e) {
    workingDir.value = '-'
  }
}

onMounted(() => {
  void loadWorkingDir()
})
</script>

<template>
  <div class="bg-white relative size-full rounded-xl border border-border shadow-sm dark:border-white/10 dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/5">
    <!-- 标题栏 -->
    <div class="flex items-center justify-between border-b border-border bg-muted/30 p-4 dark:border-white/10 dark:bg-white/5">
      <div class="flex flex-col gap-1">
        <h2 class="text-base font-semibold text-foreground">
          {{ t('settings.openclawRuntime.doctor.title') }}
        </h2>
      </div>
      <div class="flex items-center gap-2">
        <!-- 运行按钮 -->
        <Button
          size="sm"
          variant="outline"
          :disabled="isRunning"
          @click="runDoctor(false)"
        >
          <Terminal class="mr-1.5 size-3.5" />
          {{ t('settings.openclawRuntime.doctor.run') }}
        </Button>
        <!-- 运行并修复按钮 -->
        <Button
          size="sm"
          variant="outline"
          :disabled="isRunning"
          @click="runDoctor(true)"
        >
          <CheckCircle2 class="mr-1.5 size-3.5" />
          {{ t('settings.openclawRuntime.doctor.runAndFix') }}
        </Button>
        <!-- 复制按钮 -->
        <Button
          size="sm"
          variant="outline"
          :disabled="isRunning || (!stdout && !stderr)"
          @click="copyOutput"
        >
          {{ t('common.copy') }}
        </Button>
      </div>
    </div>

    <!-- 输出区域 -->
    <div class="bg-white p-4 dark:bg-card">
      <!-- 顶部状态标签 -->
      <div class="mb-4 flex items-center gap-2">
        <div
          v-if="isRunning"
          class="bg-muted flex items-center gap-1 rounded-full px-3 py-1"
        >
          <Terminal class="size-3.5 animate-pulse text-primary" />
          <span class="text-xs text-muted-foreground">
            {{ t('settings.openclawRuntime.doctor.running') }}
          </span>
        </div>

        <div
          v-if="exitCode !== null"
          :class="[
            'flex items-center gap-1 rounded-full px-3 py-1',
            isSuccess
              ? 'bg-green-50 dark:bg-green-950/20'
              : 'bg-red-50 dark:bg-red-950/20'
          ]"
        >
          <CheckCircle2
            v-if="isSuccess"
            class="size-3.5 text-green-600 dark:text-green-400"
          />
          <AlertCircle
            v-else
            class="size-3.5 text-red-600 dark:text-red-400"
          />
          <span
            :class="[
              'text-xs',
              isSuccess
                ? 'text-green-700 dark:text-green-400'
                : 'text-red-700 dark:text-red-400'
            ]"
          >
            {{
              isSuccess
                ? t('settings.openclawRuntime.doctor.fixed')
                : t('settings.openclawRuntime.doctor.failed')
            }}
          </span>
        </div>

        <div
          v-if="exitCode !== null"
          class="bg-muted flex items-center gap-1 rounded-full px-3 py-1"
        >
          <span class="text-xs font-mono text-muted-foreground">
            {{ t('settings.openclawRuntime.doctor.exitCode') }}: {{ exitCode }}
          </span>
        </div>

        <div
          v-if="duration > 0"
          class="bg-muted flex items-center gap-1 rounded-full px-3 py-1"
        >
          <Clock class="size-3.5 text-muted-foreground" />
          <span class="text-xs text-muted-foreground">
            {{ t('settings.openclawRuntime.doctor.duration') }}: {{ formatDuration(duration) }}
          </span>
        </div>
      </div>

      <!-- 命令和工作目录信息 -->
      <div class="mb-4 space-y-2 text-sm text-muted-foreground">
        <div class="flex items-center gap-2">
          <span class="shrink-0 font-medium text-foreground">
            {{ t('settings.openclawRuntime.doctor.command') }}:
          </span>
          <code class="rounded bg-muted px-2 py-0.5 font-mono text-xs">
            {{ command }}
          </code>
        </div>
        <div class="flex items-center gap-2">
          <span class="shrink-0 font-medium text-foreground">
            {{ t('settings.openclawRuntime.doctor.workingDir') }}:
          </span>
          <code class="rounded bg-muted px-2 py-0.5 font-mono text-xs break-all">
            {{ workingDir }}
          </code>
        </div>
      </div>

      <!-- 输出区域 - 两列布局 -->
      <div class="grid grid-cols-2 gap-6">
        <!-- 标准输出 -->
        <div class="flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <Terminal class="size-4 text-muted-foreground" />
            <span class="text-sm font-medium text-foreground">
              {{ t('settings.openclawRuntime.doctor.stdout') }}
            </span>
          </div>
          <div class="min-h-48 max-h-[32rem] overflow-auto rounded-lg border border-border bg-muted/20 p-3 dark:bg-muted/10">
            <pre
              v-if="stdout"
              class="whitespace-pre-wrap break-words font-mono text-xs text-foreground"
            >{{ stdout }}</pre>
            <p
              v-else
              class="text-sm text-muted-foreground/60"
            >
              {{ t('settings.openclawRuntime.doctor.noOutput') }}
            </p>
          </div>
        </div>

        <!-- 标准错误 -->
        <div class="flex flex-col gap-2">
          <div class="flex items-center gap-2">
            <AlertCircle class="size-4 text-muted-foreground" />
            <span class="text-sm font-medium text-foreground">
              {{ t('settings.openclawRuntime.doctor.stderr') }}
            </span>
          </div>
          <div class="min-h-48 max-h-[32rem] overflow-auto rounded-lg border border-border bg-muted/20 p-3 dark:bg-muted/10">
            <pre
              v-if="stderr"
              class="whitespace-pre-wrap break-words font-mono text-xs text-destructive"
            >{{ stderr }}</pre>
            <p
              v-else
              class="text-sm text-muted-foreground/60"
            >
              {{ t('settings.openclawRuntime.doctor.noErrors') }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
