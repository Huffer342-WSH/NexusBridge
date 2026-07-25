<!-- 媒体画布按构建配置和媒体类型懒加载具体播放器。 -->
<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue';
import { mediaPlayerConfig } from '../../config/mediaPlayer';
import type { PlaybackMedia } from '../../types';

const props = defineProps<{
  media: PlaybackMedia;
  poster?: string;
  title: string;
}>();

const emit = defineEmits<{
  error: [message: string];
  autoplayBlocked: [];
}>();

const ArtplayerVideo = defineAsyncComponent(() => import('./ArtplayerVideo.vue'));
const NativeVideo = defineAsyncComponent(() => import('./NativeVideo.vue'));
const VidstackAudio = defineAsyncComponent(() => import('./VidstackAudio.vue'));
const NativeAudio = defineAsyncComponent(() => import('./NativeAudio.vue'));
const NativeImageViewer = defineAsyncComponent(() => import('./NativeImageViewer.vue'));

const renderer = computed(() => {
  if (props.media.media_type === 'video') {
    return mediaPlayerConfig.video === 'artplayer' ? ArtplayerVideo : NativeVideo;
  }
  if (props.media.media_type === 'audio') {
    return mediaPlayerConfig.audio === 'vidstack' ? VidstackAudio : NativeAudio;
  }
  return NativeImageViewer;
});
</script>

<template>
  <div class="media-canvas-renderer">
    <component
      :is="renderer"
      :key="`${media.media_type}:${media.index}:${media.stream_url}`"
      :source="media.stream_url"
      :mime-type="media.mime_type"
      :name="media.name"
      :poster="poster"
      :title="title"
      @error="emit('error', $event)"
      @autoplay-blocked="emit('autoplayBlocked')"
    />
  </div>
</template>

<style scoped>
.media-canvas-renderer {
  width: 100%;
  height: 100%;
}
</style>
