import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useAppStore, useOpenClawGatewayStore, GatewayVisualStatus } from '@/stores'

/**
 * When the app is in OpenClaw system mode and the gateway is not running,
 * composer UIs should block input and show gateway status messaging.
 */
export function useOpenClawGatewayComposerGate() {
  const appStore = useAppStore()
  const gatewayStore = useOpenClawGatewayStore()
  const { visualStatus } = storeToRefs(gatewayStore)

  const blocksComposer = computed(
    () => appStore.currentSystem === 'openclaw' && visualStatus.value !== GatewayVisualStatus.Running
  )

  return { blocksComposer, visualStatus, GatewayVisualStatus }
}
