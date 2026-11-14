<!-- qB 删除按钮默认删除整个任务及文件，下拉菜单可选择保留文件。 -->
<script setup lang="ts">
import { ChevronDown, Trash2 } from '@lucide/vue';
import { NAlert, NButton, NButtonGroup, NDropdown, NIcon } from 'naive-ui';
defineProps<{ loading: boolean; error?: string }>();
const emit = defineEmits<{ confirm: [deleteFiles: boolean] }>();
</script>
<template>
  <div @click.stop @dblclick.stop>
    <NButtonGroup>
      <NButton
        type="error"
        secondary
        :loading="loading"
        :disabled="loading"
        title="删除整个 qB 任务及其全部下载文件，无法撤销"
        @click="emit('confirm', true)"
      >
        <template #icon><NIcon :component="Trash2" /></template>
        删除任务和文件
      </NButton>
      <NDropdown
        trigger="click"
        :disabled="loading"
        :options="[
          { label: '删除任务和文件', key: 'all' },
          { label: '仅删除任务，保留文件', key: 'keep' },
        ]"
        @select="(key) => !loading && emit('confirm', key === 'all')"
      >
        <NButton type="error" secondary :disabled="loading" aria-label="选择删除方式"
          ><template #icon><NIcon :component="ChevronDown" /></template
        ></NButton>
      </NDropdown>
    </NButtonGroup>
    <NAlert v-if="error" type="error" :bordered="false">{{ error }}</NAlert>
  </div>
</template>
