<!-- 媒体视图负责种子筛选、自适应卡片布局和 qB 状态展示。 -->
<script setup lang="ts">
import {
  ChevronDown,
  ExternalLink,
  Film,
  ListFilter,
  Play,
  RadioTower,
  RefreshCw,
  RotateCcw,
  Search,
  Trash2,
} from '@lucide/vue';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NEmpty,
  NIcon,
  NInput,
  NPagination,
  NPopover,
  NSelect,
  NSpace,
  NSpin,
  NSwitch,
  NTag,
} from 'naive-ui';
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router';
import { api } from '../api';
import { useMediaDisplaySettings } from '../composables/useMediaDisplaySettings';
import type { MediaFilterOptions, Site, Torrent, TorrentPageQuery } from '../types';
import { formatByteSize, formatByteSpeed } from '../utils/format';
import MediaQuickSettings from './MediaQuickSettings.vue';
import QBDeleteDialog from './QBDeleteDialog.vue';
import TorrentStatusControl from './TorrentStatusControl.vue';

const props = defineProps<{
  sites: Site[];
  torrents: Torrent[];
  total: number;
  loading: boolean;
  qbUrl: string;
  qbSyncing: boolean;
  qbActioning: string;
  qbConnected: boolean | null;
  qbStatusReady: boolean;
}>();

const emit = defineEmits<{
  download: [torrent: Torrent];
  syncQb: [];
  openQb: [];
  controlQb: [torrent: Torrent, action: 'start' | 'stop'];
  queryChange: [query: Omit<TorrentPageQuery, 'sort_by' | 'sort_direction'>];
}>();

const route = useRoute();
const router = useRouter();
const activeSite = ref('all');
const query = ref('');
const includePinned = ref(true);
const qbTask = ref<'all' | 'present' | 'absent'>('all');
const qbProgress = ref<'all' | 'complete' | 'incomplete'>('all');
const qbFilterOpen = ref(false);
const categoryFilters = ref<string[]>([]);
const siteCheckboxFilters = ref<Record<string, string[]>>({});
const promotionFilters = ref<string[]>([]);
const mediaFilterOpen = ref(false);
const mediaFilterLoading = ref(false);
const mediaFilterError = ref('');
const mediaFilterOptions = ref<MediaFilterOptions>({
  site_id: '',
  category_label: '分类',
  categories: [],
  checkboxes: [],
  promotion_label: '促销',
  promotions: [],
});
const page = ref(1);
const pageSize = ref(50);
const failedCovers = ref(new Set<string>());
const deleteDialogOpen = ref(false);
const deleteTarget = ref<Torrent | null>(null);
const deletingHash = ref('');
const deleteError = ref('');
const deleteNotice = ref('');
let searchTimer: ReturnType<typeof setTimeout> | undefined;
const { settings: displaySettings, layoutClass, layoutStyle } = useMediaDisplaySettings();
const pageSizes = new Set([20, 50, 100]);
let applyingRoute = false;
let mediaFilterRequestVersion = 0;
type QBFilterValue = 'all' | 'absent' | 'present' | 'incomplete' | 'complete';

