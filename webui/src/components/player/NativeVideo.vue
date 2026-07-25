<!-- 原生视频适配器用于构建配置切换和播放器降级。 -->
<script setup lang="ts">
import type { PlaybackSubtitle } from '../../types';

const props = defineProps<{
  source: string;
  mimeType: string;
  name: string;
  poster?: string;
  title: string;
  subtitles: PlaybackSubtitle[];
}>();

const emit = defineEmits<{
  error: [message: string];
  autoplayBlocked: [];
}>();

function attemptAutoplay(event: Event) {
  const video = event.currentTarget as HTMLVideoElement;
  void video.play().catch(() => emit('autoplayBlocked'));
}
</script>

<template>
  <video
    class="native-video"
    controls
    autoplay
    playsinline
    preload="metadata"
    :poster="poster"
    :aria-label="title"
    @canplay.once="attemptAutoplay"
    @error="emit('error', '浏览器无法加载或解码这个视频源文件')"
  >
    <source :src="source" :type="mimeType" />
    <track
      v-for="(subtitle, index) in subtitles"
      :key="subtitle.track_id"
      kind="subtitles"
      :src="subtitle.stream_url"
      :label="subtitle.label"
      :srclang="subtitle.language || 'und'"
      :default="
        subtitle.forced || subtitle.default || (!subtitles.some((item) => item.forced || item.default) && index === 0)
      "
    />
  </video>
</template>

<style scoped>
.native-video {
  width: 100%;
  height: 100%;
  background: #020617;
  object-fit: contain;
}
</style>
