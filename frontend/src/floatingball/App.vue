<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { Events } from '@wailsio/runtime'
import { FloatingBallService } from '@bindings/willchat/internal/services/floatingball'
import logoUrl from '@/assets/images/logo.svg?url'

const debugDrag = () => {
  try {
    return localStorage.getItem('debugFloatingBallDrag') === '1'
  } catch {
    return false
  }
}
const logDrag = (...args: any[]) => {
  if (!debugDrag()) return
  // eslint-disable-next-line no-console
  console.log('[floatingball-drag]', new Date().toISOString(), ...args)
}

const debugHover = () => {
  try {
    return localStorage.getItem('debugFloatingBallHover') === '1'
  } catch {
    return false
  }
}
const logHover = (...args: any[]) => {
  if (!debugHover()) return
  // eslint-disable-next-line no-console
  console.log('[floatingball-hover]', new Date().toISOString(), ...args)
}

const debugRender = () => {
  try {
    return localStorage.getItem('debugFloatingBallRender') === '1'
  } catch {
    return false
  }
}
const logRender = (...args: any[]) => {
  if (!debugRender()) return
  // eslint-disable-next-line no-console
  console.log('[floatingball-render]', new Date().toISOString(), ...args)
}

const hovered = ref(false)
const innerWidth = ref(window.innerWidth)
const collapsed = computed(() => innerWidth.value <= 36)
const appActive = ref(true)
const repaintKey = ref(0)
const coverActive = ref(false)
let coverOffTimer: number | null = null

const isWindowsUA = (() => {
  try {
    return /Windows/i.test(navigator.userAgent)
  } catch {
    return false
  }
})()

const flashCover = (why: string) => {
  // Cover for 1-2 frames to clear WebKit ghosts in transparent + resized windows (macOS).
  if (coverOffTimer != null) {
    window.clearTimeout(coverOffTimer)
    coverOffTimer = null
  }
  coverActive.value = true
  logRender('cover:on', { why })
  // Two RAFs tends to be more reliable across machines.
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      coverOffTimer = window.setTimeout(() => {
        coverActive.value = false
        logRender('cover:off', { why })
        coverOffTimer = null
      }, 0)
    })
  })
}

const activePointerId = ref<number | null>(null)
const capturedPointerId = ref<number | null>(null)
const pointerStartX = ref(0)
const pointerStartY = ref(0)
const isDragging = ref(false)

const onEnter = () => {
  if (hovered.value) return
  logHover('enter', { hidden: document.hidden, focus: document.hasFocus?.() })
  hovered.value = true
  void FloatingBallService.Hover(true)
}

const onLeave = () => {
  if (!hovered.value) return
  logHover('leave', { hidden: document.hidden, focus: document.hasFocus?.() })
  hovered.value = false
  void FloatingBallService.Hover(false)
}

const onPointerDown = (e: PointerEvent) => {
  // 仅在“真实拖拽”时才 setPointerCapture（Windows 上过早 capture 可能导致鼠标被窗口“抓住”）
  logDrag('pointerdown', { id: e.pointerId, type: e.pointerType, x: e.clientX, y: e.clientY })
  activePointerId.value = e.pointerId
  capturedPointerId.value = null
  pointerStartX.value = e.clientX
  pointerStartY.value = e.clientY
  isDragging.value = true
  void FloatingBallService.SetDragging(true)
}

const onPointerMove = (e: PointerEvent) => {
  if (activePointerId.value == null || e.pointerId !== activePointerId.value) return
  if (capturedPointerId.value != null) return
  const dx = Math.abs(e.clientX - pointerStartX.value)
  const dy = Math.abs(e.clientY - pointerStartY.value)
  if (dx <= 2 && dy <= 2) return
  try {
    ;(e.currentTarget as HTMLElement | null)?.setPointerCapture?.(e.pointerId)
    const captured = (e.currentTarget as HTMLElement | null)?.hasPointerCapture?.(e.pointerId)
    logDrag('setPointerCapture', { id: e.pointerId, captured })
    if (captured) capturedPointerId.value = e.pointerId
  } catch {
    logDrag('setPointerCapture:failed', { id: e.pointerId })
  }
}
const onPointerUp = (e: PointerEvent) => {
  logDrag('pointerup', { id: e.pointerId, type: e.pointerType, x: e.clientX, y: e.clientY })
  try {
    ;(e.currentTarget as HTMLElement | null)?.releasePointerCapture?.(e.pointerId)
  } catch {
    // ignore
  }
  activePointerId.value = null
  capturedPointerId.value = null
  isDragging.value = false
  void FloatingBallService.SetDragging(false)
}
const onPointerCancel = () => {
  logDrag('pointercancel', {})
  activePointerId.value = null
  capturedPointerId.value = null
  isDragging.value = false
  void FloatingBallService.SetDragging(false)
}

const onDblClick = () => {
  void FloatingBallService.OpenMainFromUI()
}

const onClose = () => {
  void FloatingBallService.CloseFromUI()
}

const onResize = () => {
  const prevW = innerWidth.value
  innerWidth.value = window.innerWidth
  logRender('resize', { w: window.innerWidth, h: window.innerHeight, dpr: window.devicePixelRatio })
  // macOS transparent webview can leave "ghost" pixels after shrinking.
  // When entering/being in collapsed width, force a DOM remount to clear.
  if (prevW !== innerWidth.value && innerWidth.value <= 40) {
    flashCover('resize')
    repaintKey.value++
    logRender('repaint:resize', { key: repaintKey.value, w: innerWidth.value })
    // A delayed remount after the native resize settles helps on macOS.
    window.setTimeout(() => {
      if (window.innerWidth <= 40) {
        repaintKey.value++
        logRender('repaint:resize:delayed', { key: repaintKey.value, w: window.innerWidth })
      }
    }, 90)
  }
}

