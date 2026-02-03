import { ref, computed } from 'vue'

export const TOAST_DURATION_DEFAULT = 2000
export const TOAST_DURATION_ERROR = 4000
const TOAST_LIMIT = 5

export interface ToastProps {
  id: string
  title?: string
  description?: string
  variant?: 'default' | 'success' | 'error'
  duration?: number
}

const toasts = ref<ToastProps[]>([])

let count = 0

function genId() {
  count = (count + 1) % Number.MAX_VALUE
  return count.toString()
}

function addToast(props: Omit<ToastProps, 'id'>) {
  const id = genId()
  // 根据 variant 设置不同的默认 duration
  const defaultDuration = props.variant === 'error' ? TOAST_DURATION_ERROR : TOAST_DURATION_DEFAULT
  const newToast: ToastProps = {
    id,
    variant: 'default',
    duration: defaultDuration,
    ...props,
  }

  toasts.value = [newToast, ...toasts.value].slice(0, TOAST_LIMIT)

  return id
}

function dismissToast(id: string) {
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
