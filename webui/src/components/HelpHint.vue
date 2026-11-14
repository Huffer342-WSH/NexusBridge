<!-- HelpHint 将辅助说明收纳到支持悬停、点击和键盘访问的提示按钮。 -->
<script setup lang="ts">
import { CircleAlert } from '@lucide/vue';
import { ref } from 'vue';
import { NButton, NIcon, NPopover } from 'naive-ui';
withDefaults(defineProps<{ label?: string }>(), { label: '查看说明' });
const visible = ref(false);
const pinned = ref(false);
function close() {
  visible.value = false;
  pinned.value = false;
}
</script>
<template>
  <NPopover
    trigger="manual"
    :show="visible"
    placement="top"
    :style="{ maxWidth: 'min(360px, calc(100vw - 32px))' }"
    @clickoutside="close"
  >
    <template #trigger>
      <NButton
        quaternary
        circle
        size="small"
        :aria-label="label"
        :aria-expanded="visible"
        @mouseenter="visible = true"
        @mouseleave="!pinned && (visible = false)"
        @blur="!pinned && (visible = false)"
        @click.stop="
          pinned = !pinned;
          visible = pinned;
        "
        @keydown.esc="close"
      >
        <template #icon><NIcon :component="CircleAlert" /></template>
      </NButton>
    </template>
    <div style="overflow-wrap: anywhere; line-height: 1.6"><slot /></div>
  </NPopover>
</template>
