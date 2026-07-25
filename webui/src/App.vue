<!-- 应用壳负责页面导航、共享状态和跨组件操作协调。 -->
<script setup lang="ts">
import {
  BellRing,
  Bot,
  CloudDownload,
  Database,
  FolderKanban,
  FolderOpen,
  KeyRound,
  Network,
  RefreshCw,
  Settings,
  ShieldCheck,
} from '@lucide/vue';
import { computed, onMounted, ref, watch } from 'vue';
import {
  createDiscreteApi,
  NAlert,
  NButton,
  NCard,
  NConfigProvider,
  NForm,
  NFormItem,
  NGlobalStyle,
  NIcon,
  NInput,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NModal,
  NSpace,
  type GlobalThemeOverrides,
} from 'naive-ui';
import { RouterLink, RouterView, useRoute } from 'vue-router';
import { api } from './api';
import { useQBStatusPolling } from './composables/useQBStatusPolling';
import { useSiteFetchJobs } from './composables/useSiteFetchJobs';
import { createDefaultQBittorrentConfig } from './config/qbittorrent';
import { openExternalURL } from './utils/runtime';
import type {
  DownloadPreview,
  DownloadTask,
  Health,
  FetchSettings,
  LLMConfig,
  NetworkConfig,
  OrganizeTask,
  QBittorrentConfig,
  Session,
  Site,
  SiteAttendance,
  SiteFetchJob,
  SiteFetchRequest,
  Torrent,
  TorrentPageQuery,
  QBPollResult,
} from './types';

type PageKey = 'media' | 'files' | 'subscriptions' | 'tasks' | 'settings';
type SettingsPageKey = 'sites' | 'network' | 'llm' | 'qbittorrent' | 'mihomo';
type SiteCredentialDraft = { user_agent: string; cookie: string };

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: '#2563eb',
    primaryColorHover: '#1d4ed8',
    primaryColorPressed: '#1e40af',
    borderRadius: '6px',
  },
  Card: {
    borderRadius: '8px',
  },
  Button: {
    borderRadiusMedium: '6px',
  },
};

const navItems: Array<{ key: PageKey; label: string; icon: typeof CloudDownload; to: string }> = [
  { key: 'media', label: '媒体', icon: CloudDownload, to: '/media' },
  { key: 'files', label: '文件', icon: FolderOpen, to: '/files' },
  { key: 'subscriptions', label: '订阅', icon: BellRing, to: '/subscriptions' },
  { key: 'tasks', label: '任务', icon: FolderKanban, to: '/tasks' },
  { key: 'settings', label: '设置', icon: Settings, to: '/settings/sites' },
];

const settingsItems: Array<{ key: SettingsPageKey; label: string; icon: typeof KeyRound; to: string }> = [
  { key: 'sites', label: '站点', icon: KeyRound, to: '/settings/sites' },
  { key: 'network', label: '网络代理', icon: Network, to: '/settings/network' },
  { key: 'llm', label: 'LLM', icon: Bot, to: '/settings/llm' },
  { key: 'qbittorrent', label: 'qBittorrent', icon: Database, to: '/settings/qbittorrent' },
  { key: 'mihomo', label: 'Mihomo', icon: Network, to: '/settings/mihomo' },
];

