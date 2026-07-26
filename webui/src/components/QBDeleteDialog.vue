<!-- qB 删除确认框要求用户明确选择是否同时删除已下载文件。 -->
<script setup lang="ts">
import { Trash2 } from '@lucide/vue';
import { ref, watch } from 'vue';
import { NAlert, NButton, NIcon, NModal, NRadio, NRadioGroup, NSpace } from 'naive-ui';

const props = defineProps<{
  show: boolean;
  title: string;
  loading: boolean;
  error?: string;
}>();

const emit = defineEmits<{
  'update:show': [show: boolean];
  confirm: [deleteFiles: boolean];
}>();

const deleteFiles = ref(false);

watch(
  () => props.show,
  (show) => {
    if (show) deleteFiles.value = false;
  },
);

function close() {
  if (!props.loading) emit('update:show', false);
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    title="删除 qB 任务"
    class="qb-delete-modal"
    :mask-closable="!loading"
    :closable="!loading"
    @update:show="(value) => !value && close()"
  >
    <div class="qb-delete-content">
      <p>
        即将从 qBittorrent 删除任务：<strong>{{ title }}</strong>
      </p>
      <NRadioGroup v-model:value="deleteFiles" name="qb-delete-files">
        <NSpace vertical>
          <NRadio :value="false">保留已下载文件，仅删除 qB 任务</NRadio>
          <NRadio :value="true">同时删除 qB 任务和已下载文件</NRadio>
        </NSpace>
      </NRadioGroup>
      <NAlert v-if="deleteFiles" type="error" :bordered="false">
        已下载文件会由 qBittorrent 删除，此操作无法在 NexusBridge 中撤销。
      </NAlert>
      <NAlert v-if="error" type="warning" :bordered="false">{{ error }}</NAlert>
    </div>

    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="loading" @click="close">取消</NButton>
        <NButton type="error" :loading="loading" @click="emit('confirm', deleteFiles)">
          <template #icon><NIcon :component="Trash2" /></template>
          {{ deleteFiles ? '删除任务和文件' : '删除任务' }}
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>
