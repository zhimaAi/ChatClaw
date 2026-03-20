<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Folder } from 'lucide-vue-next'

const { t } = useI18n()

interface TeamGroup {
  id: string
  name: string
  total: number
}

const props = defineProps<{
  group: TeamGroup
}>()

const emit = defineEmits<{
  (e: 'click', group: TeamGroup): void
}>()

const handleCardClick = () => {
  emit('click', props.group)
}
</script>

<template>
  <div
    class="group relative flex h-[182px] w-[166px] cursor-pointer flex-col border border-border bg-card shadow-sm transition-[box-shadow] hover:shadow-sm dark:border-white/15 dark:shadow-none dark:ring-1 dark:ring-white/5 dark:hover:ring-white/10"
    @click="handleCardClick"
  >
    <!-- Folder icon area: 6px radius, muted bg per design -->
    <div
      class="relative mx-2 mt-2 flex h-[86px] w-[150px] items-center justify-center overflow-hidden border border-border bg-[#f2f4f7] dark:bg-muted"
    >
      <Folder class="size-12 text-muted-foreground/60" />
    </div>

    <!-- Title -->
    <p
      class="mx-2 mt-2 line-clamp-2 h-[44px] text-left text-sm font-medium leading-[22px] text-foreground"
      :title="group.name"
    >
      {{ group.name }}
    </p>

    <!-- Footer -->
    <div class="mx-2 mt-auto flex items-center justify-between pb-2">
      <div class="flex items-center gap-1 text-xs text-muted-foreground/70">
        <span>{{ group.total }}项</span>
      </div>
    </div>
  </div>
</template>
