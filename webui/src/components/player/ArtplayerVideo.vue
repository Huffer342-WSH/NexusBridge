<!-- Artplayer 视频适配器只包装浏览器原生视频源，不接入转码插件。 -->
<script setup lang="ts">
import Artplayer from 'artplayer';
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue';

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

const container = ref<HTMLDivElement | null>(null);
let player: Artplayer | undefined;

function destroyPlayer() {
  player?.destroy(false);
  player = undefined;
}

async function mountPlayer() {
  await nextTick();
  if (!container.value) return;
  destroyPlayer();
  const extension = props.name.split('.').pop()?.toLowerCase() ?? '';
  player = new Artplayer({
    container: container.value,
    url: props.source,
    type: extension,
    poster: props.poster,
    autoplay: true,
    lang: 'zh-cn',
    theme: '#2563eb',
    hotkey: true,
    mutex: true,
    pip: true,
    setting: true,
    playbackRate: true,
    fullscreen: true,
    fullscreenWeb: true,
    airplay: true,
    moreVideoAttr: {
      playsInline: true,
      preload: 'metadata',
    },
  });
  player.on('video:error', () => emit('error', '浏览器无法加载或解码这个视频源文件'));
  player.on('ready', () => {
    void Promise.resolve(player?.play()).catch(() => emit('autoplayBlocked'));
  });
}

onMounted(() => void mountPlayer());
onBeforeUnmount(destroyPlayer);
</script>

<template>
  <div ref="container" class="artplayer-host" />
</template>

<style scoped>
.artplayer-host {
  width: 100%;
  height: 100%;
}
</style>
