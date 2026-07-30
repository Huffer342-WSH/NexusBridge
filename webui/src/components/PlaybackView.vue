<!-- 播放视图负责实时选集、媒体画布、简介和其他可播放种子。 -->
<script setup lang="ts">
import { ExternalLink, File, Film, Folder, Image, Music2, Play, RefreshCw, Trash2 } from '@lucide/vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { NAlert, NButton, NCard, NEmpty, NIcon, NResult, NSpin, NTabPane, NTabs, NTag } from 'naive-ui';
import { useRoute, useRouter } from 'vue-router';
import { api } from '../api';
import type {
  PlaybackContext,
  PlaybackMedia,
  PlaybackMediaType,
  PlaybackTorrent,
  FileBrowseResult,
  FileEntry,
  SeriesVideo,
} from '../types';
import { formatByteSize } from '../utils/format';
import QBDeleteDialog from './QBDeleteDialog.vue';
import MediaThumbnail from './MediaThumbnail.vue';
import MediaCanvas from './player/MediaCanvas.vue';

const route = useRoute();
const router = useRouter();
const loading = ref(true);
const updating = ref(false);
const error = ref('');
const otherError = ref('');
const playback = ref<PlaybackContext | null>(null);
const otherTorrents = ref<PlaybackTorrent[]>([]);
const selectedIndex = ref<number | null>(null);
const playerError = ref('');
const autoplayBlocked = ref(false);
const mediaTab = ref('episodes');
const directory = ref<FileBrowseResult | null>(null);
const directoryLoading = ref(false);
const directoryError = ref('');
const deleteDialogOpen = ref(false);
const deleteLoading = ref(false);
const deleteError = ref('');
const deletedQB = ref<{ deleteFiles: boolean } | null>(null);
let generation = 0;
let pollTimer: number | undefined;
let skipNextRouteLoad = false;
let preserveDirectoryOnNextLoad = false;

const siteID = computed(() => routeParam('site_id'));
const torrentID = computed(() => routeParam('torrent_id'));
const qbHash = computed(() => routeParam('hash'));
const seriesID = computed(() => routeParam('series_id'));
const routeFileName = computed(() => routeQuery('file'));
const routeFilePath = computed(() => routeQuery('path'));
const currentFile = computed(() => playback.value?.files.find((item) => item.index === selectedIndex.value) ?? null);
const coverURL = computed(() => {
  const torrent = playback.value?.torrent;
  if (!torrent?.cover_url) return '';
  return `/api/torrents/${encodeURIComponent(torrent.site_id)}/${encodeURIComponent(torrent.id)}/cover`;
});
const description = computed(() => {
  const torrent = playback.value?.torrent;
  if (!torrent) {
    if (playback.value?.source === 'qb') return '该文件来自 qB 任务，但没有匹配到数据库种子详情。';
    return '该文件没有匹配到 qB 任务或数据库种子，仅提供本机源文件播放。';
  }
  return torrent.detail_description?.trim() || torrent.description?.trim() || torrent.subtitle?.trim() || '暂无简介';
});
const hasActiveDownloads = computed(
  () => playback.value?.files.some((item) => item.selected && item.progress < 1) ?? false,
);

function routeParam(name: string) {
  const value = route.params[name];
  return Array.isArray(value) ? (value[0] ?? '') : String(value ?? '');
}

