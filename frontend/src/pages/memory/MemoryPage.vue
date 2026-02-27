<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import IconMemory from '@/assets/icons/memory.svg'
import { cn } from '@/lib/utils'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useNavigationStore } from '@/stores'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { MemoryService } from '@bindings/chatclaw/internal/services/memory'
import type { ThematicFactDTO, EventStreamDTO } from '@bindings/chatclaw/internal/services/memory'

defineProps<{
  tabId: string
}>()

type MemoryTab = 'basicInfo' | 'topicSummary' | 'conversationLog'

const { t } = useI18n()
const navigationStore = useNavigationStore()

const agents = ref<Agent[]>([])
const loading = ref(false)
const selectedAgentId = ref<number | null>(null)
const activeMemoryTab = ref<MemoryTab>('basicInfo')

const coreProfile = ref('')
const thematicFacts = ref<ThematicFactDTO[]>([])
const eventStreams = ref<EventStreamDTO[]>([])
const memoryLoading = ref(false)

const memoryTabs: { key: MemoryTab; labelKey: string }[] = [
  { key: 'basicInfo', labelKey: 'memory.basicInfo' },
  { key: 'topicSummary', labelKey: 'memory.topicSummary' },
  { key: 'conversationLog', labelKey: 'memory.conversationLog' },
]

const selectedAgent = computed(
  () => agents.value.find((a) => a.id === selectedAgentId.value) || null,
)

const eventsByDate = computed(() => {
  const groups: { date: string; items: EventStreamDTO[] }[] = []
  const map = new Map<string, EventStreamDTO[]>()
  for (const e of eventStreams.value) {
    const d = e.date || 'unknown'
    if (!map.has(d)) {
      map.set(d, [])
      groups.push({ date: d, items: map.get(d)! })
    }
    map.get(d)!.push(e)
  }
  return groups
})

const loadAgents = async () => {
  loading.value = true
  let selectedAgentChanged = false
  try {
    const list = await AgentsService.ListAgents()
    agents.value = list || []
    if (selectedAgentId.value == null && agents.value.length > 0) {
      selectedAgentId.value = agents.value[0].id
      selectedAgentChanged = true
    }
  } catch (error) {
    toast.error(getErrorMessage(error) || 'Failed to load agents')
  } finally {
    loading.value = false
  }
  return selectedAgentChanged
}

const loadMemory = async (agentId: number) => {
  memoryLoading.value = true
  try {
    const [profile, facts, events] = await Promise.all([
      MemoryService.GetCoreProfile(agentId),
      MemoryService.GetThematicFacts(agentId),
      MemoryService.GetEventStreams(agentId),
    ])
    coreProfile.value = profile || ''
    thematicFacts.value = facts || []
    eventStreams.value = events || []
  } catch (error) {
    toast.error(getErrorMessage(error) || 'Failed to load memory')
  } finally {
    memoryLoading.value = false
  }
}

watch(selectedAgentId, (id) => {
  if (id != null) {
    void loadMemory(id)
  } else {
    coreProfile.value = ''
    thematicFacts.value = []
    eventStreams.value = []
  }
})

watch(
  () => navigationStore.activeModule,
  (module) => {
    if (module === 'memory') {
      void loadAgents().then((selectedAgentChanged) => {
        if (!selectedAgentChanged && selectedAgentId.value != null) {
          void loadMemory(selectedAgentId.value)
        }
      })
    }
  },
)

onMounted(() => {
  void loadAgents()
})
</script>