const route = useRoute();
const session = ref<Session | null>(null);
const loggedIn = ref(false);
const health = ref<Health | null>(null);
const sites = ref<Site[]>([]);
const torrents = ref<Torrent[]>([]);
const torrentTotal = ref(0);
const mediaQuery = ref<TorrentPageQuery>({
  offset: 0,
  limit: 50,
  include_pinned: true,
  sort_by: 'published_at',
  sort_direction: 'desc',
});
const downloadTasks = ref<DownloadTask[]>([]);
const organizeTasks = ref<OrganizeTask[]>([]);
const siteCredentials = ref<Record<string, SiteCredentialDraft>>({});
const siteAttendances = ref<Record<string, SiteAttendance>>({});
const siteActions = ref<Record<string, 'fetch' | 'save' | 'attendance' | ''>>({});
const fetchSettings = ref<FetchSettings>({ max_pages: 3 });
const fetchJobs = ref<SiteFetchJob[]>([]);
const qbConfig = ref<QBittorrentConfig>(createDefaultQBittorrentConfig());
const llmConfig = ref<LLMConfig>({
  base_url: '',
  api_key: '',
  model: '',
});
const networkConfig = ref<NetworkConfig>({ mode: 'system', proxy_url: '', no_proxy: '' });
const qbTagsText = ref('');
const downloadPreview = ref<DownloadPreview | null>(null);
const downloadDialogOpen = ref(false);
const downloadDialogLoading = ref(false);
const downloadDialogSending = ref(false);
const downloadDialogError = ref('');
const loading = ref(true);
const qbSyncing = ref(false);
const qbActioning = ref('');
const message = ref('');
const username = ref('');
const password = ref('');
const { message: floatingMessage } = createDiscreteApi(['message']);
const qbOptimisticUntil = new Map<string, number>();
const qbOptimisticUpdateDelayMs = 800;
const autoFetchedSites = new Set<string>();

const showLogin = computed(() => session.value?.requires_login && !loggedIn.value);
const activeDownloads = computed(() => downloadTasks.value.filter((task) => task.status !== 'completed').length);
const pendingOrganize = computed(() => organizeTasks.value.filter((task) => task.status !== 'completed').length);
const activePage = computed(() => route.meta.page as PageKey | 'not-found');
const activeSettingsPage = computed(() => route.meta.settingsPage as SettingsPageKey | undefined);
const currentTitle = computed(() => (typeof route.meta.title === 'string' ? route.meta.title : 'NexusBridge'));

/** 为当前路由组件提供所需状态，避免页面组件接管全局状态。 */
const currentViewProps = computed<Record<string, unknown>>(() => {
  switch (route.name) {
    case 'media':
      return {
        sites: sites.value,
        torrents: torrents.value,
        total: torrentTotal.value,
        loading: loading.value,
        qbUrl: qbConfig.value.url,
        qbSyncing: qbSyncing.value,
        qbActioning: qbActioning.value,
      };
    case 'tasks':
      return {
        downloadTasks: downloadTasks.value,
        organizeTasks: organizeTasks.value,
        activeDownloads: activeDownloads.value,
        pendingOrganize: pendingOrganize.value,
      };
    case 'files':
      return { sites: sites.value };
    case 'subscriptions':
      return { sites: sites.value, torrents: torrents.value };
    case 'settings-sites':
      return {
        sites: sites.value,
        credentials: siteCredentials.value,
        attendances: siteAttendances.value,
        actions: siteActions.value,
        fetchSettings: fetchSettings.value,
        fetchJobs: fetchJobs.value,
      };
    case 'settings-llm':
      return { config: llmConfig.value };
    case 'settings-network':
      return { config: networkConfig.value };
    case 'settings-qbittorrent':
      return {
        config: qbConfig.value,
        tagsText: qbTagsText.value,
        connected: qbConnected.value,
        polling: qbPolling.value,
      };
    default:
      return {};
  }
});

/** 为当前路由组件连接现有业务动作。 */
const currentViewListeners = computed((): Record<string, CallableFunction> => {
  switch (route.name) {
    case 'media':
      return {
        download: openDownloadDialog,
        syncQb: syncQBittorrent,
        openQb: openQBittorrent,
        controlQb: controlTorrentQB,
        queryChange: handleMediaQueryChange,
      };
    case 'tasks':
      return { organize: organizePending };
    case 'files':
    case 'settings-mihomo':
      return { message: setMessage };
    case 'settings-sites':
      return {
        updateCredential: updateSiteCredential,
        updateAttendance: updateSiteAttendance,
        save: saveSiteCredential,
        saveAttendance: saveSiteAttendance,
        fetch: fetchSite,
        saveFetchSettings,
      };
    case 'settings-llm':
      return { update: updateLLMConfig, save: saveLLM };
    case 'settings-network':
      return { update: updateNetworkConfig, save: saveNetwork };
    case 'settings-qbittorrent':
      return {
        update: updateQBConfig,
        updateTags: setQBTagsText,
        save: saveQBittorrent,
        sync: syncQBittorrent,
      };
    default:
      return {};
  }
});

