<script setup lang="ts">
import folderIcon from '@/assets/images/folder-icon.png'

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
    class="group relative flex h-[182px] w-[166px] cursor-pointer flex-col rounded-xl border border-[#f0f0f0] bg-white shadow-sm transition-[box-shadow,border-color,background-color] duration-200 hover:border-black/[0.08] hover:bg-[#f5f5f5] hover:shadow-[0_2px_12px_rgba(0,0,0,0.07)] dark:border-white/15 dark:bg-card dark:shadow-none dark:ring-1 dark:ring-white/5 dark:hover:border-white/25 dark:hover:bg-muted dark:hover:shadow-none dark:hover:ring-white/12"
    @click="handleCardClick"
  >
    <!-- Folder icon area: 6px radius, muted bg per design -->
    <div
      class="relative mx-[7px] mt-[7px] flex h-[86px] w-[150px] items-center justify-center overflow-hidden rounded-md dark:border-border dark:bg-muted"
    >
      <img
        :src="folderIcon"
        alt=""
        class="h-auto w-[120px] max-w-full object-contain select-none"
        draggable="false"
      />
    </div>

    <!-- Title: 14px / 22px line-height per design -->
    <p
      class="mx-2 mt-2 line-clamp-2 h-[44px] text-center text-sm font-medium leading-[22px] text-foreground"
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
