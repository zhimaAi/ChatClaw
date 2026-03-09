import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { ChatWikiService, type Binding, type Robot } from '@bindings/chatclaw/internal/services/chatwiki'

/**
 * Team mode: loads ChatWiki binding (chatwiki_bindings table) and robot list
 * via /manage/chatclaw/getRobotList. Use when listMode is 'team'.
 */
/** exp is Unix timestamp in seconds; binding is valid only when exp > now */
function isBindingValid(b: Binding | null): boolean {
  if (!b) return false
  const exp = Number(b.exp)
  return exp > Math.floor(Date.now() / 1000)
}

export function useTeamRobots() {
  const { t } = useI18n()
  const teamRobots = ref<Robot[]>([])
  const activeTeamRobotId = ref<string | null>(null)
  const teamLoading = ref(false)
  const teamBindingChecked = ref(false)
  const hasBinding = ref(false)
  const binding = ref<Binding | null>(null)

  const teamBound = computed(() => isBindingValid(binding.value))

  const loadTeamRobots = async () => {
    console.info('[assistant][team] load robots: start')
    teamLoading.value = true
    teamBindingChecked.value = false
    hasBinding.value = false
    binding.value = null
    teamRobots.value = []
    activeTeamRobotId.value = null
    try {
      const latestBinding = await ChatWikiService.GetBinding()
      if (!latestBinding) {
        console.warn('[assistant][team] load robots: no chatwiki binding')
        return
      }
      binding.value = latestBinding
      hasBinding.value = true
      if (!isBindingValid(latestBinding)) {
        console.warn('[assistant][team] load robots: binding exp expired')
        return
      }
      console.info('[assistant][team] load robots: binding ok', {
        server_url: latestBinding.server_url,
        token_length: String(latestBinding.token ?? '').length,
      })
      const list = await ChatWikiService.GetRobotList()
      teamRobots.value = list ?? []
      activeTeamRobotId.value =
        teamRobots.value.length > 0 ? teamRobots.value[0].id : null
      console.info('[assistant][team] load robots: success', {
        count: teamRobots.value.length,
        active_robot_id: activeTeamRobotId.value,
      })
    } catch (error: unknown) {
      console.error('[assistant][team] load robots: failed', error)
      toast.error(getErrorMessage(error) || t('knowledge.team.needsBinding'))
      teamRobots.value = []
    } finally {
      teamLoading.value = false
      teamBindingChecked.value = true
      console.info('[assistant][team] load robots: finish')
    }
  }

  const isTeamEmpty = computed(
    () => !teamLoading.value && teamRobots.value.length === 0
  )
  const activeRobot = computed(
    () => teamRobots.value.find((robot) => robot.id === activeTeamRobotId.value) ?? null
  )

  return {
    teamRobots,
    activeTeamRobotId,
    activeRobot,
    teamLoading,
    teamBindingChecked,
    teamBound,
    hasBinding,
    binding,
    loadTeamRobots,
    isTeamEmpty,
  }
}
