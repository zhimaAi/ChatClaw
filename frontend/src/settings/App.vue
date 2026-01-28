<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
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
  <div style="padding: 16px">
    <h2>{{ t('settings.title') }}</h2>

    <div class="card" style="margin: 12px 0">
      <div style="display: flex; gap: 8px; flex-wrap: wrap">
        <button class="btn" @click="refresh">{{ t('settings.refreshWindows') }}</button>
        <button class="btn" @click="hideSelf">{{ t('settings.hideSettings') }}</button>
      </div>
    </div>

    <div class="card">
      <div style="font-weight: 600; margin-bottom: 8px">{{ t('settings.windowStatus') }}</div>
      <pre style="white-space: pre-wrap; margin: 0">{{ windows }}</pre>
    </div>
  </div>
</template>