const siteOptions = computed(() => [
  { label: '全部站点', value: 'all' },
  ...props.sites.map((site) => ({
    label: site.name,
    value: site.id,
  })),
]);
const qbFilterValue = computed<QBFilterValue>(() => {
  if (qbTask.value === 'all' || qbTask.value === 'absent') return qbTask.value;
  return qbProgress.value === 'all' ? 'present' : qbProgress.value;
});
const qbFilterLabel = computed(() => {
  switch (qbFilterValue.value) {
    case 'absent':
      return '未添加';
    case 'present':
      return '已添加';
    case 'complete':
      return '已添加 / 已完成';
    case 'incomplete':
      return '已添加 / 未完成';
    default:
      return '全部';
  }
});
const qbAbsentChecked = computed(() => qbFilterValue.value === 'absent');
const qbPresentChecked = computed(() => qbFilterValue.value === 'present');
const qbPresentIndeterminate = computed(
  () => qbFilterValue.value === 'complete' || qbFilterValue.value === 'incomplete',
);
const qbCompleteChecked = computed(() => qbFilterValue.value === 'present' || qbFilterValue.value === 'complete');
const qbIncompleteChecked = computed(() => qbFilterValue.value === 'present' || qbFilterValue.value === 'incomplete');
const mediaFilterCount = computed(
  () =>
    categoryFilters.value.length +
    Object.values(siteCheckboxFilters.value).reduce((total, values) => total + values.length, 0) +
    promotionFilters.value.length,
);
const mediaFilterLabel = computed(() => (mediaFilterCount.value ? `已选 ${mediaFilterCount.value} 项` : '站点筛选'));
const mediaFilterDisabled = computed(() => activeSite.value === 'all');

const qbFilterNotice = computed(() => {
  if (qbTask.value === 'all') return '';
  if (!props.qbStatusReady) {
    if (qbTask.value === 'present' && qbProgress.value !== 'all') {
      return '等待首次 qB 状态同步，当前暂时展示全部 qB 任务。';
    }
    return 'qB 实时状态尚未同步，当前使用最近一次任务关联快照。';
  }
  if (props.qbConnected === false) {
    return 'qB 当前断开，筛选结果使用本进程最后一次成功同步的状态。';
  }
  return '';
});

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
    qb_task: qbTask.value === 'all' ? undefined : qbTask.value,
    qb_progress:
      qbTask.value === 'present' && qbProgress.value !== 'all' && props.qbStatusReady ? qbProgress.value : undefined,
    categories: activeSite.value === 'all' || !categoryFilters.value.length ? undefined : categoryFilters.value,
    site_checkboxes:
      activeSite.value === 'all' || !Object.keys(siteCheckboxFilters.value).length
        ? undefined
        : encodedSiteCheckboxFilters(),
    promotions: activeSite.value === 'all' || !promotionFilters.value.length ? undefined : promotionFilters.value,
    offset: (page.value - 1) * pageSize.value,
    limit: pageSize.value,
    include_pinned: includePinned.value,
  });
}

function routeQueryValue(name: string) {
  const value = route.query[name];
  return Array.isArray(value) ? (value[0] ?? '') : (value ?? '');
}

function routeQueryValues(name: string) {
  const value = route.query[name];
  if (Array.isArray(value)) return value.map((item) => String(item ?? '').trim()).filter(Boolean);
  const normalized = String(value ?? '').trim();
  return normalized ? [normalized] : [];
}

function decodedSiteCheckboxFilters() {
  const result: Record<string, string[]> = {};
  for (const encoded of routeQueryValues('site_checkbox')) {
    const separator = encoded.indexOf(':');
    if (separator <= 0) continue;
    const name = encoded.slice(0, separator).trim();
    const value = encoded.slice(separator + 1).trim();
    if (!name || !value) continue;
    const values = result[name] ?? [];
    if (!values.includes(value)) result[name] = [...values, value];
  }
  return result;
}

