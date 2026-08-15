<!-- Artplayer 视频适配器使用 JASSUB 渲染 ASS/SSA，并保留同轨 WebVTT 降级。 -->
<script setup lang="ts">
import Artplayer, { type Option, type Setting, type SettingOption } from 'artplayer';
import artplayerPluginJassub, { type JassubInstance } from 'artplayer-plugin-jassub';
import defaultFontUrl from 'jassub/dist/default.woff2?url';
import modernWasmUrl from 'jassub/dist/wasm/jassub-worker-modern.wasm?url';
import workerUrl from 'jassub/dist/wasm/jassub-worker.js?url';
import wasmUrl from 'jassub/dist/wasm/jassub-worker.wasm?url';
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
let jassub: JassubInstance | undefined;
let activeSubtitle: PlaybackSubtitle | undefined;
let richSubtitlesAvailable = false;

function destroyPlayer() {
  player?.destroy(false);
  player = undefined;
  jassub = undefined;
  activeSubtitle = undefined;
  richSubtitlesAvailable = false;
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

function supportsRichSubtitles() {
  return (
    typeof WebAssembly !== 'undefined' &&
    typeof Worker !== 'undefined' &&
    typeof OffscreenCanvas !== 'undefined' &&
    typeof HTMLCanvasElement !== 'undefined' &&
    'transferControlToOffscreen' in HTMLCanvasElement.prototype
  );
}

function showNativeSubtitle(art: Artplayer, subtitle: PlaybackSubtitle) {
  jassub?.freeTrack?.();
  art.subtitle.show = true;
  void art.subtitle.switch(subtitle.stream_url, {
    name: subtitle.label,
    type: 'vtt',
    encoding: 'utf-8',
    escape: true,
  });
}

function selectSubtitle(art: Artplayer, subtitle?: PlaybackSubtitle) {
  activeSubtitle = subtitle;
  if (!subtitle) {
    jassub?.freeTrack?.();
    art.subtitle.show = false;
    return '关闭';
  }
  if (richSubtitlesAvailable && subtitle.rich_url && jassub) {
    art.subtitle.show = false;
    jassub.setTrackByUrl?.(subtitle.rich_url);
  } else {
    showNativeSubtitle(art, subtitle);
  }
  return escapeHTML(subtitle.label);
}

async function mountPlayer() {
  await nextTick();
  if (!container.value) return;
  destroyPlayer();
  const extension = props.name.split('.').pop()?.toLowerCase() ?? '';
  const selectedSubtitle =
    props.subtitles.find((item) => item.forced) ?? props.subtitles.find((item) => item.default) ?? props.subtitles[0];
  const useRichSubtitle = Boolean(selectedSubtitle?.rich_url && supportsRichSubtitles());
  const settings: Setting[] = props.subtitles.length
    ? [
        {
          html: '字幕',
          width: 260,
          tooltip: selectedSubtitle?.label ?? '关闭',
          selector: [
            ...props.subtitles.map((subtitle) => ({
              html: textElement(subtitle.label),
              default: subtitle.track_id === selectedSubtitle?.track_id,
              subtitle,
            })),
            { html: '关闭', default: !selectedSubtitle, subtitle: undefined },
          ],
          onSelect(this: Artplayer, item: SettingOption) {
            return selectSubtitle(this, item.subtitle as PlaybackSubtitle | undefined);
          },
        },
      ]
    : [];
  const nativeSubtitle: Option['subtitle'] = selectedSubtitle
    ? {
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
      }
    : undefined;
  const option: Option = {
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
    plugins: useRichSubtitle
      ? [
          artplayerPluginJassub({
            subUrl: selectedSubtitle?.rich_url,
            workerUrl,
            wasmUrl,
            modernWasmUrl,
            availableFonts: { 'liberation sans': defaultFontUrl },
            fallbackFont: 'liberation sans',
            useLocalFonts: false,
            libassMemoryLimit: 64,
            libassGlyphLimit: 32,
          }),
        ]
      : [],
    playbackRate: true,
    subtitleOffset: Boolean(selectedSubtitle),
    fullscreen: true,
    fullscreenWeb: true,
    airplay: true,
    ...(selectedSubtitle && !useRichSubtitle ? { subtitle: nativeSubtitle } : {}),
    moreVideoAttr: {
      playsInline: true,
      preload: 'metadata',
    },
  };
  try {
    activeSubtitle = selectedSubtitle;
    player = new Artplayer(option);
    if (useRichSubtitle) {
      const plugin = player.plugins.artplayerPluginJassub as { instance?: JassubInstance } | undefined;
      jassub = plugin?.instance;
      richSubtitlesAvailable = Boolean(jassub);
      jassub?.addEventListener?.('error', () => {
        richSubtitlesAvailable = false;
        if (player && activeSubtitle) showNativeSubtitle(player, activeSubtitle);
      });
    }
  } catch (reason) {
    if (useRichSubtitle) {
      richSubtitlesAvailable = false;
      jassub = undefined;
      container.value.replaceChildren();
      try {
        player = new Artplayer({ ...option, plugins: [], subtitle: nativeSubtitle });
      } catch (fallbackReason) {
        const message = fallbackReason instanceof Error ? fallbackReason.message : String(fallbackReason);
        emit('error', `播放器初始化失败：${message}`);
        return;
      }
    } else {
      const message = reason instanceof Error ? reason.message : String(reason);
      emit('error', `播放器初始化失败：${message}`);
      return;
    }
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