/** 将 qB 增量结果合并到当前媒体列表。 */
function applyQBPollResult(result: QBPollResult) {
  const torrentsByKey = new Map(torrents.value.map((torrent) => [`${torrent.site_id}:${torrent.id}`, torrent]));
  for (const update of result.updates) {
    const key = `${update.site_id}:${update.torrent_id}`;
    if ((qbOptimisticUntil.get(key) ?? 0) > Date.now()) continue;
    const torrent = torrentsByKey.get(key);
    if (torrent) torrent.qb_status = update.qb_status;
  }
}

const {
  connected: qbConnected,
  polling: qbPolling,
  trigger: triggerQBPoll,
} = useQBStatusPolling({
  config: qbConfig,
  isForeground: () => loggedIn.value && activePage.value === 'media',
  applyResult: applyQBPollResult,
});

watch(message, (value) => {
  if (!value) return;
  floatingMessage.info(value, { duration: 3200, closable: true });
  message.value = '';
});

/** 更新跨页面提示消息。 */
function setMessage(value: string) {
  message.value = value;
}

/** 更新 qB 标签文本。 */
function setQBTagsText(value: string) {
  qbTagsText.value = value;
}

/** 更新站点凭据草稿。 */
function updateSiteCredential(siteID: string, patch: Partial<SiteCredentialDraft>) {
  siteCredentials.value[siteID] = {
    user_agent: siteCredentials.value[siteID]?.user_agent ?? '',
    cookie: siteCredentials.value[siteID]?.cookie ?? '',
    ...patch,
  };
}

/** 更新 qB 配置草稿。 */
function updateQBConfig(patch: Partial<QBittorrentConfig>) {
  qbConfig.value = { ...qbConfig.value, ...patch };
}

/** 更新 LLM 配置草稿。 */
function updateLLMConfig(patch: Partial<LLMConfig>) {
  llmConfig.value = { ...llmConfig.value, ...patch };
}

/** 更新网络代理配置草稿。 */
function updateNetworkConfig(patch: Partial<NetworkConfig>) {
  networkConfig.value = { ...networkConfig.value, ...patch };
}

/** 更新单个站点的签到配置草稿。 */
function updateSiteAttendance(siteID: string, patch: Partial<SiteAttendance>) {
  const current = siteAttendances.value[siteID];
  if (!current) return;
  siteAttendances.value[siteID] = { ...current, ...patch };
}

function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

/** 刷新后端健康状态、设置和数据库快照。 */
async function refresh(clearMessage = true) {
  loading.value = true;
  if (clearMessage) {
    message.value = '';
  }
  try {
    const [healthData, siteData, torrentData, fetchSettingsData, fetchJobData] = await Promise.all([
      api.health(),
      api.sites(),
      api.torrents(mediaQuery.value),
      api.getFetchSettings(),
      api.getSiteFetchJobs(undefined, 100),
    ]);
    const attendanceRequest = Promise.allSettled(siteData.map((site) => api.getSiteAttendance(site.id)));
    const [qbData, llmData, networkData, downloadData, organizeData] = await Promise.all([
      api.getQBittorrent(),
      api.getLLM(),
      api.getNetwork(),
      api.downloadTasks(),
      api.organizeTasks(),
    ]);
    const attendanceResults = await attendanceRequest;
    health.value = healthData;
    sites.value = siteData;
    for (const site of siteData) {
      if (!siteCredentials.value[site.id]) {
        siteCredentials.value[site.id] = { user_agent: site.user_agent ?? '', cookie: '' };
      } else if (!siteCredentials.value[site.id].user_agent) {
        siteCredentials.value[site.id].user_agent = site.user_agent ?? '';
      }
    }
    const nextAttendances: Record<string, SiteAttendance> = {};
    for (const [index, site] of siteData.entries()) {
      const result = attendanceResults[index];
      const attendance =
        result?.status === 'fulfilled'
          ? result.value
          : {
              site_id: site.id,
              configured: false,
              enabled: false,
              time_of_day: '09:00',
              timezone: '',
            };
      nextAttendances[site.id] = {
        ...attendance,
        timezone: attendance.timezone || browserTimeZone(),
      };
    }
    siteAttendances.value = nextAttendances;
    mediaQuery.value = { ...mediaQuery.value, offset: torrentData.offset, limit: torrentData.limit };
    torrents.value = torrentData.items;
    torrentTotal.value = torrentData.total;
    fetchSettings.value = fetchSettingsData;
    fetchJobs.value = fetchJobData;
    qbConfig.value = { ...qbData, auth_mode: qbData.auth_mode ?? 'uid' };
    qbTagsText.value = qbData.tags?.join(', ') ?? '';
    llmConfig.value = llmData;
    networkConfig.value = networkData;
    downloadTasks.value = downloadData;
    organizeTasks.value = organizeData;
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Failed to load dashboard data';
  } finally {
    loading.value = false;
  }
}

