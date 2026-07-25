<!-- Artplayer 视频适配器只包装浏览器原生视频源，不接入转码插件。 -->
<script setup lang="ts">
import Artplayer, { type Setting, type SettingOption } from 'artplayer';
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
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

const container = ref<HTMLDivElement | null>(null);
let player: Artplayer | undefined;

function destroyPlayer() {
  player?.destroy(false);
  player = undefined;
}

function textElement(value: string) {
  const element = document.createElement('span');
  element.textContent = value;
  return element;
}

function escapeHTML(value: string) {
  return value.replace(/[&<>"']/g, (character) => {
    const entities: Record<string, string> = {
      '&': '&amp;',
      '<': '&lt;',
      '>': '&gt;',
      '"': '&quot;',
      "'": '&#39;',
    };
    return entities[character] ?? character;
  });
}

async function mountPlayer() {
  await nextTick();
  if (!container.value) return;
  destroyPlayer();
  const extension = props.name.split('.').pop()?.toLowerCase() ?? '';
  const selectedSubtitle =
    props.subtitles.find((item) => item.forced) ?? props.subtitles.find((item) => item.default) ?? props.subtitles[0];
  const settings: Setting[] =
    props.subtitles.length > 1
      ? [
          {
            html: '字幕',
            width: 260,
            tooltip: selectedSubtitle?.label ?? '关闭',
            selector: props.subtitles.map((subtitle) => ({
              html: textElement(subtitle.label),
              default: subtitle.track_id === selectedSubtitle?.track_id,
              subtitle,
            })),
            onSelect(this: Artplayer, item: SettingOption) {
              const subtitle = item.subtitle as PlaybackSubtitle;
              void this.subtitle.switch(subtitle.stream_url, {
                name: subtitle.label,
                type: 'vtt',
                encoding: 'utf-8',
                escape: true,
              });
              return escapeHTML(subtitle.label);
            },
          },
        ]
      : [];
  try {
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
      settings,
      playbackRate: true,
      subtitleOffset: Boolean(selectedSubtitle),
      fullscreen: true,
      fullscreenWeb: true,
      airplay: true,
      ...(selectedSubtitle
        ? {
            subtitle: {
              name: selectedSubtitle.label,
              url: selectedSubtitle.stream_url,
              type: 'vtt',
              encoding: 'utf-8',
              escape: true,
              style: {
                color: '#ffffff',
                fontSize: 'clamp(18px, 2.2vw, 32px)',
                textShadow: '0 1px 4px #000000, 0 1px 8px #000000',
              },
            },
          }
        : {}),
      moreVideoAttr: {
        playsInline: true,
        preload: 'metadata',
      },
    });
  } catch (reason) {
    const message = reason instanceof Error ? reason.message : String(reason);
    emit('error', `播放器初始化失败：${message}`);
    return;
  }
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
