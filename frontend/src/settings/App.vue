<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { Button } from "@/components/ui/button";
import { WindowService } from "../../bindings/willchat/internal/services/windows";

const { t } = useI18n();

type WindowInfo = {
  name: string;
  title: string;
  url: string;
  created: boolean;
  visible: boolean;
};

const windows = ref<WindowInfo[]>([]);

const refresh = async () => {
  windows.value = await WindowService.List();
};

const hideSelf = async () => {
  await WindowService.Close("settings");
};

onMounted(() => {
  void refresh();
});
</script>

<template>
  <div class="min-h-screen bg-background p-4 text-foreground">
    <h2 class="mb-4 text-xl font-semibold">{{ t('settings.title') }}</h2>

    <div class="mb-4 flex flex-wrap gap-2">
      <Button variant="outline" @click="refresh">{{ t('settings.refreshWindows') }}</Button>
      <Button variant="outline" @click="hideSelf">{{ t('settings.hideSettings') }}</Button>
    </div>

    <div class="rounded-lg border bg-card p-4">
      <div class="mb-2 font-semibold">{{ t('settings.windowStatus') }}</div>
      <pre class="m-0 whitespace-pre-wrap text-sm text-muted-foreground">{{ windows }}</pre>
    </div>
  </div>
</template>