function encodedSiteCheckboxFilters() {
  return Object.entries(siteCheckboxFilters.value).flatMap(([name, values]) =>
    values.map((value) => `${name}:${value}`),
  );
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
  if (qbTask.value !== 'all') result.qb_task = qbTask.value;
  if (qbTask.value === 'present' && qbProgress.value !== 'all') result.qb_progress = qbProgress.value;
  if (activeSite.value !== 'all' && categoryFilters.value.length) result.category = categoryFilters.value;
  const siteCheckboxes = encodedSiteCheckboxFilters();
  if (activeSite.value !== 'all' && siteCheckboxes.length) result.site_checkbox = siteCheckboxes;
  if (activeSite.value !== 'all' && promotionFilters.value.length) result.promotion = promotionFilters.value;
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
  const routeQBTask = String(routeQueryValue('qb_task')).trim();
  qbTask.value = routeQBTask === 'present' || routeQBTask === 'absent' ? routeQBTask : 'all';
  const routeQBProgress = String(routeQueryValue('qb_progress')).trim();
  qbProgress.value =
    qbTask.value === 'present' && (routeQBProgress === 'complete' || routeQBProgress === 'incomplete')
      ? routeQBProgress
      : 'all';
  categoryFilters.value = activeSite.value === 'all' ? [] : routeQueryValues('category');
  siteCheckboxFilters.value = activeSite.value === 'all' ? {} : decodedSiteCheckboxFilters();
  promotionFilters.value = activeSite.value === 'all' ? [] : routeQueryValues('promotion');
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
watch(
  [activeSite, includePinned, pageSize, qbTask, qbProgress, categoryFilters, siteCheckboxFilters, promotionFilters],
  resetPageAndSyncURL,
  { flush: 'sync' },
);
watch(activeSite, loadMediaFilterOptions, { immediate: true });
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
watch(
  () => props.qbStatusReady,
  () => {
    if (!applyingRoute && qbTask.value !== 'all') replaceMediaURL();
  },
  { flush: 'sync' },
);
watch(
  () => props.total,
  (total) => {
    const lastPage = Math.max(1, Math.ceil(total / pageSize.value));
    if (!applyingRoute && page.value > lastPage) page.value = lastPage;
  },
  { flush: 'sync' },
);
onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer);
});

/** 切换站点时清除只对原站点有效的 checkbox 与促销条件。 */
function updateActiveSite(value: string) {
  applyingRoute = true;
  activeSite.value = value;
  categoryFilters.value = [];
  siteCheckboxFilters.value = {};
  promotionFilters.value = [];
  mediaFilterOpen.value = false;
  applyingRoute = false;
  resetPageAndSyncURL();
}

/** 读取站点定义中的媒体筛选项，并清理 URL 中已失效的值。 */
async function loadMediaFilterOptions() {
  const requestVersion = ++mediaFilterRequestVersion;
  const siteID = activeSite.value;
  mediaFilterError.value = '';
  mediaFilterOptions.value = {
    site_id: '',
    category_label: '分类',
    categories: [],
    checkboxes: [],
    promotion_label: '促销',
    promotions: [],
  };
  if (siteID === 'all') {
    mediaFilterLoading.value = false;
    return;
  }
  mediaFilterLoading.value = true;
  try {
    const options = await api.getMediaFilterOptions(siteID);
    if (requestVersion !== mediaFilterRequestVersion || activeSite.value !== siteID) return;
    mediaFilterOptions.value = options;
    const allowedCategories = new Set(options.categories.map((option) => option.value));
    const allowedPromotions = new Set(options.promotions.map((option) => option.value));
    const nextCategories = categoryFilters.value.filter((value) => allowedCategories.has(value));
    const nextPromotions = promotionFilters.value.filter((value) => allowedPromotions.has(value));
    const nextCheckboxes: Record<string, string[]> = {};
    for (const group of options.checkboxes) {
      const allowed = new Set(group.options.map((option) => option.value));
      const selected = (siteCheckboxFilters.value[group.name] ?? []).filter((value) => allowed.has(value));
      if (selected.length) nextCheckboxes[group.name] = selected;
    }
    if (
      nextCategories.length !== categoryFilters.value.length ||
      JSON.stringify(nextCheckboxes) !== JSON.stringify(siteCheckboxFilters.value) ||
      nextPromotions.length !== promotionFilters.value.length
    ) {
      applyingRoute = true;
      categoryFilters.value = nextCategories;
      siteCheckboxFilters.value = nextCheckboxes;
      promotionFilters.value = nextPromotions;
      applyingRoute = false;
      replaceMediaURL();
    }
  } catch (reason) {
    if (requestVersion !== mediaFilterRequestVersion) return;
    mediaFilterError.value = reason instanceof Error ? reason.message : '站点筛选项加载失败';
  } finally {
    if (requestVersion === mediaFilterRequestVersion) mediaFilterLoading.value = false;
  }
}

