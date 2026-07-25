/** qBittorrent 前端配置模块集中维护默认值和输入约束。 */

import type { QBittorrentConfig } from '../types';

export const QB_POLLING_LIMITS = {
  syncIntervalSeconds: { min: 2, max: 60, default: 3 },
  inactiveSyncIntervalSeconds: { min: 10, max: 300, default: 30 },
  disconnectedSyncIntervalSeconds: { min: 15, max: 600, default: 60 },
} as const;

/** 创建可安全修改的 qBittorrent 默认配置。 */
export function createDefaultQBittorrentConfig(): QBittorrentConfig {
  return {
    auth_mode: 'uid',
    url: '',
    api_key: '',
    username: '',
    user_id: '',
    password: '',
    category: '',
    tags: [],
    auto_sync: true,
    sync_interval_seconds: QB_POLLING_LIMITS.syncIntervalSeconds.default,
    inactive_sync_interval_seconds: QB_POLLING_LIMITS.inactiveSyncIntervalSeconds.default,
    disconnected_sync_interval_seconds: QB_POLLING_LIMITS.disconnectedSyncIntervalSeconds.default,
  };
}
