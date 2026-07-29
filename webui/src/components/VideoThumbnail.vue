<!-- 视频缩略图固定占位尺寸，并在按需生成失败时静默回退为原视频图标。 -->
<script setup lang="ts">
import { Film } from '@lucide/vue';
import { NIcon } from 'naive-ui';
import { ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    src?: string;
    alt?: string;
    size?: 'compact' | 'default';
  }>(),
  {
    src: '',
    alt: '',
    size: 'default',
  },
);

const failed = ref(false);
const loaded = ref(false);

watch(
  () => props.src,
  () => {
    failed.value = false;
    loaded.value = false;
  },
);
</script>

<template>
  <span class="video-thumbnail" :class="`video-thumbnail--${size}`" aria-hidden="true">
    <NIcon :component="Film" class="video-thumbnail-fallback" />
    <img
      v-if="src && !failed"
      :src="src"
      :alt="alt"
      :class="{ loaded }"
      loading="lazy"
      decoding="async"
      @load="loaded = true"
      @error="failed = true"
    />
  </span>
</template>

<style scoped>
.video-thumbnail {
  position: relative;
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 5px;
  background: #e2e8f0;
  color: #64748b;
}

.video-thumbnail--default {
  width: 72px;
  height: 42px;
}

.video-thumbnail--compact {
  width: 56px;
  height: 34px;
}

.video-thumbnail-fallback {
  width: 18px;
  height: 18px;
}

.video-thumbnail img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  opacity: 0;
  transition: opacity 120ms ease;
}

.video-thumbnail img.loaded {
  opacity: 1;
}
</style>
