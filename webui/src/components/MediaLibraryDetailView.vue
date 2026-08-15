<!-- 媒体库内容页统一展示直接子媒体库与当前节点扫描到的媒体。 -->
<script setup lang="ts">
import { ChevronRight, Film, FolderOpen, Play, RefreshCw } from '@lucide/vue';
import { computed, ref, watch } from 'vue';
import { NAlert, NButton, NCard, NEmpty, NIcon, NSpin } from 'naive-ui';
import { useRoute, useRouter } from 'vue-router';
import { api } from '../api';
import type { MediaLibraryDetail, MediaLibrarySummary, SeriesVideo } from '../types';
import MediaThumbnail from './MediaThumbnail.vue';

const route = useRoute();
const router = useRouter();
const libraries = ref<MediaLibrarySummary[]>([]);
const detail = ref<MediaLibraryDetail | null>(null);
const loading = ref(true);
const scanning = ref(false);
const error = ref('');

const libraryID = computed(() => String(route.params.library_id ?? ''));
const children = computed(() => libraries.value.filter((item) => item.parent_id === libraryID.value));
const breadcrumbs = computed(() => {
  const byID = new Map(libraries.value.map((item) => [item.id, item]));
  const result: MediaLibrarySummary[] = [];
  let current = detail.value ? byID.get(detail.value.id) : undefined;
  const seen = new Set<string>();
  while (current && !seen.has(current.id)) {
    result.unshift(current);
    seen.add(current.id);
    current = current.parent_id ? byID.get(current.parent_id) : undefined;
  }
  return result;
});

async function loadLibrary() {
  if (!libraryID.value) return;
  loading.value = true;
  error.value = '';
  try {
    const [nextLibraries, nextDetail] = await Promise.all([
      api.getMediaLibraries(),
      api.getMediaLibrary(libraryID.value),
    ]);
    libraries.value = nextLibraries;
    detail.value = nextDetail;
    if (nextDetail.kind === 'series') {
      await router.replace({ name: 'playback-series', params: { series_id: nextDetail.id } });
      return;
    }
    if (!nextDetail.last_scanned_at) await scanLibrary();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库内容加载失败';
  } finally {
    loading.value = false;
  }
}

async function scanLibrary() {
  if (!libraryID.value || scanning.value) return;
  scanning.value = true;
  error.value = '';
  try {
    const scanned = await api.scanMediaLibrary(libraryID.value);
    detail.value = scanned;
    const index = libraries.value.findIndex((item) => item.id === scanned.id);
    if (index >= 0) libraries.value[index] = scanned;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库扫描失败';
  } finally {
    scanning.value = false;
  }
}

async function openLibrary(item: MediaLibrarySummary) {
  if (item.kind === 'series') {
    await router.push({ name: 'playback-series', params: { series_id: item.id } });
    return;
  }
  await router.push({ name: 'library-detail', params: { library_id: item.id } });
}

function openMedia(item: SeriesVideo) {
  if (!item.available) return;
  const target = router.resolve({ name: 'playback-file', query: { path: item.path } });
  window.open(target.href, '_blank', 'noopener,noreferrer');
}

function formatBytes(value = 0) {
  if (value < 1024) return `${value} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let size = value / 1024;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index++;
  }
  return `${size >= 10 ? size.toFixed(0) : size.toFixed(1)} ${units[index]}`;
}

function mediaDescription(item: SeriesVideo) {
  const size = formatBytes(item.size);
  if (!item.episode_label && item.relative_path === item.name) return size;
  return `${item.relative_path} · ${size}`;
}

watch(libraryID, loadLibrary, { immediate: true });
</script>

<template>
  <section class="library-content-view">
    <header class="library-content-heading">
      <div>
        <nav class="library-breadcrumbs" aria-label="媒体库路径">
          <button type="button" @click="router.push({ name: 'libraries' })">媒体库</button>
          <template v-for="item in breadcrumbs" :key="item.id">
            <NIcon :component="ChevronRight" />
            <button type="button" @click="openLibrary(item)">{{ item.name }}</button>
          </template>
        </nav>
        <h1>{{ detail?.name || '媒体库' }}</h1>
        <p v-if="detail?.directories.length">{{ detail.directories.map((item) => item.path).join(' · ') }}</p>
      </div>
      <NButton secondary :loading="scanning" @click="scanLibrary">
        <template #icon><NIcon :component="RefreshCw" /></template>
        重新扫描
      </NButton>
    </header>

    <NAlert v-if="error" type="error" :bordered="false">{{ error }}</NAlert>
    <NSpin :show="loading || scanning">
      <NCard :bordered="false" class="library-content-panel">
        <template #header>
          <div class="panel-title">
            <span>内容</span><small>{{ children.length + (detail?.videos.length ?? 0) }}</small>
          </div>
        </template>
        <NEmpty
          v-if="!children.length && !detail?.videos.length"
          description="当前媒体库没有下级或扫描媒体"
        />
        <div v-else class="library-entry-list">
          <button v-for="item in children" :key="`library:${item.id}`" type="button" @click="openLibrary(item)">
            <span class="library-entry-icon">
              <NIcon :component="item.kind === 'series' ? Film : FolderOpen" size="24" />
            </span>
            <span class="library-entry-copy">
              <strong>{{ item.name }}</strong>
              <small>{{ item.available_video_count }} / {{ item.video_count }} 个媒体可用</small>
              <small>{{ item.directories.map((directory) => directory.path).join(' · ') }}</small>
            </span>
            <NIcon :component="ChevronRight" />
          </button>
          <button
            v-for="item in detail?.videos ?? []"
            :key="`media:${item.path}`"
            type="button"
            :disabled="!item.available"
            @click="openMedia(item)"
          >
            <MediaThumbnail
              :src="item.thumbnail_url"
              :alt="`${item.name} 缩略图`"
              media-type="video"
              size="compact"
            />
            <span class="library-entry-copy">
              <strong>{{ item.episode_label || item.name }}</strong>
              <small>{{ mediaDescription(item) }}</small>
            </span>
            <NIcon :component="Play" />
          </button>
        </div>
      </NCard>
    </NSpin>
  </section>
</template>

<style scoped>
.library-content-view {
  display: grid;
  gap: 16px;
}

.library-content-heading,
.panel-title,
.library-entry-list button {
  display: flex;
  align-items: center;
}

.library-content-heading {
  justify-content: space-between;
  gap: 16px;
}

.library-content-heading h1,
.library-content-heading p {
  margin: 0;
}

.library-content-heading p,
.library-entry-list small,
.panel-title small {
  color: #667085;
}

.library-content-heading p {
  margin-top: 5px;
  font-size: 12px;
}

.library-breadcrumbs {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 8px;
  color: #667085;
}

.library-breadcrumbs button {
  border: 0;
  padding: 2px 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}

.library-content-panel :deep(.n-card-header) {
  padding-bottom: 10px;
}

.panel-title {
  justify-content: space-between;
  gap: 12px;
}

.library-entry-list {
  display: grid;
  gap: 8px;
}

.library-entry-list button {
  width: 100%;
  gap: 10px;
  border: 0;
  border-radius: 8px;
  padding: 10px;
  background: #f8fafc;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.library-entry-list button:hover {
  background: #eef4ff;
}

.library-entry-copy {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 2px;
}

.library-entry-icon {
  display: inline-flex;
  width: 56px;
  height: 34px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 5px;
  background: #e8eef7;
  color: #52677f;
}

.library-entry-list strong,
.library-entry-list small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.library-entry-list button:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

@media (max-width: 640px) {
  .library-content-heading {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
