<!-- 应用壳负责页面导航、共享状态和跨组件操作协调。 -->
<script setup lang="ts">
import { Bot, CloudDownload, Database, FolderKanban, KeyRound, RefreshCw, Settings, ShieldCheck } from '@lucide/vue';
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
import { api } from './api';
import { useQBStatusPolling } from './composables/useQBStatusPolling';
import { createDefaultQBittorrentConfig } from './config/qbittorrent';
import MediaView from './components/MediaView.vue';
import SettingsLLM from './components/SettingsLLM.vue';
import SettingsQB from './components/SettingsQB.vue';
import SettingsSites from './components/SettingsSites.vue';
import TasksView from './components/TasksView.vue';
import { openExternalURL } from './utils/runtime';
import type {
  DownloadPreview,
  DownloadTask,
  Health,
  LLMConfig,
  OrganizeTask,
  QBittorrentConfig,
  Session,
  Site,
  Torrent,
	QBPollResult,
} from './types';

type PageKey = 'media' | 'tasks' | 'settings';
type SettingsPageKey = 'sites' | 'llm' | 'qbittorrent';
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

const navItems: Array<{ key: PageKey; label: string; icon: typeof CloudDownload }> = [
  { key: 'media', label: '媒体', icon: CloudDownload },
  { key: 'tasks', label: '任务', icon: FolderKanban },
  { key: 'settings', label: '设置', icon: Settings },
];

const settingsItems: Array<{ key: SettingsPageKey; label: string; icon: typeof KeyRound }> = [
  { key: 'sites', label: '站点', icon: KeyRound },
  { key: 'llm', label: 'LLM', icon: Bot },
  { key: 'qbittorrent', label: 'qBittorrent', icon: Database },
];

const session = ref<Session | null>(null);
const loggedIn = ref(false);
const activePage = ref<PageKey>('media');
const activeSettingsPage = ref<SettingsPageKey>('sites');
const health = ref<Health | null>(null);
const sites = ref<Site[]>([]);
const torrents = ref<Torrent[]>([]);
const downloadTasks = ref<DownloadTask[]>([]);
const organizeTasks = ref<OrganizeTask[]>([]);
const siteCredentials = ref<Record<string, SiteCredentialDraft>>({});
const siteActions = ref<Record<string, 'fetch' | 'run' | 'save' | ''>>({});
const qbConfig = ref<QBittorrentConfig>(createDefaultQBittorrentConfig());
const llmConfig = ref<LLMConfig>({
  base_url: '',
  api_key: '',
  model: '',
});
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

