<script setup lang="ts">
import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useI18n } from 'vue-i18n'
import { Loader2 } from 'lucide-vue-next'
import {
  useNavigationStore,
  useAppStore,
  useOpenClawGatewayStore,
  gatewaySidebarTagShellClass,
  gatewaySidebarTagLabelClass,
  gatewaySidebarTagStatusClass,
  gatewaySidebarTagLoaderClass,
} from '@/stores'
import { GatewayVisualStatus } from '@/stores/openclaw-gateway'
import { cn } from '@/lib/utils'

const { t } = useI18n()
const navigationStore = useNavigationStore()
const appStore = useAppStore()
const gatewayStore = useOpenClawGatewayStore()

/** Same navigation as the "OpenClaw 管家" side nav item (openclaw-runtime). */
const goToOpenClawManager = () => {
  navigationStore.navigateToModule('openclaw-runtime', appStore.currentSystem)
}
const { visualStatus } = storeToRefs(gatewayStore)

const badgeText = computed(() =>
  t(`settings.openclawRuntime.statusBadge.${visualStatus.value}`)
)
const isStarting = computed(
  () =>
    visualStatus.value === GatewayVisualStatus.Starting ||
    visualStatus.value === GatewayVisualStatus.Upgrading
)

const labelSeparator = computed(() => t('settings.openclawRuntime.sidebarGatewayLabelSeparator'))

const v = computed(() => visualStatus.value)

const tagShellClass = computed(() => gatewaySidebarTagShellClass[v.value])
const tagLabelClass = computed(() => gatewaySidebarTagLabelClass[v.value])
const tagStatusClass = computed(() => gatewaySidebarTagStatusClass[v.value])
const tagLoaderClass = computed(() => gatewaySidebarTagLoaderClass[v.value])

const fullLineTitle = computed(() => {
  const prefix = t('settings.openclawRuntime.sidebarGatewayPrefix')
  return `${prefix}${labelSeparator.value}${badgeText.value}`
})

const dotClass = computed(() =>
  cn(
    'size-2.5 shrink-0 rounded-full',
    visualStatus.value === GatewayVisualStatus.Running && 'bg-emerald-500',
    visualStatus.value === GatewayVisualStatus.Error && 'bg-rose-500',
    visualStatus.value === GatewayVisualStatus.Stop && 'bg-neutral-400 dark:bg-neutral-500',
    (visualStatus.value === GatewayVisualStatus.Starting ||
      visualStatus.value === GatewayVisualStatus.Upgrading) &&
      'bg-amber-500'
  )
)
</script>

<template>
  <!-- Match ChatWikiSidebarAccountCard: w-full px-2, inner rounded-lg px-3 py-2 text-[13px] font-bold, left-aligned. -->
  <div
    v-if="!navigationStore.sidebarCollapsed"
    id="sidebar-gateway-status"
    class="w-full px-2"
  >
    <button
      type="button"
      :class="
        cn(
          'flex w-full min-w-0 flex-nowrap items-center justify-start gap-0 rounded-lg border px-3 py-2 text-left text-[13px] font-bold leading-tight',
          'cursor-pointer transition-opacity hover:opacity-90 active:opacity-80',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
          tagShellClass
        )
      "
      :title="fullLineTitle"
      @click="goToOpenClawManager"
    >
      <span :class="cn('shrink-0 whitespace-nowrap', tagLabelClass)">
        {{ t('settings.openclawRuntime.sidebarGatewayPrefix') }}
      </span>
      <span :class="cn('shrink-0 whitespace-nowrap', tagLabelClass)">
        {{ labelSeparator }}
      </span>
      <span class="inline-flex min-w-0 flex-1 items-center justify-start gap-1.5">
        <span :class="cn('min-w-0 truncate tabular-nums', tagStatusClass)">{{ badgeText }}</span>
        <Loader2
          v-if="isStarting"
          :class="cn('size-4 shrink-0 animate-spin', tagLoaderClass)"
        />
      </span>
    </button>
  </div>
  <button
    v-else
    type="button"
    class="mx-auto flex flex-col items-center py-1 cursor-pointer rounded-md transition-opacity hover:opacity-90 active:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
    :title="fullLineTitle"
    @click="goToOpenClawManager"
  >
    <Loader2
      v-if="isStarting"
      :class="cn('size-4 animate-spin', tagLoaderClass)"
    />
    <span v-else :class="dotClass" />
  </button>
</template>
