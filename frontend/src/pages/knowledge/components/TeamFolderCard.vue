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
    class="group relative flex h-[182px] w-[166px] cursor-pointer flex-col rounded-xl border border-border bg-card transition-shadow hover:shadow-md dark:hover:shadow-none dark:hover:ring-1 dark:hover:ring-white/10"
    @click="handleCardClick"
  >
    <!-- 文件夹图标区域 -->
    <div
      class="relative mx-[7px] mt-[7px] flex h-[86px] w-[150px] items-center justify-center overflow-hidden rounded-md border border-border bg-muted"
    >
      <Folder class="size-12 text-muted-foreground/60" />
    </div>

    <!-- 标题 -->
    <p
      class="mx-[7px] mt-[8px] line-clamp-2 h-[44px] text-left text-sm leading-[22px] text-foreground"
      :title="group.name"
    >
      {{ group.name }}
    </p>

    <!-- 底部信息 -->
    <div class="mx-[7px] mt-auto flex items-center justify-between pb-[7px]">
      <div class="flex items-center gap-1 text-xs text-muted-foreground/70">
        <span>{{ group.total }}项</span>
      </div>
    </div>
  </div>
</template>
