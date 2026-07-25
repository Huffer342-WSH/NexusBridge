<!-- 媒体视图负责种子筛选、自适应卡片布局和 qB 状态展示。 -->
<script setup lang="ts">
import { ExternalLink, Film, Play, RadioTower, RefreshCw, Search } from '@lucide/vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { NButton, NCard, NEmpty, NIcon, NInput, NPagination, NSelect, NSpace, NSwitch, NTag } from 'naive-ui';
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router';
import { useMediaDisplaySettings } from '../composables/useMediaDisplaySettings';
import type { Site, Torrent } from '../types';
import { formatByteSize, formatByteSpeed } from '../utils/format';
import MediaQuickSettings from './MediaQuickSettings.vue';
import TorrentStatusControl from './TorrentStatusControl.vue';

const props = defineProps<{
  sites: Site[];
  torrents: Torrent[];
  total: number;
  loading: boolean;
  qbUrl: string;
  qbSyncing: boolean;
  qbActioning: string;
}>();

const emit = defineEmits<{
  download: [torrent: Torrent];
  syncQb: [];
  openQb: [];
  controlQb: [torrent: Torrent, action: 'start' | 'stop'];
  queryChange: [query: { site_id?: string; q?: string; offset: number; limit: number; include_pinned: boolean }];
}>();

const route = useRoute();
const router = useRouter();
const activeSite = ref('all');
const query = ref('');
const includePinned = ref(true);
const page = ref(1);
const pageSize = ref(50);
const failedCovers = ref(new Set<string>());
let searchTimer: ReturnType<typeof setTimeout> | undefined;
const { settings: displaySettings, layoutClass, layoutStyle } = useMediaDisplaySettings();
const pageSizes = new Set([20, 50, 100]);
let applyingRoute = false;

const siteOptions = computed(() => [
  { label: '全部站点', value: 'all' },
  ...props.sites.map((site) => ({
    label: site.name,
    value: site.id,
  })),
]);

const siteNameByID = computed(() => {
  const names = new Map<string, string>();
  for (const site of props.sites) {
    names.set(site.id, site.name);
  }
  return names;
});

function emitQuery() {
  emit('queryChange', {
    site_id: activeSite.value === 'all' ? undefined : activeSite.value,
    q: query.value.trim() || undefined,
    offset: (page.value - 1) * pageSize.value,
    limit: pageSize.value,
    include_pinned: includePinned.value,
  });
}

function routeQueryValue(name: string) {
  const value = route.query[name];
  return Array.isArray(value) ? (value[0] ?? '') : (value ?? '');
}

function positiveInteger(value: string, fallback: number) {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function mediaURLQuery(): LocationQueryRaw {
  const result: LocationQueryRaw = {};
  if (page.value !== 1) result.page = String(page.value);
  if (pageSize.value !== 50) result.page_size = String(pageSize.value);
  if (activeSite.value !== 'all') result.site = activeSite.value;
  const normalizedQuery = query.value.trim();
  if (normalizedQuery) result.q = normalizedQuery;
  if (!includePinned.value) result.pinned = '0';
  return result;
}

function replaceMediaURL() {
  if (route.name !== 'media') return;
  const target = { name: 'media', query: mediaURLQuery() };
  if (router.resolve(target).fullPath === route.fullPath) {
    emitQuery();
    return;
  }
  void router.replace(target);
}

function applyRouteQuery() {
  if (route.name !== 'media') return;
  const routePageSize = positiveInteger(String(routeQueryValue('page_size')), 50);
  if (searchTimer) {
    clearTimeout(searchTimer);
    searchTimer = undefined;
  }
  applyingRoute = true;
  activeSite.value = String(routeQueryValue('site')).trim() || 'all';
  query.value = String(routeQueryValue('q')).trim();
  includePinned.value = routeQueryValue('pinned') !== '0';
  page.value = positiveInteger(String(routeQueryValue('page')), 1);
  pageSize.value = pageSizes.has(routePageSize) ? routePageSize : 50;
  applyingRoute = false;

  const target = { name: 'media', query: mediaURLQuery() };
  if (router.resolve(target).fullPath !== route.fullPath) {
    void router.replace(target);
    return;
  }
  emitQuery();
}

function resetPageAndSyncURL() {
  if (applyingRoute) return;
  if (page.value !== 1) page.value = 1;
  else replaceMediaURL();
}

watch(() => route.fullPath, applyRouteQuery, { immediate: true, flush: 'sync' });
watch([activeSite, includePinned, pageSize], resetPageAndSyncURL, { flush: 'sync' });
watch(
  page,
  () => {
    if (!applyingRoute) replaceMediaURL();
  },
  { flush: 'sync' },
);
watch(
  query,
  () => {
    if (applyingRoute) return;
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(resetPageAndSyncURL, 300);
  },
  { flush: 'sync' },
);
onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer);
});

