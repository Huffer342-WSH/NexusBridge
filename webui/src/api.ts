/** 封装 WebUI 调用 NexusBridge HTTP API 的统一入口。 */

import type {
  BatchDownloadPreview,
  BatchDownloadRequest,
  BatchDownloadResult,
  DeletedResult,
  DownloadTask,
  DownloadPreview,
  FilterRule,
  FileBrowseResult,
  FetchSettings,
  Health,
  LLMConfig,
  NetworkConfig,
  MihomoProviderCreateRequest,
  MihomoSettings,
  OrganizeTask,
  PlaybackTorrent,
  QBittorrentConfig,
  QBSyncResult,
  QBPollResult,
  QBCategoriesResult,
  QBTagsResult,
  QBTorrentStatus,
  RecoveryPreview,
  RecoveryPreviewRequest,
  TorrentSizeIndexStatus,
  RecoveryRequest,
  RecoveryActionResult,
  RecoveryBatchRequest,
  RecoveryBatchResult,
  RecoveryResult,
  RecoveryScanRequest,
  RecoveryScanResult,
  RulePreviewRequest,
  RulePreviewResult,
  RuleFilterOptions,
  Session,
  Site,
  SiteAttendance,
  SiteAttendanceInput,
  SiteCredential,
  SiteSchedule,
  SiteScheduleInput,
  Subscription,
  SubscriptionCandidate,
  SubscriptionPreview,
  SubscriptionRun,
  Torrent,
  TorrentPlayback,
  TorrentPage,
  TorrentPageQuery,
  SiteFetchJob,
  SiteFetchRequest,
} from './types';

/** 发送 API 请求并统一解析错误响应。 */
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {}),
    },
    ...init,
  });
  if (!response.ok) {
    let detail = `${response.status} ${response.statusText}`;
    try {
      const body = (await response.clone().json()) as { error?: string };
      if (body.error) {
        detail = body.error;
      }
    } catch {
      const text = await response.text();
      if (text.trim()) {
        detail = text.trim();
      }
    }
    throw new Error(detail);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return response.json() as Promise<T>;
}