function routeQuery(name: string) {
  const value = route.query[name];
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

function applyManifest(next: PlaybackContext, keepSelection: boolean) {
  const previous = keepSelection ? selectedIndex.value : null;
  playback.value = next;
  if (!keepSelection && next.series) mediaTab.value = 'series';
  else if (!keepSelection && next.source === 'file') mediaTab.value = 'files';
  const retained = previous == null ? undefined : next.files.find((item) => item.index === previous && item.available);
  selectedIndex.value = retained?.index ?? next.current_file_index ?? next.default_file_index ?? null;
  document.title = `${next.title} - NexusBridge`;
}

async function browsePlaybackDirectory(path: string) {
  if (!path || directoryLoading.value) return;
  directoryLoading.value = true;
  directoryError.value = '';
  try {
    directory.value = await api.browseFiles(path);
  } catch (reason) {
    directoryError.value = reason instanceof Error ? reason.message : '目录读取失败';
  } finally {
    directoryLoading.value = false;
  }
}

function playbackRoute(next: PlaybackContext, retainSeries = true) {
  if (retainSeries && next.series) {
    return {
      name: 'playback-series',
      params: { series_id: next.series.id },
      query: next.current_path ? { path: next.current_path } : {},
    };
  }
  const file = next.files.find((item) => item.index === next.current_file_index)?.name;
  if (next.source === 'torrent' && next.torrent) {
    return {
      name: 'playback',
      params: { site_id: next.torrent.site_id, torrent_id: next.torrent.id },
      query: file ? { file } : {},
    };
  }
  if (next.source === 'qb' && next.qb_hash) {
    return {
      name: 'playback-qb',
      params: { hash: next.qb_hash },
      query: file ? { file } : {},
    };
  }
  return { name: 'playback-file', query: { path: next.current_path } };
}

async function normalizePlaybackURL(next: PlaybackContext) {
  const target = playbackRoute(next);
  if (router.resolve(target).fullPath === route.fullPath) return;
  skipNextRouteLoad = true;
  await router.replace(target);
}

async function refreshManifest(token: number, keepSelection: boolean, preserveDirectory = false) {
  let next: PlaybackContext;
  if (route.name === 'playback-series') {
    next = await api.getSeriesPlayback(seriesID.value, routeFilePath.value);
  } else if (route.name === 'playback-file') {
    next = await api.getFilePlayback(routeFilePath.value);
  } else if (route.name === 'playback-qb') {
    next = await api.getQBPlayback(qbHash.value, routeFileName.value);
  } else {
    next = await api.getTorrentPlayback(siteID.value, torrentID.value, routeFileName.value);
  }
  if (token !== generation) return;
  applyManifest(next, keepSelection);
  if (!keepSelection && !preserveDirectory && next.current_directory) {
    await browsePlaybackDirectory(next.current_directory);
  }
  if (!keepSelection) await normalizePlaybackURL(next);
}

function schedulePolling(token: number) {
  if (pollTimer) window.clearTimeout(pollTimer);
  if (!hasActiveDownloads.value || playback.value?.series) return;
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
  const hadPlayback = playback.value !== null;
  const preserveDirectory = preserveDirectoryOnNextLoad;
  preserveDirectoryOnNextLoad = false;
  if (pollTimer) window.clearTimeout(pollTimer);
  loading.value = !hadPlayback;
  updating.value = hadPlayback;
  error.value = '';
  otherError.value = '';
  playerError.value = '';
  autoplayBlocked.value = false;

  try {
    await refreshManifest(token, false, preserveDirectory);
    if (token !== generation) return;
    loading.value = false;
    updating.value = false;
    schedulePolling(token);
  } catch (reason) {
    if (token !== generation) return;
    const message = reason instanceof Error ? reason.message : '播放清单加载失败';
    if (hadPlayback) playerError.value = `切换媒体失败：${message}`;
    else error.value = message;
    loading.value = false;
    updating.value = false;
    return;
  }

  if ((playback.value as PlaybackContext | null)?.source === 'file') {
    otherTorrents.value = [];
    return;
  }

  try {
    const currentPlayback = playback.value as PlaybackContext | null;
    const items = await api.getPlaybackTorrents(
      currentPlayback?.torrent?.site_id ?? '',
      currentPlayback?.torrent?.id ?? '',
      20,
    );
    if (token === generation) otherTorrents.value = items;
  } catch (reason) {
    if (token === generation) {
      otherError.value = reason instanceof Error ? reason.message : '其他种子加载失败';
    }
  }
}

async function selectFile(file: PlaybackMedia) {
  if (!file.available) return;
  playerError.value = '';
  autoplayBlocked.value = false;
  const context = playback.value;
  if (!context) return;
  const target = playbackRoute({ ...context, current_file_index: file.index }, false);
  if (router.resolve(target).fullPath !== route.fullPath) await router.push(target);
}

async function selectSeriesFile(file: SeriesVideo) {
  const context = playback.value;
  if (!context?.series || !file.available || file.path === context.current_path) return;
  playerError.value = '';
  autoplayBlocked.value = false;
  try {
    await api.selectSeriesVideo(context.series.id, { path: file.path });
    preserveDirectoryOnNextLoad = true;
    await router.push({
      name: 'playback-series',
      params: { series_id: context.series.id },
      query: { path: file.path },
    });
  } catch (reason) {
    playerError.value = reason instanceof Error ? reason.message : '剧集选集切换失败';
  }
}

async function openDirectoryEntry(file: FileEntry) {
  if (file.is_dir) {
    await browsePlaybackDirectory(file.path);
    return;
  }
  if (file.media_type && file.path !== playback.value?.current_path) {
    preserveDirectoryOnNextLoad = true;
    await router.push({ name: 'playback-file', query: { path: file.path } });
  }
}

async function openOther(torrent: PlaybackTorrent) {
  await router.push({ name: 'playback', params: { site_id: torrent.site_id, torrent_id: torrent.id } });
  window.scrollTo({ top: 0, behavior: 'smooth' });
}

/** 打开当前播放内容对应 qB 任务的删除确认框。 */
function openDeleteDialog() {
  deleteError.value = '';
  deleteDialogOpen.value = true;
}

/** 删除当前播放内容对应的 qB 任务，并停止继续读取已经失效的播放清单。 */
async function deleteQBTask(deleteFiles: boolean) {
  const hash = playback.value?.qb_hash?.trim() ?? '';
  if (!hash || deleteLoading.value) {
    deleteError.value = '当前播放内容没有可删除的 qB 任务 hash。';
    return;
  }
  deleteLoading.value = true;
  deleteError.value = '';
  try {
    await api.deleteQBTorrent(hash, deleteFiles);
    generation++;
    if (pollTimer) window.clearTimeout(pollTimer);
    deletedQB.value = { deleteFiles };
    deleteDialogOpen.value = false;
  } catch (reason) {
    deleteError.value = reason instanceof Error ? reason.message : '删除 qB 任务失败';
  } finally {
    deleteLoading.value = false;
  }
}

watch(
  () => route.fullPath,
  () => {
    if (skipNextRouteLoad) {
      skipNextRouteLoad = false;
      return;
    }
    void loadPlayback();
  },
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
      <span>正在识别媒体和文件归属…</span>
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

    <NResult
      v-else-if="deletedQB"
      status="success"
      title="qB 任务已删除"
      :description="deletedQB.deleteFiles ? 'qBittorrent 已同时删除下载文件。' : '下载文件已保留。'"
    >
      <template #footer>
        <NButton type="primary" @click="router.replace({ name: 'media' })">返回媒体页</NButton>
      </template>
    </NResult>

    <template v-else-if="playback">
      <main class="playback-main">
        <div class="playback-primary">
          <div class="playback-stage">
            <div v-if="updating" class="playback-updating">
              <NSpin size="small" />
              <span>正在切换…</span>
            </div>
            <MediaCanvas
              v-if="currentFile"
              :media="currentFile"
              :poster="coverURL"
              :title="playback.title"
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
                <h1>{{ playback.title }}</h1>
                <p class="muted">
                  <template v-if="playback.torrent">
                    {{ playback.torrent.site_id }}
                    <template v-if="playback.torrent.published_at">
                      · {{ formatDate(playback.torrent.published_at) }}
                    </template>
                  </template>
                  <template v-else-if="playback.source === 'qb'">qB 任务 · {{ playback.qb_hash }}</template>
                  <template v-else>本机文件</template>
                </p>
              </div>
              <NSpace class="playback-title-actions">
                <NButton
                  v-if="playback.torrent?.detail_url"
                  tag="a"
                  :href="playback.torrent.detail_url"
                  target="_blank"
                  rel="noopener noreferrer"
                  secondary
                >
                  <template #icon><NIcon :component="ExternalLink" /></template>
                  查看来源
                </NButton>
                <NButton v-if="playback.qb_hash" type="error" secondary @click="openDeleteDialog">
                  <template #icon><NIcon :component="Trash2" /></template>
                  删除 qB 任务
                </NButton>
              </NSpace>
            </div>
            <div class="playback-meta">
              <NTag v-if="playback.torrent?.category" type="info">{{ playback.torrent.category }}</NTag>
              <NTag v-if="playback.qb_status">{{ playback.qb_status.state || 'qB 已关联' }}</NTag>
              <NTag v-if="currentFile" :type="currentFile.complete ? 'success' : 'warning'">
                {{ mediaLabel(currentFile.media_type) }} · {{ formatByteSize(currentFile.size) }}
              </NTag>
              <NTag v-if="currentFile?.subtitles?.length" type="info">
                内嵌字幕 · {{ currentFile.subtitles.length }}
              </NTag>
            </div>
            <p v-if="currentFile" class="current-media-name">{{ currentFile.name }}</p>
            <p v-if="playback.source !== 'file'" class="playback-summary">{{ description }}</p>
          </NCard>
        </div>

        <aside class="playback-sidebar">
          <NCard :bordered="false" class="playback-panel media-browser-panel">
            <NTabs v-model:value="mediaTab" type="line" animated>
              <NTabPane v-if="playback.series" name="series">
                <template #tab>
                  <span>剧集</span>
                  <NTag size="small">
                    {{ playback.series.available_video_count }} / {{ playback.series.video_count }}
                  </NTag>
                </template>
                <NAlert
                  v-if="playback.series.scan_errors.length"
                  type="warning"
                  :bordered="false"
                  class="series-playback-alert"
                >
                  部分目录暂时不可读，已保留上次扫描到的文件。
                </NAlert>
                <div v-if="playback.series_files?.length" class="episode-list">
                  <button
                    v-for="file in playback.series_files"
                    :key="file.path"
                    type="button"
                    class="episode-item"
                    :class="{
                      active: file.path === playback.current_path,
                      disabled: !file.available,
                    }"
                    :disabled="!file.available"
                    @click="selectSeriesFile(file)"
                  >
                    <MediaThumbnail :src="file.thumbnail_url" :alt="`${file.name} 缩略图`" media-type="video" />
                    <span class="episode-copy">
                      <strong>{{ file.name }}</strong>
                      <small :title="file.path">
                        {{ file.relative_path }} · {{ formatByteSize(file.size) }}
                        <template v-if="!file.available"> · 目录不可用</template>
                      </small>
                    </span>
                    <NIcon v-if="file.available" :component="Play" size="17" />
                  </button>
                </div>
                <NEmpty v-else description="剧集中没有扫描到支持的视频" />
              </NTabPane>

              <NTabPane v-if="playback.source !== 'file'" name="episodes">
                <template #tab>
                  <span>选集</span>
                  <NTag size="small">{{ playback.files.length }}</NTag>
                </template>
                <div v-if="playback.files.length" class="episode-list">
                  <button
                    v-for="file in playback.files"
                    :key="file.index"
                    type="button"
                    class="episode-item"
                    :class="{
                      active: file.index === selectedIndex,
                      disabled: !file.available,
                    }"
                    :disabled="!file.available"
                    @click="selectFile(file)"
                  >
                    <MediaThumbnail
                      v-if="file.media_type === 'video' || file.media_type === 'image'"
                      :src="file.thumbnail_url"
                      :alt="`${baseName(file.name)} 缩略图`"
                      :media-type="file.media_type"
                    />
                    <NIcon v-else :component="mediaIcon(file.media_type)" size="18" />
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
              </NTabPane>

              <NTabPane name="files">
                <template #tab>
                  <span>文件</span>
                  <NTag size="small">{{ directory?.entries.length ?? 0 }}</NTag>
                </template>
                <p class="directory-path" :title="directory?.path">{{ directory?.path }}</p>
                <NAlert v-if="directoryError" type="warning" :bordered="false">{{ directoryError }}</NAlert>
                <NSpin :show="directoryLoading">
                  <div v-if="directory" class="directory-list">
                    <button
                      v-if="directory.parent"
                      type="button"
                      class="directory-item"
                      @click="browsePlaybackDirectory(directory.parent)"
                    >
                      <NIcon :component="Folder" size="18" />
                      <span><strong>..</strong><small>上一个目录</small></span>
                    </button>
                    <button
                      v-for="file in directory.entries"
                      :key="file.path"
                      type="button"
                      class="directory-item"
                      :class="{
                        active: file.path === playback.current_path,
                        disabled: !file.is_dir && !file.media_type,
                      }"
                      :disabled="(!file.is_dir && !file.media_type) || file.path === playback.current_path"
                      @click="openDirectoryEntry(file)"
                    >
                      <MediaThumbnail
                        v-if="file.media_type === 'video' || file.media_type === 'image'"
                        :src="file.thumbnail_url"
                        :alt="`${file.name} 缩略图`"
                        :media-type="file.media_type"
                        size="compact"
                      />
                      <NIcon
                        v-else
                        :component="file.is_dir ? Folder : file.media_type ? mediaIcon(file.media_type) : File"
                        size="18"
                      />
                      <span>
                        <strong>{{ file.name }}</strong>
                        <small>
                          {{ file.is_dir ? '目录' : file.media_type ? mediaLabel(file.media_type) : '文件' }}
                          <template v-if="!file.is_dir"> · {{ formatByteSize(file.size ?? 0) }}</template>
                        </small>
                      </span>
                      <NIcon v-if="!file.is_dir && file.media_type" :component="Play" size="17" />
                    </button>
                  </div>
                </NSpin>
              </NTabPane>
            </NTabs>
          </NCard>

          <NCard v-if="playback.source !== 'file'" :bordered="false" class="playback-panel other-panel">
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

    <QBDeleteDialog
      v-model:show="deleteDialogOpen"
      :title="playback?.title ?? ''"
      :loading="deleteLoading"
      :error="deleteError"
      @confirm="deleteQBTask"
    />
  </section>
</template>
