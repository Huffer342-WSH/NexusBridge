/** 封装 WebUI 调用 NexusBridge HTTP API 的统一入口。 */

import type {
  DownloadTask,
  DownloadPreview,
  FetchResult,
  Health,
  LLMConfig,
  OrganizeTask,
  QBittorrentConfig,
	QBSyncResult,
	QBPollResult,
	QBTorrentStatus,
  Session,
  Site,
  SiteCredential,
  Torrent,
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
  torrents: () => request<Torrent[]>('/api/torrents'),
  fetchSite: (siteID: string) =>
    request<FetchResult>(`/api/sites/${siteID}/fetch`, {
      method: 'POST',
    }),
  runOnce: (siteID: string) =>
    request<FetchResult>(`/api/sites/${siteID}/run-once`, {
      method: 'POST',
    }),
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
  previewTorrentDownload: (siteID: string, torrentID: string) =>
    request<DownloadPreview>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/download/preview`, {
      method: 'POST',
    }),
  sendTorrentDownload: (siteID: string, torrentID: string, formattedTitle: string) =>
    request<DownloadTask>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/download`, {
      method: 'POST',
      body: JSON.stringify({ formatted_title: formattedTitle }),
    }),
	torrentQBStatus: (siteID: string, torrentID: string) =>
		request<QBTorrentStatus>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/qb-status`),
	controlTorrentQB: (siteID: string, torrentID: string, action: 'start' | 'stop') =>
		request<QBTorrentStatus>(`/api/torrents/${encodeURIComponent(siteID)}/${encodeURIComponent(torrentID)}/qb-control`, {
			method: 'POST',
			body: JSON.stringify({ action }),
		}),
	syncQB: () => request<QBSyncResult>('/api/qb/sync', { method: 'POST' }),
	pollQB: (rid: number) => request<QBPollResult>(`/api/qb/poll?rid=${Math.max(0, Math.trunc(rid))}`),
  downloadTasks: () => request<DownloadTask[]>('/api/download-tasks'),
  organizeTasks: () => request<OrganizeTask[]>('/api/organize-tasks'),
  organizePending: () =>
    request<{ processed: number; failed: number; dry_run: boolean }>('/api/organize/pending', { method: 'POST' }),
};
