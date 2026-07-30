<!-- 媒体缩略图固定占位尺寸，并在加载失败时静默回退为对应媒体图标。 -->
<script setup lang="ts">
import { Film, Image } from '@lucide/vue';
import { NIcon } from 'naive-ui';
import { computed, ref, watch } from 'vue';

const props = withDefaults(
  defineProps<{
    src?: string;
    alt?: string;
    mediaType?: 'video' | 'image';
    size?: 'compact' | 'default';
  }>(),
  {
    src: '',
    alt: '',
    mediaType: 'video',
    size: 'default',
  },
);

const failed = ref(false);
const loaded = ref(false);
const fallbackIcon = computed(() => (props.mediaType === 'image' ? Image : Film));

watch(
  () => props.src,
  () => {
    failed.value = false;
    loaded.value = false;
  },
);
</script>

<template>
  <span
    class="media-thumbnail"
    :class="[`media-thumbnail--${size}`, `media-thumbnail--${mediaType}`]"
    aria-hidden="true"
  >
    <NIcon :component="fallbackIcon" class="media-thumbnail-fallback" />
    <img
      v-if="src && !failed"
      :src="src"
      :alt="alt"
      :class="{ loaded }"
      loading="lazy"
      decoding="async"
      :fetchpriority="mediaType === 'image' ? 'high' : 'auto'"
      @load="loaded = true"
      @error="failed = true"
    />
  </span>
</template>

<style scoped>
.media-thumbnail {
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

.media-thumbnail--default {
  width: 72px;
  height: 42px;
}

.media-thumbnail--compact {
  width: 56px;
  height: 34px;
}

.media-thumbnail-fallback {
  width: 26px;
  height: 26px;
}

.media-thumbnail--compact .media-thumbnail-fallback {
  width: 22px;
  height: 22px;
}

.media-thumbnail img {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: contain;
  opacity: 0;
  transition: opacity 120ms ease;
}

.media-thumbnail img.loaded {
  opacity: 1;
}

.media-thumbnail--image img {
  transition: none;
}
</style>
