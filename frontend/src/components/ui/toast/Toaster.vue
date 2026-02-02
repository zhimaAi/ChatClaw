<script setup lang="ts">
import { CircleCheck, CircleX } from 'lucide-vue-next'
import ToastProvider from './ToastProvider.vue'
import ToastViewport from './ToastViewport.vue'
import Toast from './Toast.vue'
import ToastTitle from './ToastTitle.vue'
import ToastClose from './ToastClose.vue'
import { useToast, TOAST_DURATION } from './useToast'

const { toasts, dismiss } = useToast()
</script>

<template>
  <ToastProvider :duration="TOAST_DURATION" swipe-direction="right">
    <Toast
      v-for="t in toasts"
      :key="t.id"
      :variant="t.variant"
      :duration="t.duration"
      @update:open="(open) => !open && dismiss(t.id)"
    >
      <div class="flex items-center gap-3">
        <CircleCheck v-if="t.variant === 'success'" class="size-5 shrink-0 text-green-600 dark:text-green-400" />
        <CircleX v-else-if="t.variant === 'error'" class="size-5 shrink-0 text-red-600 dark:text-red-400" />
        <div class="grid gap-1">
          <ToastTitle v-if="t.title">{{ t.title }}</ToastTitle>
        </div>
      </div>
      <ToastClose />
    </Toast>
    <ToastViewport />
  </ToastProvider>
</template>