/** 按媒体视图给出的范围和筛选条件读取数据库。 */
async function loadTorrentPage(query: TorrentPageQuery = mediaQuery.value) {
  mediaQuery.value = { ...query, sort_by: 'published_at', sort_direction: 'desc' };
  loading.value = true;
  try {
    const result = await api.torrents(mediaQuery.value);
    torrents.value = result.items;
    torrentTotal.value = result.total;
  } catch (error) {
    message.value = error instanceof Error ? error.message : '媒体列表加载失败';
  } finally {
    loading.value = false;
  }
}

const { merge: mergeFetchJobs, monitor: monitorFetchJob } = useSiteFetchJobs({
  jobs: fetchJobs,
  onCompleted: async (siteID) => {
    if (mediaQuery.value.site_id === siteID) await loadTorrentPage();
  },
});

async function startSiteFetch(siteID: string, request: SiteFetchRequest, trigger: 'manual' | 'homepage') {
  const job = await api.fetchSite(siteID, request, trigger);
  mergeFetchJobs([job]);
  monitorFetchJob(siteID, job.id);
  return job;
}

/** 响应媒体分页、筛选和单站点首页自动抓取。 */
async function handleMediaQueryChange(query: Omit<TorrentPageQuery, 'sort_by' | 'sort_direction'>) {
  await loadTorrentPage({ ...query, sort_by: 'published_at', sort_direction: 'desc' });
  const siteID = query.site_id;
  if (!siteID || autoFetchedSites.has(siteID)) return;
  const site = sites.value.find((item) => item.id === siteID);
  if (!site?.has_cookie) return;
  try {
    await startSiteFetch(siteID, { mode: 'incremental' }, 'homepage');
    autoFetchedSites.add(siteID);
  } catch (error) {
    message.value = error instanceof Error ? error.message : '首页自动抓取失败';
  }
}

/** 登录需要鉴权的本地服务。 */
async function login() {
  message.value = '';
  try {
    await api.login(username.value, password.value);
    loggedIn.value = true;
    await refresh();
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Login failed';
  }
}

/** 保存单个站点凭据。 */
async function saveSiteCredential(site: Site) {
  message.value = '';
  siteActions.value[site.id] = 'save';
  const draft = siteCredentials.value[site.id] ?? { user_agent: '', cookie: '' };
  try {
    const saved = await api.saveSiteCredential(site.id, {
      site_id: site.id,
      base_url: site.base_url,
      user_agent: draft.user_agent,
      cookie: draft.cookie,
      has_cookie: site.has_cookie,
    });
    siteCredentials.value[site.id] = { user_agent: saved.user_agent, cookie: '' };
    message.value = `Credential saved for ${site.name}`;
    await refresh(false);
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Failed to save site credential';
  } finally {
    siteActions.value[site.id] = '';
  }
}

/** 保存单个站点的自动签到配置。 */
async function saveSiteAttendance(siteID: string) {
  const attendance = siteAttendances.value[siteID];
  if (!attendance) return;
  message.value = '';
  siteActions.value[siteID] = 'attendance';
  try {
    siteAttendances.value[siteID] = await api.saveSiteAttendance(siteID, {
      enabled: attendance.enabled,
      time_of_day: attendance.time_of_day,
      timezone: attendance.timezone,
    });
    message.value = '自动签到设置已保存';
  } catch (error) {
    message.value = error instanceof Error ? error.message : '自动签到设置保存失败';
  } finally {
    siteActions.value[siteID] = '';
  }
}

