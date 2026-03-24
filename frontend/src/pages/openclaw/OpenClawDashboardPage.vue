<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2 } from 'lucide-vue-next'
import { useNavigationStore } from '@/stores'
import * as OpenClawRuntimeService from '@bindings/chatclaw/internal/openclaw/runtime/openclawruntimeservice'
import * as MultiaskService from '@bindings/chatclaw/internal/services/multiask/multiaskservice'
import { PanelBounds } from '@bindings/chatclaw/internal/services/multiask/models'

const PANEL_ID = 'openclaw-dashboard'

const props = defineProps<{
  tabId: string
}>()

const { t } = useI18n()
const navigationStore = useNavigationStore()

const containerRef = ref<HTMLElement>()
const loading = ref(true)
const error = ref('')
const panelCreated = ref(false)

const isTabActive = computed(() => navigationStore.activeTabId === props.tabId)

const getBounds = () => {
  if (!containerRef.value) return null
  const rect = containerRef.value.getBoundingClientRect()
  return {
    x: Math.round(rect.left),
    y: Math.round(rect.top),
    width: Math.round(rect.width),
    height: Math.round(rect.height),
  }
}

const updateBounds = async () => {
  if (!panelCreated.value) return
  const bounds = getBounds()
  if (!bounds || bounds.width <= 0 || bounds.height <= 0) return
  try {
    await MultiaskService.UpdatePanelBounds(PANEL_ID, new PanelBounds(bounds))
  } catch {
    // panel may have been destroyed externally
  }
}

const createPanel = async () => {
  try {
    const url = await OpenClawRuntimeService.GetDashboardURL()
    await MultiaskService.Initialize('ChatClaw')

    const bounds = getBounds()
    if (!bounds || bounds.width <= 0 || bounds.height <= 0) {
      error.value = 'Invalid panel bounds'
      return
    }

    try {
      await MultiaskService.DestroyPanel(PANEL_ID)
    } catch {
      // ignore if not exists
    }

    await MultiaskService.CreatePanel(
      PANEL_ID,
      PANEL_ID,
      'OpenClaw Dashboard',
      url,
      new PanelBounds(bounds),
    )
    panelCreated.value = true

    if (!isTabActive.value) {
      await MultiaskService.HidePanel(PANEL_ID)
    }
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

watch(isTabActive, async (active) => {
  if (!panelCreated.value) return
  try {
    if (active) {
      await MultiaskService.ShowPanel(PANEL_ID)
      await updateBounds()
    } else {
      await MultiaskService.HidePanel(PANEL_ID)
    }
  } catch {
    // panel may have been destroyed externally
  }
})

let resizeObserver: ResizeObserver | null = null

onMounted(async () => {
  await nextTick()
  setTimeout(async () => {
    await createPanel()

    if (containerRef.value) {
      resizeObserver = new ResizeObserver(() => {
        void updateBounds()
      })
      resizeObserver.observe(containerRef.value)
    }
    window.addEventListener('resize', updateBounds)
  }, 100)
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
  }
  window.removeEventListener('resize', updateBounds)

  if (panelCreated.value) {
    void MultiaskService.DestroyPanel(PANEL_ID).catch(() => {})
    panelCreated.value = false
  }
})
</script>

<template>
  <div class="relative flex h-full w-full flex-col overflow-hidden bg-background">
    <!-- WebView container: always rendered so getBoundingClientRect works before panel creation -->
    <div ref="containerRef" class="relative flex-1" />

    <!-- Loading / error overlays -->
    <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-background">
      <Loader2 class="size-6 animate-spin text-muted-foreground" />
    </div>

    <div v-else-if="error" class="absolute inset-0 z-10 flex items-center justify-center bg-background px-8">
      <p class="text-sm text-muted-foreground">
        {{ t('settings.openclawRuntime.dashboardError') }}
      </p>
    </div>
  </div>
</template>
