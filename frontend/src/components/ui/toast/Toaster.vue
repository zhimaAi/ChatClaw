<script setup lang="ts">
import { CircleCheck, CircleX } from 'lucide-vue-next'
import ToastProvider from './ToastProvider.vue'
import ToastViewport from './ToastViewport.vue'
import Toast from './Toast.vue'
import ToastTitle from './ToastTitle.vue'
import ToastClose from './ToastClose.vue'
import { useToast, TOAST_DURATION_DEFAULT } from './useToast'

const { toasts, dismiss } = useToast()
</script>

<template>
  <ToastProvider :duration="TOAST_DURATION_DEFAULT" swipe-direction="right">
    <Toast
      v-for="t in toasts"
      :key="t.id"
      :variant="t.variant"
      :duration="t.duration"
      @update:open="(open) => !open && dismiss(t.id)"
    >
      <div class="flex items-center gap-3">
        <!-- 黑白灰科技风：图标用灰阶，不用彩色 -->
        <CircleCheck v-if="t.variant === 'success'" class="size-5 shrink-0 text-muted-foreground" />
        <CircleX v-else-if="t.variant === 'error'" class="size-5 shrink-0 text-foreground/80" />
        <div class="grid gap-1">
          <ToastTitle v-if="t.title">{{ t.title }}</ToastTitle>
        </div>
      </div>
      <ToastClose />
    </Toast>
    <ToastViewport />
  </ToastProvider>
</template>