/** 抓取单个站点并刷新数据库列表。 */
async function fetchSite(siteID: string, request: SiteFetchRequest) {
  message.value = '';
  siteActions.value[siteID] = 'fetch';
  try {
    const result = await startSiteFetch(siteID, request, 'manual');
    message.value = `扫描任务已提交：${result.id}`;
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Fetch failed';
  } finally {
    siteActions.value[siteID] = '';
  }
}

/** 保存全局站点扫描页数限制。 */
async function saveFetchSettings(settings: FetchSettings) {
  try {
    fetchSettings.value = await api.saveFetchSettings(settings);
    message.value = '抓取设置已保存';
  } catch (error) {
    message.value = error instanceof Error ? error.message : '抓取设置保存失败';
  }
}

/** 保存 qBittorrent 配置。 */
async function saveQBittorrent() {
  message.value = '';
  try {
    const payload = {
      ...qbConfig.value,
      tags: qbTagsText.value
        .split(',')
        .map((tag) => tag.trim())
        .filter(Boolean),
    };
    const saved = await api.saveQBittorrent(payload);
    qbConfig.value = saved;
    qbTagsText.value = saved.tags?.join(', ') ?? '';
    message.value = 'qBittorrent settings saved';
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Failed to save qBittorrent settings';
  }
}

/** 保存 LLM 配置。 */
async function saveLLM() {
  message.value = '';
  try {
    const saved = await api.saveLLM(llmConfig.value);
    llmConfig.value = saved;
    message.value = 'LLM settings saved';
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Failed to save LLM settings';
  }
}

/** 保存网络代理配置。 */
async function saveNetwork() {
  message.value = '';
  try {
    const saved = await api.saveNetwork(networkConfig.value);
    networkConfig.value = saved;
    message.value = '网络代理设置已保存';
  } catch (error) {
    message.value = error instanceof Error ? error.message : '保存网络代理设置失败';
  }
}

/** 打开种子标题整理和下载确认弹窗。 */
async function openDownloadDialog(torrent: Torrent) {
  downloadDialogOpen.value = true;
  downloadDialogLoading.value = true;
  downloadDialogSending.value = false;
  downloadDialogError.value = '';
  downloadPreview.value = null;
  try {
    downloadPreview.value = await api.previewTorrentDownload(torrent.site_id, torrent.id);
  } catch (error) {
    downloadDialogError.value = error instanceof Error ? error.message : 'Failed to preview download';
    downloadPreview.value = {
      site_id: torrent.site_id,
      torrent_id: torrent.id,
      original_title: torrent.title,
      formatted_title: torrent.title,
      download_url: torrent.download_url ?? '',
    };
  } finally {
    downloadDialogLoading.value = false;
  }
}

/** 在未发送时关闭下载确认弹窗。 */
function closeDownloadDialog() {
  if (downloadDialogSending.value) {
    return;
  }
  downloadDialogOpen.value = false;
  downloadPreview.value = null;
  downloadDialogError.value = '';
}

/** 发送当前确认的 torrent 到 qB。 */
async function sendDownload() {
  if (!downloadPreview.value) {
    return;
  }
  downloadDialogSending.value = true;
  downloadDialogError.value = '';
  try {
    const task = await api.sendTorrentDownload(
      downloadPreview.value.site_id,
      downloadPreview.value.torrent_id,
      downloadPreview.value.formatted_title,
    );
    message.value = `Download task ${task.status}: ${task.torrent_title}`;
    downloadDialogOpen.value = false;
    await refresh(false);
    triggerQBPoll(true);
  } catch (error) {
    downloadDialogError.value = error instanceof Error ? error.message : 'Failed to send download';
  } finally {
    downloadDialogSending.value = false;
  }
}

/** 显式同步 qB 状态并刷新媒体卡片快照。 */
async function syncQBittorrent() {
  message.value = '';
  qbSyncing.value = true;
  try {
    const result = await api.syncQB();
    message.value = `qB 同步：匹配 ${result.torrent_matched ?? 0}，更新 ${result.torrent_updated ?? 0}，移除 ${result.torrent_removed ?? 0}，完成 ${result.completed}`;
    await refresh(false);
    triggerQBPoll(true);
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'qB sync failed';
  } finally {
    qbSyncing.value = false;
  }
}

