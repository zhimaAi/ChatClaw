<script setup lang="ts">
import { ref, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Events } from '@wailsio/runtime'
import { useLocaleSync } from '@/composables/useLocale'
import { TextSelectionService } from '@bindings/chatclaw/internal/services/textselection'

const { t } = useI18n()

const unsubLocale = useLocaleSync()

const handleMouseDown = (e: MouseEvent) => {
  if (e.button !== 0) return
  e.preventDefault()
  try {
    Events.Emit('text-selection:button-click')
  } catch (err) {
    console.error('[SelectionPopup] Button click error:', err)
  }
}

let hideTimer: number | null = null

const handleMouseEnter = () => {
  if (hideTimer) {
    window.clearTimeout(hideTimer)
    hideTimer = null
  }
  // Reset stale context menu state from previous popup activation
  if (contextMenuVisible.value) {
    contextMenuVisible.value = false
  }
}

const handleMouseLeave = () => {
  // When context menu is open, don't auto-hide on mouse leave.
  // Transparent window areas on Windows cause mouse-through, triggering
  // spurious mouseLeave while the user is navigating to the context menu.
  if (contextMenuVisible.value) return

  hideTimer = window.setTimeout(() => {
    Events.Emit('text-selection:hide')
  }, 500)
}

const contextMenuVisible = ref(false)

const handleContextMenu = async (e: MouseEvent) => {
  e.preventDefault()
  try {
    await TextSelectionService.ExpandPopupForContextMenu()
  } catch (err) {
    console.error('[SelectionPopup] Expand window error:', err)
  }
  contextMenuVisible.value = true
}

const hideContextMenu = async () => {
  if (!contextMenuVisible.value) return
  contextMenuVisible.value = false
  try {
    await TextSelectionService.RestorePopupFromContextMenu()
  } catch (err) {
    console.error('[SelectionPopup] Restore window error:', err)
  }
}

const handleDisableSelectionSearch = () => {
  if (contextMenuVisible.value) {
    contextMenuVisible.value = false
    TextSelectionService.RestorePopupFromContextMenu().catch(() => {})
  }
  // Use event instead of direct binding calls — the popup window has
  // WS_EX_NOACTIVATE which can interfere with Wails IPC responses.
  Events.Emit('text-selection:disable-selection-search')
}

onUnmounted(() => {
  unsubLocale()
  if (hideTimer) {
    window.clearTimeout(hideTimer)
    hideTimer = null
  }
})
</script>

<template>
  <div
    class="flex h-screen w-screen select-none items-start justify-center"
    @mouseenter="handleMouseEnter"
    @mouseleave="handleMouseLeave"
    @click="hideContextMenu"
  >
    <div class="flex flex-col items-center pt-1">
      <div
        class="flex cursor-pointer items-center rounded-full border border-border bg-background px-4 py-2 shadow-lg transition-all hover:bg-accent hover:shadow-xl active:scale-95"
        @mousedown="handleMouseDown"
        @contextmenu="handleContextMenu"
      >
        <span class="text-sm font-medium text-foreground">{{ t('selection.aiChat') }}</span>
      </div>

      <!-- Right-click context menu (below the button) -->
      <div
        v-if="contextMenuVisible"
        class="mt-1 min-w-[120px] rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md"
      >
        <div
          class="flex cursor-pointer items-center rounded-sm px-3 py-1.5 text-sm transition-colors hover:bg-accent hover:text-accent-foreground active:bg-accent/80"
          @mousedown.left.prevent="handleDisableSelectionSearch"
        >
          {{ t('selection.disableSelectionSearch') }}
        </div>
      </div>
    </div>
  </div>
</template>

<style>
/* Make background transparent */
html,
body,
#app {
  background: transparent !important;
  margin: 0;
  padding: 0;
}
</style>