export const api = {
  session: () => request<Session>('/api/session'),
  login: (username: string, password: string) =>
    request<{ token: string }>('/api/session/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  health: () => request<Health>('/api/health'),
  sites: () => request<Site[]>('/api/sites'),
  getSiteCredential: (siteID: string) => request<SiteCredential>(`/api/sites/${siteID}/credential`),
  saveSiteCredential: (siteID: string, credential: SiteCredential) =>
    request<SiteCredential>(`/api/sites/${siteID}/credential`, {
      method: 'POST',
      body: JSON.stringify(credential),
    }),
  getSiteAttendance: (siteID: string) => request<SiteAttendance>(`/api/sites/${encodeURIComponent(siteID)}/attendance`),
  saveSiteAttendance: (siteID: string, attendance: SiteAttendanceInput) =>
    request<SiteAttendance>(`/api/sites/${encodeURIComponent(siteID)}/attendance`, {
      method: 'POST',
      body: JSON.stringify(attendance),
    }),
  torrents: (query: TorrentPageQuery) => {
    const params = new URLSearchParams({
      offset: String(query.offset),
      limit: String(query.limit),
      include_pinned: String(query.include_pinned),
      sort_by: query.sort_by ?? 'published_at',
      sort_direction: query.sort_direction ?? 'desc',
    });
    if (query.site_id) params.set('site_id', query.site_id);
    if (query.q?.trim()) params.set('q', query.q.trim());
    return request<TorrentPage>(`/api/torrents?${params.toString()}`);
  },
  getTorrentPlayback: (siteID: string, torrentID: string) =>
    request<TorrentPlayback>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/playback`),
  getPlaybackTorrents: (excludeSiteID: string, excludeTorrentID: string, limit = 20) => {
    const params = new URLSearchParams({
      exclude_site_id: excludeSiteID,
      exclude_torrent_id: excludeTorrentID,
      limit: String(limit),
    });
    return request<PlaybackTorrent[]>(`/api/playback/torrents?${params.toString()}`);
  },
  fetchSite: (
    siteID: string,
    payload: SiteFetchRequest = { mode: 'incremental' },
    trigger: 'manual' | 'homepage' = 'manual',
  ) =>
    request<SiteFetchJob>(`/api/sites/${encodeURIComponent(siteID)}/fetch?trigger=${trigger}`, {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  getSiteFetchJobs: (siteID?: string, limit = 100) => {
    const params = new URLSearchParams({ limit: String(limit) });
    if (siteID) params.set('site_id', siteID);
    return request<SiteFetchJob[]>(`/api/site-fetch-jobs?${params.toString()}`);
  },
  getFetchSettings: () => request<FetchSettings>('/api/settings/fetch'),
  saveFetchSettings: (settings: FetchSettings) =>
    request<FetchSettings>('/api/settings/fetch', { method: 'POST', body: JSON.stringify(settings) }),
  getRules: () => request<FilterRule[]>('/api/rules'),
  getRuleFilterOptions: (siteID: string) =>
    request<RuleFilterOptions>(`/api/sites/${encodeURIComponent(siteID)}/filter-options`),
  saveRule: (rule: FilterRule) =>
    request<FilterRule>('/api/rules', {
      method: 'POST',
      body: JSON.stringify(rule),
    }),
  updateRule: (originalName: string, rule: FilterRule) =>
    request<FilterRule>(`/api/rules/${encodeURIComponent(originalName)}`, {
      method: 'PUT',
      body: JSON.stringify(rule),
    }),
  deleteRule: (ruleName: string) =>
    request<DeletedResult>(`/api/rules/${encodeURIComponent(ruleName)}`, { method: 'DELETE' }),
  previewRule: (payload: RulePreviewRequest) =>
    request<RulePreviewResult>('/api/rules/preview', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  getSubscriptions: () => request<Subscription[]>('/api/subscriptions'),
  saveSubscription: (subscription: Subscription) =>
    request<Subscription>('/api/subscriptions', {
      method: 'POST',
      body: JSON.stringify(subscription),
    }),
  deleteSubscription: (subscriptionID: string) =>
    request<DeletedResult>(`/api/subscriptions/${encodeURIComponent(subscriptionID)}`, { method: 'DELETE' }),
  previewSubscription: (subscriptionID: string, limit?: number) =>
    request<SubscriptionPreview>(
      `/api/subscriptions/${encodeURIComponent(subscriptionID)}/preview${limit ? `?limit=${limit}` : ''}`,
      {
        method: 'POST',
      },
    ),
  getSiteSchedule: (siteID: string) => request<SiteSchedule>(`/api/sites/${encodeURIComponent(siteID)}/schedule`),
  saveSiteSchedule: (siteID: string, schedule: SiteScheduleInput) =>
    request<SiteSchedule>(`/api/sites/${encodeURIComponent(siteID)}/schedule`, {
      method: 'POST',
      body: JSON.stringify(schedule),
    }),
  getQBCategories: (refresh = false) =>
    request<QBCategoriesResult>(`/api/qb/categories${refresh ? '?refresh=true' : ''}`),
  createQBCategory: (name: string, savePath: string) =>
    request<QBCategoriesResult>('/api/qb/categories', {
      method: 'POST',
      body: JSON.stringify({ name, save_path: savePath }),
    }),
  getQBTags: (refresh = false) => request<QBTagsResult>(`/api/qb/tags${refresh ? '?refresh=true' : ''}`),
  createQBTags: (tags: string[]) =>
    request<QBTagsResult>('/api/qb/tags', {
      method: 'POST',
      body: JSON.stringify({ tags }),
    }),
  browseFiles: (path = '') =>
    request<FileBrowseResult>('/api/files/browse', {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),
  previewRecovery: (payload: RecoveryPreviewRequest) =>
    request<RecoveryPreview>('/api/qb/recovery/preview', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  getTorrentSizeIndexStatus: () => request<TorrentSizeIndexStatus>('/api/qb/recovery/index'),
  rebuildTorrentSizeIndex: () => request<TorrentSizeIndexStatus>('/api/qb/recovery/index/rebuild', { method: 'POST' }),
  recoverFolder: (payload: RecoveryRequest) =>
    request<RecoveryResult>('/api/qb/recovery', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  controlRecoveryTorrent: (hash: string, action: 'start' | 'delete') =>
    request<RecoveryActionResult>(`/api/qb/recovery/${encodeURIComponent(hash)}/action`, {
      method: 'POST',
      body: JSON.stringify({ action }),
    }),
  scanRecoveryCandidates: (payload: RecoveryScanRequest) =>
    request<RecoveryScanResult>('/api/qb/recovery/scan', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  recoverFolders: (payload: RecoveryBatchRequest) =>
    request<RecoveryBatchResult>('/api/qb/recovery/batch', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  getSubscriptionCandidates: (subscriptionID: string, status?: string) =>
    request<SubscriptionCandidate[]>(
      `/api/subscriptions/${encodeURIComponent(subscriptionID)}/candidates${status ? `?status=${encodeURIComponent(status)}` : ''}`,
    ),
  getSubscriptionRuns: (subscriptionID?: string) =>
    request<SubscriptionRun[]>(
      `/api/subscription-runs${subscriptionID ? `?subscription_id=${encodeURIComponent(subscriptionID)}` : ''}`,
    ),
  previewBatchDownloads: (payload: BatchDownloadRequest) =>
    request<BatchDownloadPreview>('/api/downloads/batch/preview', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  executeBatchDownloads: (payload: BatchDownloadRequest) =>
    request<BatchDownloadResult>('/api/downloads/batch', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  retryDownloadTask: (taskID: string) =>
    request<DownloadTask>(`/api/download-tasks/${encodeURIComponent(taskID)}/retry`, { method: 'POST' }),
  getQBittorrent: () => request<QBittorrentConfig>('/api/settings/qbittorrent'),
  saveQBittorrent: (config: QBittorrentConfig) =>
    request<QBittorrentConfig>('/api/settings/qbittorrent', {
      method: 'POST',
      body: JSON.stringify(config),
    }),
  getLLM: () => request<LLMConfig>('/api/settings/llm'),
  saveLLM: (config: LLMConfig) =>
    request<LLMConfig>('/api/settings/llm', {
      method: 'POST',
      body: JSON.stringify(config),
    }),
  getNetwork: () => request<NetworkConfig>('/api/settings/network'),
  saveNetwork: (config: NetworkConfig) =>
    request<NetworkConfig>('/api/settings/network', {
      method: 'POST',
      body: JSON.stringify(config),
    }),
  getMihomo: (configDir?: string) =>
    request<MihomoSettings>(`/api/settings/mihomo${configDir ? `?config_dir=${encodeURIComponent(configDir)}` : ''}`),
  saveMihomoDirectory: (configDir: string) =>
    request<MihomoSettings>('/api/settings/mihomo/directory', {
      method: 'POST',
      body: JSON.stringify({ config_dir: configDir }),
    }),
  addMihomoProvider: (provider: MihomoProviderCreateRequest) =>
    request<MihomoSettings>('/api/settings/mihomo/providers', {
      method: 'POST',
      body: JSON.stringify(provider),
    }),
  previewTorrentDownload: (siteID: string, torrentID: string) =>
    request<DownloadPreview>(
      `/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/download/preview`,
      {
        method: 'POST',
      },
    ),
  sendTorrentDownload: (siteID: string, torrentID: string, formattedTitle: string) =>
    request<DownloadTask>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/download`, {
      method: 'POST',
      body: JSON.stringify({ formatted_title: formattedTitle }),
    }),
  torrentQBStatus: (siteID: string, torrentID: string) =>
    request<QBTorrentStatus>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/qb-status`),
  controlTorrentQB: (siteID: string, torrentID: string, action: 'start' | 'stop') =>
    request<QBTorrentStatus>(
      `/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/qb-control`,
      {
        method: 'POST',
        body: JSON.stringify({ action }),
      },
    ),
  syncQB: () => request<QBSyncResult>('/api/qb/sync', { method: 'POST' }),
  pollQB: (rid: number) => request<QBPollResult>(`/api/qb/poll?rid=${Math.max(0, Math.trunc(rid))}`),
  downloadTasks: () => request<DownloadTask[]>('/api/download-tasks'),
  organizeTasks: () => request<OrganizeTask[]>('/api/organize-tasks'),
  organizePending: () =>
    request<{ processed: number; failed: number; dry_run: boolean }>('/api/organize/pending', { method: 'POST' }),
};