const setActive = (active: boolean) => {
  if (appActive.value === active) return
  appActive.value = active
  void FloatingBallService.SetAppActive(active)
  // 失焦时确保结束拖拽（避免 Windows 捕获导致无法点击其它应用）
  if (!active) {
    activePointerId.value = null
    capturedPointerId.value = null
    if (isDragging.value) {
      isDragging.value = false
      void FloatingBallService.SetDragging(false)
    }
  }
  // 如果应用重新激活且鼠标仍在悬浮球上，触发展开
  if (active && hovered.value) {
    void FloatingBallService.Hover(true)
  }
}

const onWindowFocus = () => setActive(true)
const onWindowBlur = () => setActive(false)
const onVisibilityChange = () => setActive(!document.hidden)

let offMacActive: (() => void) | null = null
let offMacInactive: (() => void) | null = null
let nativeHoverHandler: ((entered: boolean) => void) | null = null

onMounted(() => {
  // 初始状态：优先以 visibility 判断（切应用时更可靠）
  setActive(!document.hidden)
  window.addEventListener('resize', onResize)
  window.addEventListener('focus', onWindowFocus)
  window.addEventListener('blur', onWindowBlur)
  document.addEventListener('visibilitychange', onVisibilityChange)

  // macOS：监听应用激活/失活事件（更准确）
  offMacActive = Events.On(Events.Types.Mac.ApplicationDidBecomeActive, () => setActive(true))
  offMacInactive = Events.On(Events.Types.Mac.ApplicationDidResignActive, () => setActive(false))

  // macOS native hover bridge: native tracking area may call into this even when window not focused
  nativeHoverHandler = (entered: boolean) => {
    if (entered) onEnter()
    else onLeave()
  }
  ;(window as any).__floatingballNativeHover = nativeHoverHandler

  logRender('mounted', { w: window.innerWidth, h: window.innerHeight, dpr: window.devicePixelRatio })
})

onUnmounted(() => {
  window.removeEventListener('resize', onResize)
  window.removeEventListener('focus', onWindowFocus)
  window.removeEventListener('blur', onWindowBlur)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  offMacActive?.()
  offMacInactive?.()
  if ((window as any).__floatingballNativeHover === nativeHoverHandler) {
    delete (window as any).__floatingballNativeHover
  }
})

watch(
  () => collapsed.value,
  (val) => {
    logRender('collapsed', { val, w: window.innerWidth, h: window.innerHeight })
    if (val) {
      flashCover('collapsed')
      repaintKey.value++
      logRender('repaint:collapsed', { key: repaintKey.value })
      window.setTimeout(() => {
        if (collapsed.value) {
          repaintKey.value++
          logRender('repaint:collapsed:delayed', { key: repaintKey.value })
        }
      }, 120)
    }
  },
)
</script>

<template>
  <div class="h-full w-full bg-transparent select-none overflow-hidden">
    <div :key="repaintKey" class="h-full w-full">
      <div
      class="relative h-full w-full"
      @pointerenter="onEnter"
      @pointerleave="onLeave"
      @pointerover="onEnter"
      @pointerdown.capture="onPointerDown"
      @pointermove.capture="onPointerMove"
      @pointerup.capture="onPointerUp"
      @pointercancel.capture="onPointerCancel"
      @dblclick.stop="onDblClick"
    >
      <!-- Paint layer: tiny alpha to force proper redraw on transparent resize (macOS WebKit can leave ghosts) -->
      <div
        class="absolute inset-0 rounded-full pointer-events-none z-30"
        style="background: rgba(0, 0, 0, 0.0001)"
        aria-hidden="true"
      />
      <!-- Cover layer: briefly shown to force clear; only affects mac ghosting -->
      <div
        v-show="coverActive"
        class="absolute inset-0 rounded-full pointer-events-none z-60"
        :style="{ background: collapsed ? 'rgba(0,0,0,0.2)' : 'rgba(0,0,0,0.05)' }"
        aria-hidden="true"
      />

      <!-- Close button (show on hover) -->
      <button
        v-show="hovered && !collapsed && appActive"
        class="absolute right-2 top-2 z-40 h-5 w-5 rounded-full bg-black/70 text-white text-xs leading-5"
        style="--wails-draggable: no-drag"
        @click.stop="onClose"
        aria-label="close floating ball"
        title="Close"
      >
        ×
      </button>

      <!-- Ball (draggable) -->
      <div
        :class="[
          'h-full w-full flex items-center justify-center relative z-10',
          collapsed
            ? isWindowsUA
              ? 'rounded-full bg-black/45 border border-white/20'
              : 'rounded-full bg-black/25'
            : 'rounded-full bg-transparent',
        ]"
        style="--wails-draggable: drag"
      >
        <img
          :src="logoUrl"
          :key="collapsed ? 'collapsed' : 'expanded'"
          :class="collapsed ? 'h-7 w-7' : 'h-11 w-11'"
          class="block"
          style="transform: translateZ(0); backface-visibility: hidden"
          alt="WillChat"
          draggable="false"
        />
      </div>
      </div>
    </div>
  </div>
</template>

