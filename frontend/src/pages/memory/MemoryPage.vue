<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Pencil, Trash2 } from 'lucide-vue-next'
import { useThemeLogo } from '@/composables/useLogo'
import { cn } from '@/lib/utils'
import { toast } from '@/components/ui/toast'
import { getErrorMessage } from '@/composables/useErrorMessage'
import { useNavigationStore } from '@/stores'
import { AgentsService } from '@bindings/chatclaw/internal/services/agents'
import type { Agent } from '@bindings/chatclaw/internal/services/agents'
import { MemoryService } from '@bindings/chatclaw/internal/services/memory'
import type { ThematicFactDTO, EventStreamDTO } from '@bindings/chatclaw/internal/services/memory'
import { EventStreamPageInput } from '@bindings/chatclaw/internal/services/memory'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

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

const EVENT_PAGE_SIZE = 50
const eventBeforeDate = ref('')
const eventBeforeID = ref<number>(0)
const eventHasMore = ref(true)
const eventLoadingMore = ref(false)
let eventLoadToken = 0

const scrollContainerRef = ref<HTMLElement | null>(null)
const loadMoreSentinelRef = ref<HTMLElement | null>(null)
let loadMoreObserver: IntersectionObserver | null = null

// Edit state
const editingCoreProfile = ref(false)
const editCoreProfileContent = ref('')

const editFactDialogOpen = ref(false)
const editFactId = ref<number | null>(null)
const editFactTopic = ref('')
const editFactContent = ref('')

const editEventDialogOpen = ref(false)
const editEventId = ref<number | null>(null)
const editEventContent = ref('')

// Delete confirmation state
const deleteFactDialogOpen = ref(false)
const deleteFactId = ref<number | null>(null)

const deleteEventDialogOpen = ref(false)
const deleteEventId = ref<number | null>(null)

const saving = ref(false)

const memoryTabs: { key: MemoryTab; labelKey: string }[] = [
  { key: 'basicInfo', labelKey: 'memory.basicInfo' },
  { key: 'topicSummary', labelKey: 'memory.topicSummary' },
  { key: 'conversationLog', labelKey: 'memory.conversationLog' },
]

const selectedAgent = computed(
  () => agents.value.find((a) => a.id === selectedAgentId.value) || null
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

const { logoSrc } = useThemeLogo()

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
  eventStreams.value = []
  eventBeforeDate.value = ''
  eventBeforeID.value = 0
  eventHasMore.value = true
  eventLoadToken += 1
  try {
    const [profile, facts] = await Promise.all([
      MemoryService.GetCoreProfile(agentId),
      MemoryService.GetThematicFacts(agentId),
    ])
    coreProfile.value = profile || ''
    thematicFacts.value = facts || []
    await loadMoreEvents(eventLoadToken)
  } catch (error) {
    toast.error(getErrorMessage(error) || 'Failed to load memory')
  } finally {
    memoryLoading.value = false
  }
}

const loadMoreEvents = async (token?: number) => {
  if (selectedAgentId.value == null) return
  if (!eventHasMore.value) return
  if (eventLoadingMore.value) return

  const currentToken = token ?? eventLoadToken
  const isFirst = eventStreams.value.length === 0
  if (!isFirst) {
    eventLoadingMore.value = true
  }

  try {
    const result = await MemoryService.GetEventStreamsPage(
      new EventStreamPageInput({
        agent_id: selectedAgentId.value,
        before_date: eventBeforeDate.value,
        before_id: eventBeforeID.value,
        limit: EVENT_PAGE_SIZE,
      })
    )
    if (currentToken !== eventLoadToken) return

    const incoming = result || []
    const existingIDs = new Set(eventStreams.value.map((e) => e.id))
    const merged: EventStreamDTO[] = []
    for (const ev of incoming) {
      if (!existingIDs.has(ev.id)) merged.push(ev)
    }

    if (merged.length > 0) {
      eventStreams.value.push(...merged)
      const last = merged[merged.length - 1]
      eventBeforeDate.value = last.date
      eventBeforeID.value = last.id
    }

    eventHasMore.value = incoming.length >= EVENT_PAGE_SIZE
    if (incoming.length > 0 && merged.length === 0) {
      eventHasMore.value = false
    }
  } catch (error) {
    console.error('Failed to load event streams:', error)
  } finally {
    eventLoadingMore.value = false
  }
}