/** 切换一个多选筛选值。 */
function toggleMediaFilter(target: 'category' | 'promotion', value: string) {
  const source = target === 'category' ? categoryFilters : promotionFilters;
  source.value = source.value.includes(value)
    ? source.value.filter((item) => item !== value)
    : [...source.value, value];
}

/** 切换站点 checkbox 分组中的一个值。 */
function toggleSiteCheckboxFilter(groupName: string, value: string) {
  const current = siteCheckboxFilters.value[groupName] ?? [];
  const next = current.includes(value) ? current.filter((item) => item !== value) : [...current, value];
  const filters = { ...siteCheckboxFilters.value };
  if (next.length) filters[groupName] = next;
  else delete filters[groupName];
  siteCheckboxFilters.value = filters;
}

/** 清空当前站点的 checkbox 与促销筛选。 */
function resetMediaFilters() {
  applyingRoute = true;
  categoryFilters.value = [];
  siteCheckboxFilters.value = {};
  promotionFilters.value = [];
  applyingRoute = false;
  resetPageAndSyncURL();
}

/** 返回站点显示名称。 */
function siteDisplayName(siteID: string) {
  return siteNameByID.value.get(siteID) ?? siteID;
}

/** 将单选菜单值转换为后端使用的 qB 任务与进度条件。 */
function updateQBFilter(value: QBFilterValue) {
  applyingRoute = true;
  if (value === 'incomplete' || value === 'complete') {
    qbTask.value = 'present';
    qbProgress.value = value;
  } else {
    qbTask.value = value;
    qbProgress.value = 'all';
  }
  applyingRoute = false;
  resetPageAndSyncURL();
}

/** 切换未添加分类，并保持筛选结果可由后端准确分页。 */
function toggleQBAbsent() {
  updateQBFilter(qbFilterValue.value === 'absent' ? 'all' : 'absent');
}

/** 切换已添加分类；半选时补全两个进度子项。 */
function toggleQBPresent() {
  updateQBFilter(qbFilterValue.value === 'present' ? 'all' : 'present');
}

