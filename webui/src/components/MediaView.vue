<!-- 媒体视图负责种子筛选、自适应卡片布局和 qB 状态展示。 -->
<script setup lang="ts">
import { ExternalLink, Film, RadioTower, RefreshCw, Search } from '@lucide/vue';
import { computed, ref } from 'vue';
import { NButton, NCard, NEmpty, NIcon, NInput, NSelect, NSpace, NTag } from 'naive-ui';
import { useMediaDisplaySettings } from '../composables/useMediaDisplaySettings';
import type { Site, Torrent } from '../types';
import { formatByteSize, formatByteSpeed } from '../utils/format';
import MediaQuickSettings from './MediaQuickSettings.vue';
import TorrentStatusControl from './TorrentStatusControl.vue';

const props = defineProps<{
  sites: Site[];
  torrents: Torrent[];
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
}>();

const activeSite = ref('all');
const query = ref('');
const failedCovers = ref(new Set<string>());
const { settings: displaySettings, layoutClass, layoutStyle } = useMediaDisplaySettings();

const siteOptions = computed(() => [
  { label: `全部站点 (${props.torrents.length})`, value: 'all' },
  ...props.sites.map((site) => ({
    label: `${site.name} (${props.torrents.filter((torrent) => torrent.site_id === site.id).length})`,
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

const filteredTorrents = computed(() => {
  const keyword = query.value.trim().toLowerCase();
  return props.torrents.filter((torrent) => {
    if (activeSite.value !== 'all' && torrent.site_id !== activeSite.value) {
      return false;
    }
    if (!keyword) {
      return true;
    }
    return [
      torrent.title,
      torrent.category,
      torrent.promotion,
      siteNameByID.value.get(torrent.site_id),
      torrent.site_id,
    ]
      .filter(Boolean)
      .some((value) => value?.toLowerCase().includes(keyword));
  });
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

/** 解析 qB 返回的逗号分隔标签。 */
function qbTags(torrent: Torrent) {
	return (torrent.qb_status?.tags ?? '').split(',').map((tag) => tag.trim()).filter(Boolean);
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

    <div v-if="filteredTorrents.length" class="media-list" :class="layoutClass" :style="layoutStyle">
      <NCard v-for="torrent in filteredTorrents" :key="`${torrent.site_id}:${torrent.id}`" :bordered="false" class="media-item">
        <div class="media-poster">
          <img v-if="canShowCover(torrent)" :src="coverProxyURL(torrent)" :alt="torrent.title" loading="lazy" @error="markCoverFailed(torrent)" />
          <div v-else class="media-poster-placeholder">
            <NIcon :component="Film" size="28" />
            <span>{{ torrent.site_id }}</span>
          </div>
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
              <h3 :class="`media-title-${displaySettings.titleMode}`">{{ torrent.title }}</h3>
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
            <NTag round size="small">{{ siteDisplayName(torrent.site_id) }}</NTag>
            <NTag v-if="torrent.category" round size="small" type="info">{{ torrent.category }}</NTag>
            <NTag v-if="torrent.promotion" round size="small" type="success">{{ torrent.promotion }}</NTag>
            <NTag round size="small">{{ formatByteSize(torrent.size_bytes) }}</NTag>
          </NSpace>

			<div v-if="torrent.qb_status?.added" class="qb-status-block">
				<div class="qb-status-header">
					<span class="muted">最近同步</span>
					<span v-if="torrent.qb_status.fetched_at" class="muted">{{ formatDate(torrent.qb_status.fetched_at) }}</span>
				</div>
				<NSpace v-if="torrent.qb_status?.added" size="small" class="qb-status-tags">
					<NTag v-if="torrent.qb_status.category" size="small" type="info">{{ torrent.qb_status.category }}</NTag>
					<NTag v-for="tag in qbTags(torrent)" :key="tag" size="small">{{ tag }}</NTag>
				</NSpace>
				<p v-if="torrent.qb_status?.added" class="muted qb-transfer">
					↓ {{ formatByteSpeed(torrent.qb_status.download_speed) }} · ↑ {{ formatByteSpeed(torrent.qb_status.upload_speed) }}
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

	<MediaQuickSettings v-model="displaySettings" />
  </section>
</template>
