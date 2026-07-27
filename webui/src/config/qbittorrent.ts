/** qBittorrent 前端配置模块集中维护默认值和输入约束。 */

import type { QBittorrentConfig } from '../types';

export const QB_POLLING_LIMITS = {
  syncIntervalSeconds: { min: 2, max: 60, default: 3 },
  inactiveSyncIntervalSeconds: { min: 10, max: 300, default: 30 },
  disconnectedSyncIntervalSeconds: { min: 15, max: 600, default: 60 },
} as const;

/** 判断 qB WebUI 主机是否只指向 NexusBridge 所在机器。 */
function isLocalQBHostname(hostname: string): boolean {
  const normalized = hostname.toLowerCase().replace(/^\[(.*)\]$/, '$1');
  return (
    normalized === 'localhost' ||
    normalized === '::' ||
    normalized === '::1' ||
    normalized === '0.0.0.0' ||
    normalized.startsWith('127.')
  );
}

/**
 * 解析浏览器应打开的 qB WebUI 地址。
 * qB 配置指向本机时使用当前 NexusBridge WebUI 主机，同时保留 qB 的协议、端口和路径。
 */
export function resolveQBWebUIURL(configuredURL: string, nexusHostname = ''): string {
  const value = configuredURL.trim();
  if (!value) return '';

  const target = new URL(value.includes('://') ? value : `http://${value}`);
  if (nexusHostname && isLocalQBHostname(target.hostname)) {
    target.hostname = nexusHostname;
  }
  return target.toString();
}

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
