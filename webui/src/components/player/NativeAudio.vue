<!-- 原生音频适配器用于构建配置切换和播放器降级。 -->
<script setup lang="ts">
defineProps<{
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

function attemptAutoplay(event: Event) {
  const audio = event.currentTarget as HTMLAudioElement;
  void audio.play().catch(() => emit('autoplayBlocked'));
}
</script>

<template>
  <div class="native-audio" :style="poster ? { backgroundImage: `url(${poster})` } : undefined">
    <div class="native-audio-shade" />
    <audio
      class="native-audio-control"
      controls
      autoplay
      preload="metadata"
      :aria-label="title"
      @canplay.once="attemptAutoplay"
      @error="emit('error', '浏览器无法加载或解码这个音频源文件')"
    >
      <source :src="source" :type="mimeType" />
    </audio>
  </div>
</template>

<style scoped>
.native-audio {
  position: relative;
  display: grid;
  width: 100%;
  height: 100%;
  place-items: end center;
  padding: clamp(24px, 6vw, 64px);
  background-color: #0f172a;
  background-position: center;
  background-size: cover;
}

.native-audio-shade {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, rgba(15, 23, 42, 0.32), rgba(15, 23, 42, 0.9));
  backdrop-filter: blur(18px);
}

.native-audio-control {
  position: relative;
  z-index: 1;
  width: min(680px, 100%);
}
</style>
