<!-- 播放视图负责实时选集、媒体画布、简介和其他可播放种子。 -->
<script setup lang="ts">
import { ExternalLink, Film, Image, Music2, Play, RefreshCw } from '@lucide/vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { NAlert, NButton, NCard, NEmpty, NIcon, NSpin, NTag } from 'naive-ui';
import { useRoute, useRouter } from 'vue-router';
import { api } from '../api';
import type { PlaybackMedia, PlaybackMediaType, PlaybackTorrent, TorrentPlayback } from '../types';
import { formatByteSize } from '../utils/format';
import MediaCanvas from './player/MediaCanvas.vue';

const route = useRoute();
const router = useRouter();
const loading = ref(true);
const error = ref('');
const otherError = ref('');
const playback = ref<TorrentPlayback | null>(null);
const otherTorrents = ref<PlaybackTorrent[]>([]);
const selectedIndex = ref<number | null>(null);
const playerError = ref('');
const autoplayBlocked = ref(false);
let generation = 0;
let pollTimer: number | undefined;

const siteID = computed(() => routeParam('site_id'));
const torrentID = computed(() => routeParam('torrent_id'));
const currentFile = computed(() => playback.value?.files.find((item) => item.index === selectedIndex.value) ?? null);
const coverURL = computed(() => {
  const torrent = playback.value?.torrent;
  if (!torrent?.cover_url) return '';
  return `/api/torrents/${encodeURIComponent(torrent.site_id)}/${encodeURIComponent(torrent.id)}/cover`;
});
const description = computed(() => {
  const torrent = playback.value?.torrent;
  return torrent?.detail_description?.trim() || torrent?.description?.trim() || torrent?.subtitle?.trim() || '暂无简介';
});
const hasActiveDownloads = computed(
  () => playback.value?.files.some((item) => item.selected && item.progress < 1) ?? false,
);

function routeParam(name: string) {
  const value = route.params[name];
  return Array.isArray(value) ? (value[0] ?? '') : String(value ?? '');
}

function mediaLabel(type: PlaybackMediaType) {
  if (type === 'video') return '视频';
  if (type === 'audio') return '音频';
  return '图片';
}

function mediaIcon(type: PlaybackMediaType) {
  if (type === 'video') return Film;
  if (type === 'audio') return Music2;
  return Image;
}

function baseName(name: string) {
  return name.split(/[\\/]/).pop() || name;
}

function formatProgress(progress: number) {
  return `${Math.max(0, Math.min(100, progress * 100)).toFixed(progress >= 1 ? 0 : 1)}%`;
}

function formatDate(value?: string) {
  if (!value) return '时间未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function otherCoverURL(torrent: PlaybackTorrent) {
  if (!torrent.cover_url) return '';
  return `/api/torrents/${encodeURIComponent(torrent.site_id)}/${encodeURIComponent(torrent.id)}/cover`;
}

function applyManifest(next: TorrentPlayback, keepSelection: boolean) {
  const previous = keepSelection ? selectedIndex.value : null;
  playback.value = next;
  const retained = previous == null ? undefined : next.files.find((item) => item.index === previous && item.available);
  selectedIndex.value = retained?.index ?? next.default_file_index ?? null;
  document.title = `${next.torrent.title} - NexusBridge`;
}

async function refreshManifest(token: number, keepSelection: boolean) {
  const next = await api.getTorrentPlayback(siteID.value, torrentID.value);
  if (token !== generation) return;
  applyManifest(next, keepSelection);
}

function schedulePolling(token: number) {
  if (pollTimer) window.clearTimeout(pollTimer);
  if (!hasActiveDownloads.value) return;
  pollTimer = window.setTimeout(async () => {
    try {
      await refreshManifest(token, true);
    } catch {
      // 页面已有清单可继续使用，轮询失败不覆盖当前播放状态。
    }
    if (token === generation) schedulePolling(token);
  }, 5000);
}

async function loadPlayback() {
  const token = ++generation;
  if (pollTimer) window.clearTimeout(pollTimer);
  loading.value = true;
  error.value = '';
  otherError.value = '';
  playerError.value = '';
  autoplayBlocked.value = false;
  playback.value = null;
  otherTorrents.value = [];
  selectedIndex.value = null;

  try {
    await refreshManifest(token, false);
    if (token !== generation) return;
    loading.value = false;
    schedulePolling(token);
  } catch (reason) {
    if (token !== generation) return;
    error.value = reason instanceof Error ? reason.message : '播放清单加载失败';
    loading.value = false;
    return;
  }

  try {
    const items = await api.getPlaybackTorrents(siteID.value, torrentID.value, 20);
    if (token === generation) otherTorrents.value = items;
  } catch (reason) {
    if (token === generation) {
      otherError.value = reason instanceof Error ? reason.message : '其他种子加载失败';
    }
  }
}

function selectFile(file: PlaybackMedia) {
  if (!file.available) return;
  selectedIndex.value = file.index;
  playerError.value = '';
  autoplayBlocked.value = false;
}