// Core Profile edit
const startEditCoreProfile = () => {
  editCoreProfileContent.value = coreProfile.value
  editingCoreProfile.value = true
}

const cancelEditCoreProfile = () => {
  editingCoreProfile.value = false
}

const saveCoreProfile = async () => {
  if (selectedAgentId.value == null) return
  saving.value = true
  try {
    await MemoryService.UpdateCoreProfile(selectedAgentId.value, editCoreProfileContent.value)
    coreProfile.value = editCoreProfileContent.value
    editingCoreProfile.value = false
    toast.success(t('memory.updateSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('memory.updateFailed'))
  } finally {
    saving.value = false
  }
}

// Thematic Fact edit / delete
const startEditFact = (fact: ThematicFactDTO) => {
  editFactId.value = fact.id
  editFactTopic.value = fact.topic
  editFactContent.value = fact.content
  editFactDialogOpen.value = true
}

const saveThematicFact = async () => {
  if (editFactId.value == null) return
  saving.value = true
  try {
    await MemoryService.UpdateThematicFact(
      editFactId.value,
      editFactTopic.value,
      editFactContent.value
    )
    const idx = thematicFacts.value.findIndex((f) => f.id === editFactId.value)
    if (idx !== -1) {
      thematicFacts.value[idx] = {
        ...thematicFacts.value[idx],
        topic: editFactTopic.value,
        content: editFactContent.value,
      }
    }
    editFactDialogOpen.value = false
    toast.success(t('memory.updateSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('memory.updateFailed'))
  } finally {
    saving.value = false
  }
}

const confirmDeleteFact = (id: number) => {
  deleteFactId.value = id
  deleteFactDialogOpen.value = true
}

const doDeleteFact = async () => {
  if (deleteFactId.value == null) return
  try {
    await MemoryService.DeleteThematicFact(deleteFactId.value)
    thematicFacts.value = thematicFacts.value.filter((f) => f.id !== deleteFactId.value)
    toast.success(t('memory.deleteSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('memory.deleteFailed'))
  } finally {
    deleteFactDialogOpen.value = false
  }
}

// Event Stream edit / delete
const startEditEvent = (event: EventStreamDTO) => {
  editEventId.value = event.id
  editEventContent.value = event.content
  editEventDialogOpen.value = true
}

const saveEventStream = async () => {
  if (editEventId.value == null) return
  saving.value = true
  try {
    await MemoryService.UpdateEventStream(editEventId.value, editEventContent.value)
    const flat = eventStreams.value
    const idx = flat.findIndex((e) => e.id === editEventId.value)
    if (idx !== -1) {
      flat[idx] = { ...flat[idx], content: editEventContent.value }
    }
    editEventDialogOpen.value = false
    toast.success(t('memory.updateSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('memory.updateFailed'))
  } finally {
    saving.value = false
  }
}

const confirmDeleteEvent = (id: number) => {
  deleteEventId.value = id
  deleteEventDialogOpen.value = true
}

const doDeleteEvent = async () => {
  if (deleteEventId.value == null) return
  try {
    await MemoryService.DeleteEventStream(deleteEventId.value)
    eventStreams.value = eventStreams.value.filter((e) => e.id !== deleteEventId.value)
    if (eventStreams.value.length > 0) {
      const last = eventStreams.value[eventStreams.value.length - 1]
      eventBeforeDate.value = last.date
      eventBeforeID.value = last.id
    } else {
      eventBeforeDate.value = ''
      eventBeforeID.value = 0
    }
    toast.success(t('memory.deleteSuccess'))
  } catch (error) {
    toast.error(getErrorMessage(error) || t('memory.deleteFailed'))
  } finally {
    deleteEventDialogOpen.value = false
  }
}

watch(selectedAgentId, (id) => {
  editingCoreProfile.value = false
  if (id != null) {
    void loadMemory(id)
  } else {
    coreProfile.value = ''
    thematicFacts.value = []
    eventStreams.value = []
    eventBeforeDate.value = ''
    eventBeforeID.value = 0
    eventHasMore.value = true
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
  }
)

const setupScrollObserver = async () => {
  await nextTick()
  if (loadMoreObserver) {
    loadMoreObserver.disconnect()
    loadMoreObserver = null
  }
  if (!scrollContainerRef.value || !loadMoreSentinelRef.value) return
  loadMoreObserver = new IntersectionObserver(
    (entries) => {
      const entry = entries[0]
      if (!entry?.isIntersecting) return
      if (activeMemoryTab.value !== 'conversationLog') return
      void loadMoreEvents()
    },
    {
      root: scrollContainerRef.value,
      rootMargin: '200px',
      threshold: 0,
    }
  )
  loadMoreObserver.observe(loadMoreSentinelRef.value)
}

watch([scrollContainerRef, loadMoreSentinelRef], setupScrollObserver)
watch(activeMemoryTab, (tab) => {
  if (tab === 'conversationLog') {
    void setupScrollObserver()
  }
})

onBeforeUnmount(() => {
  if (loadMoreObserver) {
    loadMoreObserver.disconnect()
    loadMoreObserver = null
  }
})

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
                  : 'text-foreground hover:bg-accent/50'
              )
            "
            @click="selectedAgentId = agent.id"
          >
            <img v-if="agent.icon" :src="agent.icon" class="size-5 shrink-0 rounded" alt="" />
            <div
              v-else
              class="flex size-5 shrink-0 items-center justify-center overflow-hidden rounded border border-border bg-white dark:border-white/15 dark:bg-white/5"
            >
              <img :src="logoSrc" class="size-4 opacity-90" alt="ChatClaw logo" />
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
                    : 'text-muted-foreground hover:text-foreground'
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
        <div v-else ref="scrollContainerRef" class="flex-1 overflow-auto">
          <div class="mx-auto max-w-3xl p-6">
            <!-- Basic Info -->
            <div v-if="activeMemoryTab === 'basicInfo'">
              <p class="mb-4 text-xs text-muted-foreground">{{ t('memory.basicInfoDesc') }}</p>
              <div
                class="rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
              >
                <template v-if="editingCoreProfile">
                  <textarea
                    v-model="editCoreProfileContent"
                    class="min-h-[120px] w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                  <div class="mt-3 flex justify-end gap-2">
                    <button
                      type="button"
                      class="rounded-md border border-border px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent"
                      @click="cancelEditCoreProfile"
                    >
                      {{ t('memory.cancel') }}
                    </button>
                    <button
                      type="button"
                      :disabled="saving"
                      class="rounded-md bg-foreground px-3 py-1.5 text-xs text-background transition-colors hover:bg-foreground/90 disabled:opacity-50"
                      @click="saveCoreProfile"
                    >
                      {{ t('memory.save') }}
                    </button>
                  </div>
                </template>
                <template v-else>
                  <div class="flex items-start justify-between gap-2">
                    <p
                      v-if="coreProfile"
                      class="flex-1 whitespace-pre-wrap text-sm text-foreground"
                    >
                      {{ coreProfile }}
                    </p>
                    <p v-else class="flex-1 text-sm text-muted-foreground">
                      {{ t('memory.basicInfoEmpty') }}
                    </p>
                    <button
                      type="button"
                      class="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                      :title="t('memory.edit')"
                      @click="startEditCoreProfile"
                    >
                      <Pencil class="size-3.5" />
                    </button>
                  </div>
                </template>
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
                  class="group rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
                >
                  <div class="flex items-start justify-between gap-2">
                    <div class="min-w-0 flex-1">
                      <div class="mb-1 text-xs font-medium text-muted-foreground">
                        {{ fact.topic }}
                      </div>
                      <p class="whitespace-pre-wrap text-sm text-foreground">{{ fact.content }}</p>
                    </div>
                    <div
                      class="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover:opacity-100"
                    >
                      <button
                        type="button"
                        class="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                        :title="t('memory.edit')"
                        @click="startEditFact(fact)"
                      >
                        <Pencil class="size-3.5" />
                      </button>
                      <button
                        type="button"
                        class="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-destructive"
                        :title="t('memory.delete')"
                        @click="confirmDeleteFact(fact.id)"
                      >
                        <Trash2 class="size-3.5" />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Conversation Log -->
            <div v-else-if="activeMemoryTab === 'conversationLog'">
              <p class="mb-4 text-xs text-muted-foreground">
                {{ t('memory.conversationLogDesc') }}
              </p>
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
                      class="group rounded-lg border border-border bg-card p-4 shadow-sm dark:shadow-none dark:ring-1 dark:ring-white/10"
                    >
                      <div class="flex items-start justify-between gap-2">
                        <p class="min-w-0 flex-1 whitespace-pre-wrap text-sm text-foreground">
                          {{ event.content }}
                        </p>
                        <div
                          class="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover:opacity-100"
                        >
                          <button
                            type="button"
                            class="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                            :title="t('memory.edit')"
                            @click="startEditEvent(event)"
                          >
                            <Pencil class="size-3.5" />
                          </button>
                          <button
                            type="button"
                            class="rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-destructive"
                            :title="t('memory.delete')"
                            @click="confirmDeleteEvent(event.id)"
                          >
                            <Trash2 class="size-3.5" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div class="flex items-center justify-center py-2">
                  <div v-if="eventLoadingMore" class="text-xs text-muted-foreground">
                    {{ t('memory.loadingMore') }}
                  </div>
                  <div v-else-if="!eventHasMore" class="text-xs text-muted-foreground/60">
                    {{ t('memory.noMore') }}
                  </div>
                </div>
                <div ref="loadMoreSentinelRef" class="h-1 w-full" />
              </div>
            </div>
          </div>
        </div>
      </template>
    </main>

    <!-- Edit Thematic Fact Dialog -->
    <Dialog v-model:open="editFactDialogOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{{ t('memory.editThematicFact') }}</DialogTitle>
        </DialogHeader>
        <div class="flex flex-col gap-3 py-2">
          <div class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-foreground">{{ t('memory.topic') }}</label>
            <input
              v-model="editFactTopic"
              class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
          <div class="flex flex-col gap-1.5">
            <label class="text-sm font-medium text-foreground">{{ t('memory.content') }}</label>
            <textarea
              v-model="editFactContent"
              class="min-h-[120px] w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>
        <DialogFooter>
          <button
            type="button"
            class="rounded-md border border-border px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent"
            @click="editFactDialogOpen = false"
          >
            {{ t('memory.cancel') }}
          </button>
          <button
            type="button"
            :disabled="saving || !editFactTopic.trim() || !editFactContent.trim()"
            class="rounded-md bg-foreground px-3 py-1.5 text-sm text-background transition-colors hover:bg-foreground/90 disabled:opacity-50"
            @click="saveThematicFact"
          >
            {{ t('memory.save') }}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Edit Event Stream Dialog -->
    <Dialog v-model:open="editEventDialogOpen">
      <DialogContent class="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{{ t('memory.editEventStream') }}</DialogTitle>
        </DialogHeader>
        <div class="py-2">
          <textarea
            v-model="editEventContent"
            class="min-h-[120px] w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <DialogFooter>
          <button
            type="button"
            class="rounded-md border border-border px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-accent"
            @click="editEventDialogOpen = false"
          >
            {{ t('memory.cancel') }}
          </button>
          <button
            type="button"
            :disabled="saving || !editEventContent.trim()"
            class="rounded-md bg-foreground px-3 py-1.5 text-sm text-background transition-colors hover:bg-foreground/90 disabled:opacity-50"
            @click="saveEventStream"
          >
            {{ t('memory.save') }}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- Delete Thematic Fact Confirmation -->
    <AlertDialog v-model:open="deleteFactDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('memory.deleteConfirmTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('memory.deleteThematicFactConfirm') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('memory.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            class="bg-foreground text-background hover:bg-foreground/90"
            @click.prevent="doDeleteFact"
          >
            {{ t('memory.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- Delete Event Stream Confirmation -->
    <AlertDialog v-model:open="deleteEventDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('memory.deleteConfirmTitle') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('memory.deleteEventStreamConfirm') }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('memory.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            class="bg-foreground text-background hover:bg-foreground/90"
            @click.prevent="doDeleteEvent"
          >
            {{ t('memory.delete') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