/** 返回站点显示名称。 */
function siteDisplayName(siteID: string) {
  return siteNameByID.value.get(siteID) ?? siteID;
}

/** 格式化日期时间。 */
function formatDate(value?: string) {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

/** 返回通过后端同源代理加载的封面地址。 */
function coverProxyURL(torrent: Torrent) {
  return `/api/torrents/${encodeURIComponent(torrent.site_id)}/${encodeURIComponent(torrent.id)}/cover`;
}

/** 返回封面是否可以尝试显示。 */
function canShowCover(torrent: Torrent) {
  return Boolean(torrent.cover_url) && !failedCovers.value.has(`${torrent.site_id}:${torrent.id}`);
}

/** 封面代理加载失败时切换到站点占位图。 */
function markCoverFailed(torrent: Torrent) {
  failedCovers.value = new Set(failedCovers.value).add(`${torrent.site_id}:${torrent.id}`);
}

/** 在浏览器新标签页打开已关联 qB 任务的播放页。 */
function openPlayback(torrent: Torrent) {
  if (!torrent.qb_status?.added) return;
  const target = router.resolve({
    name: 'playback',
    params: { site_id: torrent.site_id, torrent_id: torrent.id },
  });
  window.open(target.href, '_blank', 'noopener,noreferrer');
}

/** 解析 qB 返回的逗号分隔标签。 */
function qbTags(torrent: Torrent) {
  return (torrent.qb_status?.tags ?? '')
    .split(',')
    .map((tag) => tag.trim())
    .filter(Boolean);
}
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div class="media-toolbar">
        <div>
          <h2>媒体</h2>
          <p class="muted">汇总展示所有站点缓存结果，也可以切换到单个站点查看。</p>
        </div>
        <div class="media-toolbar-controls">
          <NSpace class="media-filters">
            <NSelect v-model:value="activeSite" :options="siteOptions" class="site-filter" />
            <NInput v-model:value="query" clearable placeholder="搜索标题、分类或站点">
              <template #prefix><NIcon :component="Search" /></template>
            </NInput>
          </NSpace>
          <NSpace align="center"><span class="muted">显示置顶</span><NSwitch v-model:value="includePinned" /></NSpace>
          <NSpace>
            <NButton secondary :loading="qbSyncing" @click="emit('syncQb')">
              <template #icon><NIcon :component="RefreshCw" /></template>
              同步 qB
            </NButton>
            <NButton secondary :disabled="!qbUrl" @click="emit('openQb')">
              <template #icon><NIcon :component="ExternalLink" /></template>
              打开 qB WebUI
            </NButton>
          </NSpace>
        </div>
      </div>
    </NCard>

    <div v-if="torrents.length" class="media-list" :class="layoutClass" :style="layoutStyle">
      <NCard v-for="torrent in torrents" :key="`${torrent.site_id}:${torrent.id}`" :bordered="false" class="media-item">
        <div class="media-poster">
          <img
            v-if="canShowCover(torrent)"
            :src="coverProxyURL(torrent)"
            :alt="torrent.title"
            loading="lazy"
            @error="markCoverFailed(torrent)"
          />
          <div v-else class="media-poster-placeholder">
            <NIcon :component="Film" size="28" />
            <span>{{ torrent.site_id }}</span>
          </div>
          <NButton
            v-if="torrent.qb_status?.added"
            class="card-play-button"
            type="primary"
            circle
            aria-label="在新标签页播放"
            @click.stop="openPlayback(torrent)"
          >
            <template #icon><NIcon :component="Play" /></template>
          </NButton>
          <TorrentStatusControl
            class="card-status-control"
            :torrent="torrent"
            :loading="qbActioning === `${torrent.site_id}:${torrent.id}`"
            @download="emit('download', $event)"
            @control="(item, action) => emit('controlQb', item, action)"
          />
        </div>

        <div class="media-body">
          <div class="media-title-row">
            <div>
              <button
                v-if="torrent.qb_status?.added"
                type="button"
                class="media-title-link"
                title="在新标签页播放"
                @click="openPlayback(torrent)"
              >
                <h3 :class="`media-title-${displaySettings.titleMode}`">{{ torrent.title }}</h3>
              </button>
              <h3 v-else :class="`media-title-${displaySettings.titleMode}`">{{ torrent.title }}</h3>
              <p class="muted">{{ torrent.published_text || formatDate(torrent.published_at) }}</p>
            </div>
            <TorrentStatusControl
              class="list-status-control"
              :torrent="torrent"
              :loading="qbActioning === `${torrent.site_id}:${torrent.id}`"
              @download="emit('download', $event)"
              @control="(item, action) => emit('controlQb', item, action)"
            />
          </div>

          <NSpace class="media-tags">
            <NTag v-if="torrent.sticky_level > 0" round size="small" type="warning"
              >置顶 {{ torrent.sticky_level }}</NTag
            >
            <NTag round size="small">{{ siteDisplayName(torrent.site_id) }}</NTag>
            <NTag v-if="torrent.category" round size="small" type="info">{{ torrent.category }}</NTag>
            <NTag v-if="torrent.promotion" round size="small" type="success">{{ torrent.promotion }}</NTag>
            <NTag round size="small">{{ formatByteSize(torrent.size_bytes) }}</NTag>
          </NSpace>

          <div v-if="torrent.qb_status?.added" class="qb-status-block">
            <div class="qb-status-header">
              <span class="muted">最近同步</span>
              <span v-if="torrent.qb_status.fetched_at" class="muted">{{
                formatDate(torrent.qb_status.fetched_at)
              }}</span>
            </div>
            <NSpace v-if="torrent.qb_status?.added" size="small" class="qb-status-tags">
              <NTag v-if="torrent.qb_status.category" size="small" type="info">{{ torrent.qb_status.category }}</NTag>
              <NTag v-for="tag in qbTags(torrent)" :key="tag" size="small">{{ tag }}</NTag>
            </NSpace>
            <p v-if="torrent.qb_status?.added" class="muted qb-transfer">
              ↓ {{ formatByteSpeed(torrent.qb_status.download_speed) }} · ↑
              {{ formatByteSpeed(torrent.qb_status.upload_speed) }}
            </p>
            <p v-if="torrent.qb_status?.save_path" class="muted text-break">{{ torrent.qb_status.save_path }}</p>
          </div>

          <div class="media-metrics">
            <span>
              <NIcon :component="RadioTower" />
              Seed {{ torrent.seeders ?? '-' }}
            </span>
            <span>Leech {{ torrent.leechers ?? '-' }}</span>
            <span>Snatch {{ torrent.snatches ?? '-' }}</span>
            <span>Comments {{ torrent.comments ?? '-' }}</span>
          </div>
        </div>
      </NCard>
    </div>

    <NCard v-else :bordered="false">
      <NEmpty :description="loading ? 'Loading media...' : 'No cached torrents matched'" />
    </NCard>

    <NCard :bordered="false">
      <NPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :item-count="total"
        :page-sizes="[20, 50, 100]"
        show-size-picker
      />
    </NCard>

    <MediaQuickSettings v-model="displaySettings" />
  </section>
</template>
