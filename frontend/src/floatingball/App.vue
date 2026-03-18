<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events, Window } from '@wailsio/runtime'
import { FloatingBallService } from '@bindings/chatclaw/internal/services/floatingball'
import { UpdaterService } from '@bindings/chatclaw/internal/services/updater'
import { SettingsService } from '@bindings/chatclaw/internal/services/settings'
import logoFloatingball from '@/assets/images/logo-floatingball.png'

const BALL_SIZE = 64
const MENU_AREA_HEIGHT = 140
const { t } = useI18n()

const debugDrag = () => {
  try {
    return localStorage.getItem('debugFloatingBallDrag') === '1'
  } catch {
    return false
  }
}
const logDrag = (...args: any[]) => {
  if (!debugDrag()) return

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
  if (coverOffTimer != null) {
    window.clearTimeout(coverOffTimer)
    coverOffTimer = null
  }
  coverActive.value = true
  logRender('cover:on', { why })
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
const screenStartX = ref(0)
const screenStartY = ref(0)
const relStartX = ref<number | null>(null)
const relStartY = ref<number | null>(null)
const isDragging = ref(false)

let rafMoveId: number | null = null
let pendingMove: { x: number; y: number } | null = null

const scheduleDragMove = (x: number, y: number) => {
  pendingMove = { x, y }
  if (rafMoveId != null) return
  rafMoveId = requestAnimationFrame(() => {
    rafMoveId = null
    const p = pendingMove
    pendingMove = null
    if (!p) return
    void FloatingBallService.DragMoveTo(Math.round(p.x), Math.round(p.y))
  })
}

const cancelDragMove = () => {
  if (rafMoveId != null) {
    cancelAnimationFrame(rafMoveId)
    rafMoveId = null
  }
  pendingMove = null
}

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

const menuVisible = ref(false)

const showMenu = async () => {
  if (menuVisible.value || collapsed.value) return
  try {
    await Window.SetMaxSize(BALL_SIZE, BALL_SIZE + MENU_AREA_HEIGHT)
    await Window.SetSize(BALL_SIZE, BALL_SIZE + MENU_AREA_HEIGHT)
    menuVisible.value = true
  } catch (e) {
    console.error('Failed to show context menu:', e)
  }
}

const hideMenu = async () => {
  if (!menuVisible.value) return
  menuVisible.value = false
  try {
    await Window.SetSize(BALL_SIZE, BALL_SIZE)
    await Window.SetMaxSize(BALL_SIZE, BALL_SIZE)
  } catch (e) {
    console.error('Failed to hide context menu:', e)
  }
}

const onContextMenu = () => {
  if (menuVisible.value) {
    void hideMenu()
  } else {
    void showMenu()
  }
}

const onMenuSettings = async () => {
  await hideMenu()
  Events.Emit('floatingball:open-settings')
  void FloatingBallService.OpenMainFromUI()
}

const onMenuRestart = async () => {
  await hideMenu()
  void UpdaterService.RestartApp()
}

const onMenuHide = async () => {
  await hideMenu()
  try {
    await SettingsService.SetValue('show_floating_window', 'false')
  } catch (e) {
    console.error('Failed to save floating window setting:', e)
  }
  void FloatingBallService.CloseFromUI()
}

const onPointerDown = (e: PointerEvent) => {
  if (e.button !== 0) return
  if (menuVisible.value) {
    void hideMenu()
    return
  }
  logDrag('pointerdown', { id: e.pointerId, type: e.pointerType, x: e.clientX, y: e.clientY })
  activePointerId.value = e.pointerId
  capturedPointerId.value = null
  pointerStartX.value = e.clientX
  pointerStartY.value = e.clientY
  screenStartX.value = e.screenX
  screenStartY.value = e.screenY
  relStartX.value = null
  relStartY.value = null
  isDragging.value = true
  void FloatingBallService.SetDragging(true)
  void FloatingBallService.GetRelativePosition().then((pos) => {
    if (activePointerId.value !== e.pointerId) return
    relStartX.value = pos.x ?? null
    relStartY.value = pos.y ?? null
    logDrag('dragStartRel', { x: pos.x, y: pos.y })
  })
}

const onPointerMove = (e: PointerEvent) => {
  if (activePointerId.value == null || e.pointerId !== activePointerId.value) return

  if (capturedPointerId.value == null) {
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
    return
  }

  if (relStartX.value == null || relStartY.value == null) return
  const dx = e.screenX - screenStartX.value
  const dy = e.screenY - screenStartY.value
  scheduleDragMove(relStartX.value + dx, relStartY.value + dy)
}
const onPointerUp = (e: PointerEvent) => {
  logDrag('pointerup', { id: e.pointerId, type: e.pointerType, x: e.clientX, y: e.clientY })
  try {
    ;(e.currentTarget as HTMLElement | null)?.releasePointerCapture?.(e.pointerId)
  } catch {
    // ignore
  }
  cancelDragMove()
  activePointerId.value = null
  capturedPointerId.value = null
  isDragging.value = false
  relStartX.value = null
  relStartY.value = null
  void FloatingBallService.SetDragging(false)
}
const onPointerCancel = () => {
  logDrag('pointercancel', {})
  cancelDragMove()
  activePointerId.value = null
  capturedPointerId.value = null
  isDragging.value = false
  relStartX.value = null
  relStartY.value = null
  void FloatingBallService.SetDragging(false)
}

const onDblClick = () => {
  if (menuVisible.value) {
    void hideMenu()
    return
  }
  void FloatingBallService.OpenMainFromUI()
}

const onResize = () => {
  const prevW = innerWidth.value
  innerWidth.value = window.innerWidth
  logRender('resize', { w: window.innerWidth, h: window.innerHeight, dpr: window.devicePixelRatio })
  if (prevW !== innerWidth.value && innerWidth.value <= 40) {
    flashCover('resize')
    repaintKey.value++
    logRender('repaint:resize', { key: repaintKey.value, w: innerWidth.value })
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
  if (!active) {
    if (menuVisible.value) {
      void hideMenu()
    }
    activePointerId.value = null
    capturedPointerId.value = null
    cancelDragMove()
    if (isDragging.value) {
      isDragging.value = false
      relStartX.value = null
      relStartY.value = null
      void FloatingBallService.SetDragging(false)
    }
  }
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
  setActive(!document.hidden)
  window.addEventListener('resize', onResize)
  window.addEventListener('focus', onWindowFocus)
  window.addEventListener('blur', onWindowBlur)
  document.addEventListener('visibilitychange', onVisibilityChange)

  offMacActive = Events.On(Events.Types.Mac.ApplicationDidBecomeActive, () => setActive(true))
  offMacInactive = Events.On(Events.Types.Mac.ApplicationDidResignActive, () => setActive(false))

  nativeHoverHandler = (entered: boolean) => {
    if (entered) onEnter()
    else onLeave()
  }
  ;(window as any).__floatingballNativeHover = nativeHoverHandler

  logRender('mounted', {
    w: window.innerWidth,
    h: window.innerHeight,
    dpr: window.devicePixelRatio,
  })
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
      if (menuVisible.value) {
        void hideMenu()
      }
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
  }
)
</script>

<template>
  <div
    class="h-full w-full bg-transparent select-none overflow-hidden flex flex-col"
    @contextmenu.prevent
  >
    <!-- Ball area: fixed height -->
    <div :key="repaintKey" class="w-full shrink-0" :style="{ height: BALL_SIZE + 'px' }">
      <div
        class="relative w-full h-full"
        @pointerenter="onEnter"
        @pointerleave="onLeave"
        @pointerover="onEnter"
        @pointerdown.capture="onPointerDown"
        @pointermove.capture="onPointerMove"
        @pointerup.capture="onPointerUp"
        @pointercancel.capture="onPointerCancel"
        @dblclick.stop="onDblClick"
        @contextmenu.prevent="onContextMenu"
      >
        <div
          class="absolute inset-0 rounded-full pointer-events-none z-30"
          style="background: rgba(0, 0, 0, 0.0001)"
          aria-hidden="true"
        />
        <div
          v-show="coverActive"
          class="absolute inset-0 rounded-full pointer-events-none z-60"
          :style="{ background: collapsed ? 'rgba(0,0,0,0.2)' : 'rgba(0,0,0,0.05)' }"
          aria-hidden="true"
        />

        <div
          :class="[
            'h-full w-full flex items-center justify-center relative z-10',
            collapsed
              ? isWindowsUA
                ? 'rounded-full bg-black/45 border border-white/20'
                : 'rounded-full bg-black/25'
              : 'rounded-full bg-transparent',
          ]"
          style="--wails-draggable: no-drag"
        >
          <img
            :key="collapsed ? 'collapsed' : 'expanded'"
            :src="logoFloatingball"
            :class="collapsed ? 'h-7 w-7' : 'h-11 w-11'"
            class="block"
            alt="ChatClaw floating icon"
            draggable="false"
            style="transform: translateZ(0); backface-visibility: hidden"
            @dragstart.prevent
          />
        </div>
      </div>
    </div>

    <!-- Context menu -->
    <div v-if="menuVisible" class="w-full flex-1 pt-1" style="--wails-draggable: no-drag">
      <div
        class="mx-0.5 rounded-lg border border-border bg-popover text-popover-foreground shadow-sm overflow-hidden"
      >
        <button
          class="w-full py-2 text-xs text-center transition-colors hover:bg-accent hover:text-accent-foreground"
          @click.stop="onMenuSettings"
        >
          {{ t('floatingball.menu.settings') }}
        </button>
        <button
          class="w-full py-2 text-xs text-center transition-colors hover:bg-accent hover:text-accent-foreground"
          @click.stop="onMenuRestart"
        >
          {{ t('floatingball.menu.restart') }}
        </button>
        <div class="border-t border-border" />
        <button
          class="w-full py-2 text-xs text-center transition-colors hover:bg-accent hover:text-accent-foreground"
          @click.stop="onMenuHide"
        >
          {{ t('floatingball.menu.hide') }}
        </button>
      </div>
    </div>
  </div>
</template>
