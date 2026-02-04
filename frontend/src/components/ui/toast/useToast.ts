import { ref, computed } from 'vue'

export const TOAST_DURATION_DEFAULT = 2000
export const TOAST_DURATION_ERROR = 4000
const TOAST_LIMIT = 5
// 备份计时器的额外延迟（给 reka-ui 足够的时间正常处理）
const FALLBACK_EXTRA_DELAY = 500

export interface ToastProps {
  id: string
  title?: string
  description?: string
  variant?: 'default' | 'success' | 'error'
  duration?: number
}

const toasts = ref<ToastProps[]>([])
// 存储每个 toast 的备份计时器，确保 toast 一定会消失
const toastTimers = new Map<string, ReturnType<typeof setTimeout>>()

let count = 0

function genId() {
  count = (count + 1) % Number.MAX_VALUE
  return count.toString()
}

function addToast(props: Omit<ToastProps, 'id'>) {
  const id = genId()
  // 根据 variant 设置不同的默认 duration
  const defaultDuration = props.variant === 'error' ? TOAST_DURATION_ERROR : TOAST_DURATION_DEFAULT
  const duration = props.duration ?? defaultDuration
  const newToast: ToastProps = {
    id,
    variant: 'default',
    duration,
    ...props,
  }

  const newList = [newToast, ...toasts.value]
  // 清理被截断的 toast 的计时器，避免内存泄漏
  const removed = newList.slice(TOAST_LIMIT)
  for (const t of removed) {
    const timer = toastTimers.get(t.id)
    if (timer) {
      clearTimeout(timer)
      toastTimers.delete(t.id)
    }
  }
  toasts.value = newList.slice(0, TOAST_LIMIT)

  // 设置备份计时器，确保 toast 一定会消失
  // 时间比 duration 稍长，给 reka-ui 足够的时间正常处理
  // duration <= 0: 视为“不自动关闭”，不设置备份计时器
  if (duration > 0) {
    const timer = setTimeout(() => {
      dismissToast(id)
    }, duration + FALLBACK_EXTRA_DELAY)
    toastTimers.set(id, timer)
  }

  return id
}

function dismissToast(id: string) {
  // 清除备份计时器
  const timer = toastTimers.get(id)
  if (timer) {
    clearTimeout(timer)
    toastTimers.delete(id)
  }

  toasts.value = toasts.value.filter((t) => t.id !== id)
}

export function useToast() {
  return {
    toasts: computed(() => toasts.value),
    toast: addToast,
    dismiss: dismissToast,
  }
}

// 便捷方法
export const toast = {
  success: (message: string) =>
    addToast({ title: message, variant: 'success', duration: TOAST_DURATION_DEFAULT }),
  error: (message: string) =>
    addToast({ title: message, variant: 'error', duration: TOAST_DURATION_ERROR }),
  default: (message: string) =>
    addToast({ title: message, variant: 'default', duration: TOAST_DURATION_DEFAULT }),
}