async function openOther(torrent: PlaybackTorrent) {
  await router.push({ name: 'playback', params: { site_id: torrent.site_id, torrent_id: torrent.id } });
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

watch(
  () => route.fullPath,
  () => void loadPlayback(),
  { immediate: true },
);
onBeforeUnmount(() => {
  generation++;
  if (pollTimer) window.clearTimeout(pollTimer);
});
</script>

<template>
  <section class="playback-view">
    <div v-if="loading" class="playback-loading">
      <NSpin size="large" />
      <span>正在读取 qB 媒体清单…</span>
    </div>

    <NAlert v-else-if="error" type="error" :bordered="false" class="playback-page-error">
      <div class="playback-error-content">
        <span>{{ error }}</span>
        <NButton secondary size="small" @click="loadPlayback">
          <template #icon><NIcon :component="RefreshCw" /></template>
          重试
        </NButton>
      </div>
    </NAlert>

    <template v-else-if="playback">
      <main class="playback-main">
        <div class="playback-primary">
          <div class="playback-stage">
            <MediaCanvas
              v-if="currentFile"
              :media="currentFile"
              :poster="coverURL"
              :title="playback.torrent.title"
              @error="playerError = $event"
              @autoplay-blocked="autoplayBlocked = true"
            />
            <NEmpty v-else description="当前没有已下载数据可供浏览器尝试播放" />
          </div>

          <NAlert v-if="currentFile && currentFile.progress < 1" type="warning" :bordered="false">
            该文件尚未下载完成。浏览器可以尝试读取已有数据，但跳转到缺失片段时可能停止或报错。
          </NAlert>
          <NAlert v-if="autoplayBlocked" type="info" :bordered="false">
            浏览器阻止了自动播放，请在播放器中点击播放。
          </NAlert>
          <NAlert v-if="playerError" type="error" :bordered="false">{{ playerError }}</NAlert>

          <NCard :bordered="false" class="playback-description">
            <div class="playback-title-row">
              <div>
                <h1>{{ playback.torrent.title }}</h1>
                <p class="muted">
                  {{ playback.torrent.site_id }}
                  <template v-if="playback.torrent.published_at">
                    · {{ formatDate(playback.torrent.published_at) }}
                  </template>
                </p>
              </div>
              <NButton
                v-if="playback.torrent.detail_url"
                tag="a"
                :href="playback.torrent.detail_url"
                target="_blank"
                rel="noopener noreferrer"
                secondary
              >
                <template #icon><NIcon :component="ExternalLink" /></template>
                查看来源
              </NButton>
            </div>
            <div class="playback-meta">
              <NTag v-if="playback.torrent.category" type="info">{{ playback.torrent.category }}</NTag>
              <NTag>{{ playback.qb_status.state || 'qB 已关联' }}</NTag>
              <NTag v-if="currentFile" :type="currentFile.complete ? 'success' : 'warning'">
                {{ mediaLabel(currentFile.media_type) }} · {{ formatByteSize(currentFile.size) }}
              </NTag>
            </div>
            <p v-if="currentFile" class="current-media-name">{{ currentFile.name }}</p>
            <p class="playback-summary">{{ description }}</p>
          </NCard>
        </div>

        <aside class="playback-sidebar">
          <NCard :bordered="false" class="playback-panel">
            <template #header>
              <div class="panel-heading">
                <span>选集</span>
                <NTag size="small">{{ playback.files.length }}</NTag>
              </div>
            </template>
            <div v-if="playback.files.length" class="episode-list">
              <button
                v-for="file in playback.files"
                :key="file.index"
                type="button"
                class="episode-item"
                :class="{ active: file.index === selectedIndex, disabled: !file.available }"
                :disabled="!file.available"
                @click="selectFile(file)"
              >
                <NIcon :component="mediaIcon(file.media_type)" size="18" />
                <span class="episode-copy">
                  <strong>{{ baseName(file.name) }}</strong>
                  <small>
                    {{ mediaLabel(file.media_type) }} · {{ formatByteSize(file.size) }} ·
                    {{ file.selected ? formatProgress(file.progress) : '未选择下载' }}
                  </small>
                  <span class="episode-progress">
                    <i :style="{ width: `${Math.max(0, Math.min(100, file.progress * 100))}%` }" />
                  </span>
                </span>
                <NIcon v-if="file.available" :component="Play" size="17" />
              </button>
            </div>
            <NEmpty v-else description="种子中没有支持的图片、视频或音频" />
          </NCard>

          <NCard :bordered="false" class="playback-panel other-panel">
            <template #header>
              <div class="panel-heading">
                <span>其他种子</span>
                <NTag size="small">{{ otherTorrents.length }}</NTag>
              </div>
            </template>
            <NAlert v-if="otherError" type="warning" :bordered="false">{{ otherError }}</NAlert>
            <div v-else-if="otherTorrents.length" class="other-torrent-list">
              <button
                v-for="torrent in otherTorrents"
                :key="`${torrent.site_id}:${torrent.id}`"
                type="button"
                class="other-torrent"
                @click="openOther(torrent)"
              >
                <div class="other-cover">
                  <img
                    v-if="otherCoverURL(torrent)"
                    :src="otherCoverURL(torrent)"
                    :alt="torrent.title"
                    loading="lazy"
                  />
                  <NIcon v-else :component="Film" size="22" />
                </div>
                <span>
                  <strong>{{ torrent.title }}</strong>
                  <small>{{ torrent.site_id }} · {{ formatDate(torrent.published_at) }}</small>
                </span>
              </button>
            </div>
            <NEmpty v-else description="暂无其他含完整音视频的种子" />
          </NCard>
        </aside>
      </main>
    </template>
  </section>
</template>