/** 控制单个 qB 任务暂停或恢复并刷新媒体快照。 */
async function controlTorrentQB(torrent: Torrent, action: 'start' | 'stop') {
  const key = `${torrent.site_id}:${torrent.id}`;
  qbActioning.value = key;
  message.value = '';
  try {
    const status = await api.controlTorrentQB(torrent.site_id, torrent.id, action);
    const completed = (status.progress ?? torrent.qb_status?.progress ?? 0) >= 1;
    status.state =
      action === 'stop' ? (completed ? 'stoppedUP' : 'stoppedDL') : completed ? 'uploading' : 'downloading';
    if (action === 'stop') {
      status.download_speed = 0;
      status.upload_speed = 0;
    }
    status.fetched_at = new Date().toISOString();
    message.value = action === 'stop' ? 'qB 任务已暂停' : 'qB 任务已恢复';
    torrent.qb_status = status;
    const optimisticUntil = Date.now() + qbOptimisticUpdateDelayMs;
    qbOptimisticUntil.set(key, optimisticUntil);
    window.setTimeout(() => {
      if (qbOptimisticUntil.get(key) !== optimisticUntil) return;
      qbOptimisticUntil.delete(key);
      triggerQBPoll();
    }, qbOptimisticUpdateDelayMs);
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'qB control failed';
  } finally {
    qbActioning.value = '';
  }
}

/** 使用浏览器或桌面系统默认浏览器打开 qBittorrent WebUI。 */
async function openQBittorrent() {
  let target = qbConfig.value.url.trim();
  if (!target) {
    message.value = '请先配置 qBittorrent WebUI URL';
    return;
  }
  if (!target.includes('://')) {
    target = `http://${target}`;
  }
  try {
    await openExternalURL(target);
  } catch (error) {
    message.value = error instanceof Error ? error.message : '无法打开 qBittorrent WebUI';
  }
}

/** 处理数据库中的待整理任务。 */
async function organizePending() {
  message.value = '';
  try {
    const result = await api.organizePending();
    message.value = `Organize processed ${result.processed}, failed ${result.failed}, dry run ${result.dry_run}`;
    await refresh();
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Organize failed';
  }
}

onMounted(async () => {
  try {
    session.value = await api.session();
    loggedIn.value = !session.value.requires_login;
    if (loggedIn.value) {
      await refresh();
    }
  } catch (error) {
    loading.value = false;
    message.value = error instanceof Error ? error.message : 'Failed to connect to service';
  }
});
</script>