/** 切换一个进度叶节点。 */
function toggleQBProgress(value: 'complete' | 'incomplete') {
  const current = qbFilterValue.value;
  const sibling = value === 'complete' ? 'incomplete' : 'complete';
  if (current === 'present') {
    updateQBFilter(sibling);
    return;
  }
  if (current === value) {
    updateQBFilter('all');
    return;
  }
  updateQBFilter(current === sibling ? 'present' : value);
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

/** 打开删除确认框并默认保留已下载文件。 */
function openDeleteDialog(torrent: Torrent) {
  deleteTarget.value = torrent;
  deleteError.value = '';
  deleteDialogOpen.value = true;
}

/** 删除当前卡片关联的 qB 任务，并立即更新卡片状态。 */
async function deleteQBTask(deleteFiles: boolean) {
  const torrent = deleteTarget.value;
  const hash = torrent?.qb_status?.hash?.trim() ?? '';
  if (!torrent || !hash || deletingHash.value) {
    deleteError.value = '当前卡片没有可删除的 qB 任务 hash，请先同步 qB 状态。';
    return;
  }
  deletingHash.value = hash;
  deleteError.value = '';
  deleteNotice.value = '';
  try {
    await api.deleteQBTorrent(hash, deleteFiles);
    if (torrent.qb_status) {
      torrent.qb_status = {
        ...torrent.qb_status,
        added: false,
        state: 'missing',
        progress: 0,
        completed: 0,
        download_speed: 0,
        upload_speed: 0,
        fetched_at: new Date().toISOString(),
      };
    }
    deleteNotice.value = deleteFiles ? 'qB 任务及已下载文件已删除。' : 'qB 任务已删除，已下载文件保留。';
    deleteDialogOpen.value = false;
  } catch (reason) {
    deleteError.value = reason instanceof Error ? reason.message : '删除 qB 任务失败';
  } finally {
    deletingHash.value = '';
  }
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
            <NSelect :value="activeSite" :options="siteOptions" class="site-filter" @update:value="updateActiveSite" />
            <NPopover
              v-model:show="mediaFilterOpen"
              trigger="click"
              placement="bottom-start"
              :show-arrow="false"
              content-class="media-filter-popover"
            >
              <template #trigger>
                <NButton
                  secondary
                  class="media-filter-trigger"
                  :disabled="mediaFilterDisabled"
                  :title="mediaFilterDisabled ? '请先选择单个站点' : '筛选当前站点的 checkbox 与促销状态'"
                  :aria-expanded="mediaFilterOpen"
                  aria-haspopup="menu"
                >
                  <span class="media-filter-trigger-label">
                    <NIcon :component="ListFilter" />
                    <span>{{ mediaFilterLabel }}</span>
                  </span>
                  <NIcon :component="ChevronDown" class="media-filter-chevron" />
                </NButton>
              </template>
              <div class="media-filter-panel" role="menu" aria-label="站点 checkbox 与促销筛选">
                <div class="media-filter-heading">
                  <div>
                    <strong>{{ siteNameByID.get(activeSite) ?? activeSite }}</strong>
                    <span>同组内匹配任一选项，不同分组同时生效</span>
                  </div>
                  <NButton
                    text
                    size="small"
                    :disabled="mediaFilterCount === 0"
                    class="media-filter-reset"
                    @click="resetMediaFilters"
                  >
                    <template #icon><NIcon :component="RotateCcw" /></template>
                    重置
                  </NButton>
                </div>
                <NSpin v-if="mediaFilterLoading" class="media-filter-loading" size="small" />
                <NAlert v-else-if="mediaFilterError" type="warning" :bordered="false">
                  {{ mediaFilterError }}
                </NAlert>
                <template v-else>
                  <section v-if="mediaFilterOptions.categories.length" class="media-filter-group">
                    <div class="media-filter-group-title">
                      <strong>{{ mediaFilterOptions.category_label }}</strong>
                      <span>{{ categoryFilters.length }} / {{ mediaFilterOptions.categories.length }}</span>
                    </div>
                    <div class="media-filter-options">
                      <NCheckbox
                        v-for="option in mediaFilterOptions.categories"
                        :key="option.value"
                        :checked="categoryFilters.includes(option.value)"
                        class="media-filter-option"
                        @update:checked="toggleMediaFilter('category', option.value)"
                      >
                        {{ option.label }}
                      </NCheckbox>
                    </div>
                  </section>
                  <section v-for="group in mediaFilterOptions.checkboxes" :key="group.name" class="media-filter-group">
                    <div class="media-filter-group-title">
                      <strong>{{ group.label }}</strong>
                      <span>{{ siteCheckboxFilters[group.name]?.length ?? 0 }} / {{ group.options.length }}</span>
                    </div>
                    <div class="media-filter-options">
                      <NCheckbox
                        v-for="option in group.options"
                        :key="option.value"
                        :checked="siteCheckboxFilters[group.name]?.includes(option.value) ?? false"
                        class="media-filter-option"
                        @update:checked="toggleSiteCheckboxFilter(group.name, option.value)"
                      >
                        {{ option.label }}
                      </NCheckbox>
                    </div>
                  </section>
                  <section v-if="mediaFilterOptions.promotions.length" class="media-filter-group">
                    <div class="media-filter-group-title">
                      <strong>{{ mediaFilterOptions.promotion_label }}</strong>
                      <span>{{ promotionFilters.length }} / {{ mediaFilterOptions.promotions.length }}</span>
                    </div>
                    <div class="media-filter-options media-filter-options--promotion">
                      <NCheckbox
                        v-for="option in mediaFilterOptions.promotions"
                        :key="option.value"
                        :checked="promotionFilters.includes(option.value)"
                        class="media-filter-option"
                        @update:checked="toggleMediaFilter('promotion', option.value)"
                      >
                        {{ option.label }}
                      </NCheckbox>
                    </div>
                  </section>
                  <NEmpty
                    v-if="
                      !mediaFilterOptions.categories.length &&
                      !mediaFilterOptions.checkboxes.length &&
                      !mediaFilterOptions.promotions.length
                    "
                    size="small"
                    description="该站点未定义可用的 checkbox 或促销筛选"
                  />
                </template>
                <div class="media-filter-footer">
                  <span>未选择时显示该站点全部种子</span>
                  <strong>{{ mediaFilterCount ? `已选 ${mediaFilterCount} 项` : '未筛选' }}</strong>
                </div>
              </div>
            </NPopover>
            <NPopover
              v-model:show="qbFilterOpen"
              trigger="click"
              placement="bottom-start"
              :show-arrow="false"
              content-class="qb-filter-popover"
            >
              <template #trigger>
                <NButton secondary class="qb-filter-trigger" :aria-expanded="qbFilterOpen" aria-haspopup="menu">
                  <span class="qb-filter-trigger-label">
                    <span class="qb-filter-badge">qB</span>
                    <span>{{ qbFilterLabel }}</span>
                  </span>
                  <NIcon :component="ChevronDown" class="qb-filter-chevron" />
                </NButton>
              </template>
              <div class="qb-filter-menu" role="menu" aria-label="qB 状态筛选">
                <NCheckbox :checked="qbAbsentChecked" class="qb-filter-check" @update:checked="toggleQBAbsent">
                  未添加
                </NCheckbox>
                <NCheckbox
                  :checked="qbPresentChecked"
                  :indeterminate="qbPresentIndeterminate"
                  class="qb-filter-check"
                  @update:checked="toggleQBPresent"
                >
                  已添加
                </NCheckbox>
                <NCheckbox
                  :checked="qbCompleteChecked"
                  class="qb-filter-check qb-filter-check--grandchild"
                  @update:checked="toggleQBProgress('complete')"
                >
                  已完成
                </NCheckbox>
                <NCheckbox
                  :checked="qbIncompleteChecked"
                  class="qb-filter-check qb-filter-check--grandchild"
                  @update:checked="toggleQBProgress('incomplete')"
                >
                  未完成
                </NCheckbox>
                <div class="qb-filter-footer">
                  <span>未选择时显示全部</span>
                  <NButton
                    text
                    size="tiny"
                    :disabled="qbFilterValue === 'all'"
                    class="qb-filter-reset"
                    @click="updateQBFilter('all')"
                  >
                    <template #icon><NIcon :component="RotateCcw" /></template>
                    重置筛选
                  </NButton>
                </div>
              </div>
            </NPopover>
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

    <NAlert v-if="deleteNotice" type="success" closable :bordered="false" @close="deleteNotice = ''">
      {{ deleteNotice }}
    </NAlert>
    <NAlert v-if="qbFilterNotice" type="warning" :bordered="false">
      {{ qbFilterNotice }}
    </NAlert>

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
              <NSpace align="center" size="small">
                <span v-if="torrent.qb_status.fetched_at" class="muted">{{
                  formatDate(torrent.qb_status.fetched_at)
                }}</span>
                <NButton
                  quaternary
                  type="error"
                  size="tiny"
                  :loading="deletingHash === torrent.qb_status.hash"
                  @click.stop="openDeleteDialog(torrent)"
                >
                  <template #icon><NIcon :component="Trash2" /></template>
                  删除任务
                </NButton>
              </NSpace>
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
    <QBDeleteDialog
      v-model:show="deleteDialogOpen"
      :title="deleteTarget?.title ?? ''"
      :loading="Boolean(deletingHash)"
      :error="deleteError"
      @confirm="deleteQBTask"
    />
  </section>
</template>