const showLogin = computed(() => session.value?.requires_login && !loggedIn.value);
const activeDownloads = computed(() => downloadTasks.value.filter((task) => task.status !== 'completed').length);
const pendingOrganize = computed(() => organizeTasks.value.filter((task) => task.status !== 'completed').length);
const currentTitle = computed(() => {
  if (activePage.value === 'media') {
    return '媒体库';
  }
  if (activePage.value === 'tasks') {
    return '任务';
  }
  const item = settingsItems.find((option) => option.key === activeSettingsPage.value);
  return `设置 / ${item?.label ?? '站点'}`;
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

/** 切换主导航页面。 */
function selectPage(page: PageKey) {
  activePage.value = page;
}

/** 切换设置子页面。 */
function selectSettingsPage(page: SettingsPageKey) {
  activePage.value = 'settings';
  activeSettingsPage.value = page;
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

/** 刷新后端健康状态、设置和数据库快照。 */
async function refresh(clearMessage = true) {
  loading.value = true;
  if (clearMessage) {
    message.value = '';
  }
  try {
    const [healthData, siteData, torrentData] = await Promise.all([
      api.health(),
      api.sites(),
      api.torrents(),
    ]);
    const [qbData, llmData, downloadData, organizeData] = await Promise.all([
      api.getQBittorrent(),
      api.getLLM(),
      api.downloadTasks(),
      api.organizeTasks(),
    ]);
    health.value = healthData;
    sites.value = siteData;
    for (const site of siteData) {
      if (!siteCredentials.value[site.id]) {
        siteCredentials.value[site.id] = { user_agent: site.user_agent ?? '', cookie: '' };
      } else if (!siteCredentials.value[site.id].user_agent) {
        siteCredentials.value[site.id].user_agent = site.user_agent ?? '';
      }
    }
    torrents.value = torrentData;
    qbConfig.value = { ...qbData, auth_mode: qbData.auth_mode ?? 'uid' };
    qbTagsText.value = qbData.tags?.join(', ') ?? '';
    llmConfig.value = llmData;
    downloadTasks.value = downloadData;
    organizeTasks.value = organizeData;
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Failed to load dashboard data';
  } finally {
    loading.value = false;
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

/** 抓取单个站点并刷新数据库列表。 */
async function fetchSite(siteID: string) {
  message.value = '';
  siteActions.value[siteID] = 'fetch';
  try {
    const result = await api.fetchSite(siteID);
    message.value = `Fetch ${result.status}: fetched ${result.fetched}, changed ${result.changed}`;
    await refresh(false);
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Fetch failed';
  } finally {
    siteActions.value[siteID] = '';
  }
}

/** 执行站点抓取、匹配和发送闭环。 */
async function runOnce(siteID: string) {
  message.value = '';
  siteActions.value[siteID] = 'run';
  try {
    const result = await api.runOnce(siteID);
    message.value = `Run once ${result.status}: matched ${result.matched ?? 0}, sent ${result.download_sent ?? 0}`;
    await refresh(false);
  } catch (error) {
    message.value = error instanceof Error ? error.message : 'Run once failed';
  } finally {
    siteActions.value[siteID] = '';
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
		status.state = action === 'stop' ? (completed ? 'stoppedUP' : 'stoppedDL') : (completed ? 'uploading' : 'downloading');
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
            <NButton
              v-for="item in navItems"
              :key="item.key"
              :type="activePage === item.key ? 'primary' : 'default'"
              :secondary="activePage !== item.key"
              block
              @click="selectPage(item.key)"
            >
              <template #icon>
                <NIcon :component="item.icon" />
              </template>
              {{ item.label }}
            </NButton>
          </nav>

          <div class="side-section">
            <p>设置</p>
            <NButton
              v-for="item in settingsItems"
              :key="item.key"
              :type="activePage === 'settings' && activeSettingsPage === item.key ? 'primary' : 'default'"
              :secondary="activePage !== 'settings' || activeSettingsPage !== item.key"
              block
              @click="selectSettingsPage(item.key)"
            >
              <template #icon>
                <NIcon :component="item.icon" />
              </template>
              {{ item.label }}
            </NButton>
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
              <span>{{ health?.addr ?? '127.0.0.1:8090' }}</span>
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
              <MediaView
                v-if="activePage === 'media'"
                :sites="sites"
                :torrents="torrents"
                :loading="loading"
				:qb-url="qbConfig.url"
				:qb-syncing="qbSyncing"
				:qb-actioning="qbActioning"
                @download="openDownloadDialog"
				@sync-qb="syncQBittorrent"
				@open-qb="openQBittorrent"
				@control-qb="controlTorrentQB"
              />
              <TasksView
                v-else-if="activePage === 'tasks'"
                :download-tasks="downloadTasks"
                :organize-tasks="organizeTasks"
                :active-downloads="activeDownloads"
                :pending-organize="pendingOrganize"
                @organize="organizePending"
              />
              <section v-else class="settings-page">
                <nav class="settings-tabs">
                  <NButton
                    v-for="item in settingsItems"
                    :key="item.key"
                    :type="activeSettingsPage === item.key ? 'primary' : 'default'"
                    :secondary="activeSettingsPage !== item.key"
                    @click="selectSettingsPage(item.key)"
                  >
                    <template #icon>
                      <NIcon :component="item.icon" />
                    </template>
                    {{ item.label }}
                  </NButton>
                </nav>

                <SettingsSites
                  v-if="activeSettingsPage === 'sites'"
                  :sites="sites"
                  :credentials="siteCredentials"
                  :actions="siteActions"
                  @update-credential="updateSiteCredential"
                  @save="saveSiteCredential"
                  @fetch="fetchSite"
                  @run="runOnce"
                />
                <SettingsLLM
                  v-else-if="activeSettingsPage === 'llm'"
                  :config="llmConfig"
                  @update="updateLLMConfig"
                  @save="saveLLM"
                />
                <SettingsQB
                  v-else
                  :config="qbConfig"
                  :tags-text="qbTagsText"
				  :connected="qbConnected"
				  :polling="qbPolling"
                  @update="updateQBConfig"
                  @update-tags="(value) => (qbTagsText = value)"
                  @save="saveQBittorrent"
                  @sync="syncQBittorrent"
                />
              </section>
            </template>
          </NLayoutContent>
        </NLayout>
      </NLayout>

      <nav v-if="!showLogin" class="mobile-bottom-nav">
        <button
          v-for="item in navItems"
          :key="item.key"
          type="button"
          :class="{ active: activePage === item.key }"
          @click="selectPage(item.key)"
        >
          <NIcon :component="item.icon" />
          <span>{{ item.label }}</span>
        </button>
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
        <NAlert v-if="downloadDialogLoading" type="info" :bordered="false">
          Extracting title with LLM...
        </NAlert>
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