<template>
  <NConfigProvider :theme-overrides="themeOverrides">
    <NGlobalStyle />
    <NLayout class="app-shell" has-sider>
      <NLayoutSider class="desktop-sider" bordered :width="248">
        <div class="brand-block sider-brand">
          <NIcon :component="Database" size="28" class="brand-icon" />
          <div>
            <h1>NexusBridge</h1>
            <p>Local PT bridge</p>
          </div>
        </div>

        <nav class="side-nav">
          <RouterLink v-for="item in navItems" :key="item.key" v-slot="{ href, navigate }" :to="item.to" custom>
            <NButton
              tag="a"
              :href="href"
              :type="activePage === item.key ? 'primary' : 'default'"
              :secondary="activePage !== item.key"
              block
              @click="navigate"
            >
              <template #icon>
                <NIcon :component="item.icon" />
              </template>
              {{ item.label }}
            </NButton>
          </RouterLink>
        </nav>

        <div class="side-section">
          <p>设置</p>
          <RouterLink v-for="item in settingsItems" :key="item.key" v-slot="{ href, navigate }" :to="item.to" custom>
            <NButton
              tag="a"
              :href="href"
              :type="activePage === 'settings' && activeSettingsPage === item.key ? 'primary' : 'default'"
              :secondary="activePage !== 'settings' || activeSettingsPage !== item.key"
              block
              @click="navigate"
            >
              <template #icon>
                <NIcon :component="item.icon" />
              </template>
              {{ item.label }}
            </NButton>
          </RouterLink>
        </div>
      </NLayoutSider>

      <NLayout>
        <NLayoutHeader class="app-header" bordered>
          <div class="brand-block mobile-brand">
            <NIcon :component="Database" size="26" class="brand-icon" />
            <div>
              <h1>NexusBridge</h1>
              <p>{{ currentTitle }}</p>
            </div>
          </div>
          <div class="page-title">
            <strong>{{ currentTitle }}</strong>
            <span>{{ health?.addr ?? '0.0.0.0:8090' }}</span>
          </div>
          <NButton v-if="!showLogin" type="primary" :loading="loading" @click="() => refresh()">
            <template #icon>
              <NIcon :component="RefreshCw" />
            </template>
            Refresh
          </NButton>
        </NLayoutHeader>

        <NLayoutContent class="app-content">
          <NCard v-if="showLogin" class="login-card" title="Sign in" :bordered="false">
            <NForm @submit.prevent="login">
              <NFormItem label="Username">
                <NInput v-model:value="username" autocomplete="username" />
              </NFormItem>
              <NFormItem label="Password">
                <NInput
                  v-model:value="password"
                  type="password"
                  show-password-on="click"
                  autocomplete="current-password"
                />
              </NFormItem>
              <NButton type="primary" attr-type="submit" block>
                <template #icon>
                  <NIcon :component="ShieldCheck" />
                </template>
                Sign in
              </NButton>
            </NForm>
          </NCard>

          <template v-else>
            <RouterView v-slot="{ Component }">
              <section v-if="activePage === 'settings'" class="settings-page">
                <nav class="settings-tabs">
                  <RouterLink
                    v-for="item in settingsItems"
                    :key="item.key"
                    v-slot="{ href, navigate }"
                    :to="item.to"
                    custom
                  >
                    <NButton
                      tag="a"
                      :href="href"
                      :type="activeSettingsPage === item.key ? 'primary' : 'default'"
                      :secondary="activeSettingsPage !== item.key"
                      @click="navigate"
                    >
                      <template #icon>
                        <NIcon :component="item.icon" />
                      </template>
                      {{ item.label }}
                    </NButton>
                  </RouterLink>
                </nav>

                <component :is="Component" v-bind="currentViewProps" v-on="currentViewListeners" />
              </section>
              <component :is="Component" v-else v-bind="currentViewProps" v-on="currentViewListeners" />
            </RouterView>
          </template>
        </NLayoutContent>
      </NLayout>
    </NLayout>

    <nav v-if="!showLogin" class="mobile-bottom-nav">
      <RouterLink v-for="item in navItems" :key="item.key" :to="item.to" :class="{ active: activePage === item.key }">
        <NIcon :component="item.icon" />
        <span>{{ item.label }}</span>
      </RouterLink>
    </nav>

    <NModal
      v-model:show="downloadDialogOpen"
      preset="card"
      title="Download"
      class="download-modal"
      :mask-closable="!downloadDialogSending"
      :closable="!downloadDialogSending"
      @close="closeDownloadDialog"
    >
      <NAlert v-if="downloadDialogLoading" type="info" :bordered="false"> Extracting title with LLM... </NAlert>
      <NAlert v-if="downloadDialogError" type="warning" :bordered="false" class="dialog-alert">
        {{ downloadDialogError }}
      </NAlert>
      <NForm v-if="downloadPreview" class="dialog-form">
        <NFormItem label="Original Title">
          <NInput :value="downloadPreview.original_title" type="textarea" readonly :autosize="{ minRows: 3 }" />
        </NFormItem>
        <NFormItem label="LLM Result">
          <NInput v-model:value="downloadPreview.formatted_title" type="textarea" :autosize="{ minRows: 3 }" />
        </NFormItem>
        <NFormItem label="Download URL">
          <NInput :value="downloadPreview.download_url" readonly />
        </NFormItem>
      </NForm>

      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="downloadDialogSending" @click="closeDownloadDialog">Cancel</NButton>
          <NButton
            type="primary"
            :loading="downloadDialogSending"
            :disabled="downloadDialogLoading || !downloadPreview?.download_url"
            @click="sendDownload"
          >
            Send to qBittorrent
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </NConfigProvider>
</template>