<template>
  <div class="flex h-full w-full bg-background text-foreground">
    <!-- Left: Agent list -->
    <aside class="flex w-sidebar shrink-0 flex-col border-r border-border">
      <div class="flex items-center gap-2 px-3 py-3">
        <span class="text-sm font-medium text-foreground">{{ t('memory.title') }}</span>
      </div>

      <div class="flex-1 overflow-auto px-2 pb-2">
        <div v-if="loading" class="px-2 py-6 text-sm text-muted-foreground">
          {{ t('common.loading') }}
        </div>

        <div
          v-else-if="agents.length === 0"
          class="mx-2 mt-2 flex items-center justify-center rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground"
        >
          {{ t('memory.noData') }}
        </div>

        <div v-else class="flex flex-col gap-1">
          <button
            v-for="agent in agents"
            :key="agent.id"
            type="button"
            :class="
              cn(
                'flex h-10 w-full items-center gap-2 rounded-lg px-2 text-left text-sm font-normal transition-colors',
                selectedAgentId === agent.id
                  ? 'bg-accent text-accent-foreground'
                  : 'text-foreground hover:bg-accent/50',
              )
            "
            @click="selectedAgentId = agent.id"
          >
            <img
              v-if="agent.icon"
              :src="agent.icon"
              class="size-5 shrink-0 rounded"
              alt=""
            />
            <div
              v-else
              class="grid size-5 shrink-0 place-items-center rounded bg-muted"
            >
              <IconMemory class="size-3 text-muted-foreground" />
            </div>
            <span class="min-w-0 flex-1 truncate">{{ agent.name }}</span>
          </button>
        </div>
      </div>
    </aside>

    <!-- Right: Memory content -->
    <main class="flex flex-1 flex-col overflow-hidden bg-background">
      <!-- No agent selected -->
      <div v-if="!selectedAgent" class="flex h-full items-center justify-center px-8">
        <div
          class="rounded-2xl border border-border bg-card p-8 text-muted-foreground shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5"
        >
          {{ t('memory.selectAgent') }}
        </div>
      </div>

      <template v-else>
        <!-- Tab bar -->
        <div class="flex items-center border-b border-border px-4 py-2">
          <div class="inline-flex rounded-md bg-muted p-1">
            <button
              v-for="tab in memoryTabs"
              :key="tab.key"
              type="button"
              :class="
                cn(
                  'rounded px-3 py-1 text-sm transition-colors',
                  activeMemoryTab === tab.key
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground',
                )
              "
              @click="activeMemoryTab = tab.key"
            >
              {{ t(tab.labelKey) }}
            </button>
          </div>
        </div>

        <!-- Loading -->
        <div v-if="memoryLoading" class="flex flex-1 items-center justify-center">
          <span class="text-sm text-muted-foreground">{{ t('common.loading') }}</span>
        </div>

        <!-- Tab content -->
        <div v-else class="flex-1 overflow-auto">
          <div class="mx-auto max-w-3xl p-6">
            <!-- Basic Info -->
            <div v-if="activeMemoryTab === 'basicInfo'">
              <p class="mb-4 text-xs text-muted-foreground">{{ t('memory.basicInfoDesc') }}</p>
              <div
                class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
              >
                <p v-if="coreProfile" class="whitespace-pre-wrap text-sm text-foreground">
                  {{ coreProfile }}
                </p>
                <p v-else class="text-sm text-muted-foreground">
                  {{ t('memory.basicInfoEmpty') }}
                </p>
              </div>
            </div>

            <!-- Topic Summary -->
            <div v-else-if="activeMemoryTab === 'topicSummary'">
              <p class="mb-4 text-xs text-muted-foreground">{{ t('memory.topicSummaryDesc') }}</p>
              <div
                v-if="thematicFacts.length === 0"
                class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
              >
                <p class="text-sm text-muted-foreground">
                  {{ t('memory.topicSummaryEmpty') }}
                </p>
              </div>
              <div v-else class="space-y-2">
                <div
                  v-for="fact in thematicFacts"
                  :key="fact.id"
                  class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
                >
                  <div class="mb-1 text-xs font-medium text-muted-foreground">{{ fact.topic }}</div>
                  <p class="whitespace-pre-wrap text-sm text-foreground">{{ fact.content }}</p>
                </div>
              </div>
            </div>

            <!-- Conversation Log -->
            <div v-else-if="activeMemoryTab === 'conversationLog'">
              <p class="mb-4 text-xs text-muted-foreground">{{ t('memory.conversationLogDesc') }}</p>
              <div
                v-if="eventStreams.length === 0"
                class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
              >
                <p class="text-sm text-muted-foreground">
                  {{ t('memory.conversationLogEmpty') }}
                </p>
              </div>
              <div v-else class="space-y-4">
                <div v-for="group in eventsByDate" :key="group.date">
                  <div class="mb-2 text-xs font-medium text-muted-foreground">{{ group.date }}</div>
                  <div class="space-y-2">
                    <div
                      v-for="event in group.items"
                      :key="event.id"
                      class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
                    >
                      <p class="whitespace-pre-wrap text-sm text-foreground">{{ event.content }}</p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
    </main>
  </div>
</template>
