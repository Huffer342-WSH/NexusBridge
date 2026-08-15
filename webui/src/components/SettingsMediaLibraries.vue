<script setup lang="ts">
import { Save } from '@lucide/vue';
import { onMounted, ref } from 'vue';
import { NAlert, NButton, NCard, NForm, NFormItem, NIcon, NInputNumber, NSpin } from 'naive-ui';
import { api } from '../api';

const intervalMinutes = ref<number | null>(360);
const loading = ref(true);
const saving = ref(false);
const error = ref('');
const saved = ref(false);

async function load() {
  loading.value = true;
  error.value = '';
  try {
    intervalMinutes.value = (await api.getSubtitleScanSettings()).interval_minutes;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库设置加载失败';
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (intervalMinutes.value === null) {
    error.value = '请输入扫描周期';
    return;
  }
  saving.value = true;
  saved.value = false;
  error.value = '';
  try {
    const result = await api.saveSubtitleScanSettings({ interval_minutes: intervalMinutes.value });
    intervalMinutes.value = result.interval_minutes;
    saved.value = true;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库设置保存失败';
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>媒体库设置</h2>
        <p class="muted">配置所有媒体库共用的后台扫描行为。</p>
      </div>
    </NCard>

    <NCard :bordered="false">
      <NSpin :show="loading">
        <NForm class="settings-form" @submit.prevent="save">
          <NFormItem label="MKV 字幕扫描周期（分钟）">
            <NInputNumber v-model:value="intervalMinutes" :min="0" :max="1440" :precision="0" />
          </NFormItem>
          <p class="muted">
            所有媒体库共用，默认 6 小时。0 表示关闭周期扫描；启动时扫描、人工重扫和播放时按需生成仍然保留。保存后立即生效，无需重启。
          </p>
          <NAlert v-if="error" type="error" :bordered="false">{{ error }}</NAlert>
          <NAlert v-else-if="saved" type="success" :bordered="false">设置已保存并立即生效。</NAlert>
          <NButton type="primary" attr-type="submit" :loading="saving" :disabled="loading">
            <template #icon><NIcon :component="Save" /></template>
            保存媒体库设置
          </NButton>
        </NForm>
      </NSpin>
    </NCard>
  </section>
</template>
