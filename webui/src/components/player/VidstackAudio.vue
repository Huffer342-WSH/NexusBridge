<!-- Vidstack 音频适配器使用浏览器原生 audio provider。 -->
<script setup lang="ts">
import { computed } from 'vue';
import 'vidstack/define/media-player.js';
import 'vidstack/define/media-outlet.js';
import 'vidstack/define/media-community-skin.js';
import 'vidstack/styles/base.css';
import 'vidstack/styles/defaults.css';
import 'vidstack/styles/community-skin/audio.css';

const props = defineProps<{
  source: string;
  mimeType: string;
  name: string;
  poster?: string;
  title: string;
}>();

const emit = defineEmits<{
  error: [message: string];
  autoplayBlocked: [];
}>();

const playerSource = computed(() => ({
  src: props.source,
  type: props.mimeType,
}));
</script>

<template>
  <div class="vidstack-audio" :style="poster ? { backgroundImage: `url(${poster})` } : undefined">
    <div class="vidstack-audio-shade" />
    <media-player
      class="vidstack-player"
      view-type="audio"
      stream-type="on-demand"
      load="eager"
      autoplay
      :src="playerSource"
      :title="title"
      @error="emit('error', '浏览器无法加载或解码这个音频源文件')"
      @play-fail="emit('autoplayBlocked')"
    >
      <media-outlet />
      <media-community-skin />
    </media-player>
  </div>
</template>

<style scoped>
.vidstack-audio {
  position: relative;
  display: grid;
  width: 100%;
  height: 100%;
  place-items: end center;
  padding: clamp(24px, 6vw, 64px);
  overflow: hidden;
  background-color: #0f172a;
  background-position: center;
  background-size: cover;
}

.vidstack-audio-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.34), rgba(15, 23, 42, 0.92));
  backdrop-filter: blur(18px);
}

.vidstack-player {
  position: relative;
  z-index: 1;
  width: min(680px, 100%);
  --media-brand: #2563eb;
  --media-focus-ring-color: #60a5fa;
}
</style>
