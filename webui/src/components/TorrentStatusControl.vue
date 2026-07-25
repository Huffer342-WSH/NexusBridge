<!-- 种子状态控件将下载入口、qB 进度和暂停恢复操作合并为圆形按钮。 -->
<script setup lang="ts">
import { ArrowDown, ArrowUp, Check, LoaderCircle, Play, Square } from '@lucide/vue';
import { computed } from 'vue';
import { NIcon } from 'naive-ui';
import type { Torrent } from '../types';
import { formatByteSize } from '../utils/format';

const props = defineProps<{
  torrent: Torrent;
  loading: boolean;
}>();

const emit = defineEmits<{
  download: [torrent: Torrent];
  control: [torrent: Torrent, action: 'start' | 'stop'];
}>();

const status = computed(() => props.torrent.qb_status);
const added = computed(() => Boolean(status.value?.added));
const complete = computed(() => {
  const state = (status.value?.state ?? '').toLowerCase();
  const completedStates = ['uploading', 'stalledup', 'queuedup', 'pausedup', 'stoppedup', 'forcedup', 'checkingup'];
  return added.value && ((status.value?.progress ?? 0) >= 1 || completedStates.includes(state));
});
const paused = computed(() => {
  const state = (status.value?.state ?? '').toLowerCase();
  return state.includes('stopped') || state.includes('paused');
});
const progress = computed(() => Math.max(0, Math.min(100, Math.round((status.value?.progress ?? 0) * 1000) / 10)));

const totalSize = computed(() => status.value?.size || props.torrent.size_bytes || 0);
const completedSize = computed(() => {
  if (status.value?.completed !== undefined) return status.value.completed;
  if (status.value?.amount_left !== undefined && totalSize.value > 0)
    return Math.max(0, totalSize.value - status.value.amount_left);
  return Math.round(totalSize.value * (status.value?.progress ?? 0));
});

const label = computed(() => {
  if (!added.value) return '下载';
  if (complete.value) return paused.value ? '已完成' : '做种中';
  if (paused.value) return '继续';
  return `${progress.value}%`;
});

const actionLabel = computed(() => {
  if (!added.value) return `下载 ${props.torrent.title}`;
  return paused.value ? `恢复 ${props.torrent.title}` : `暂停 ${props.torrent.title}`;
});

const sizeLabel = computed(() => {
  if (added.value && !complete.value)
    return `${formatByteSize(completedSize.value)} / ${formatByteSize(totalSize.value)}`;
  return formatByteSize(totalSize.value);
});

/** 根据当前 qB 状态触发下载、暂停或恢复。 */
function activate() {
  if (props.loading) return;
  if (!added.value) {
    emit('download', props.torrent);
    return;
  }
  emit('control', props.torrent, paused.value ? 'start' : 'stop');
}
</script>

<template>
  <div class="torrent-status-control-wrap">
    <button
      type="button"
      class="torrent-status-control"
      :class="{ 'is-complete': complete, 'is-paused': paused, 'is-idle': !added }"
      :disabled="loading || (!added && !torrent.download_url)"
      :aria-label="actionLabel"
      @click="activate"
    >
      <span class="torrent-status-center">
        <NIcon v-if="loading" :component="LoaderCircle" class="spin" />
        <NIcon v-else-if="!added" :component="ArrowDown" />
        <NIcon v-else-if="paused && complete" :component="Check" />
        <NIcon v-else-if="paused" :component="Play" />
        <NIcon v-else-if="!complete" :component="Square" />
        <NIcon v-else :component="ArrowUp" />
      </span>
    </button>
    <span class="torrent-status-copy">
      <strong>{{ label }}</strong>
      <small>{{ sizeLabel }}</small>
    </span>
    <span v-if="added && !complete" class="torrent-status-progress" aria-hidden="true">
      <i :style="{ width: `${progress}%` }" />
    </span>
  </div>
</template>
