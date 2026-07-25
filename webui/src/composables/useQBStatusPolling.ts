/** qB 状态轮询模块根据连接状态和页面可见性调度增量同步。 */

import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue';
import { api } from '../api';
import type { QBPollResult, QBittorrentConfig } from '../types';

interface QBStatusPollingOptions {
  config: Ref<QBittorrentConfig>;
  isForeground: () => boolean;
  applyResult: (result: QBPollResult) => void;
}

/** 创建可暂停、可立即触发并自动退避的 qB 增量轮询器。 */
export function useQBStatusPolling(options: QBStatusPollingOptions) {
  const connected = ref<boolean | null>(null);
  const polling = ref(false);
  let rid = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let pendingImmediate = false;
  let lastURL = '';

  /** 判断当前配置是否允许发起自动同步。 */
  function canPoll() {
    return options.config.value.auto_sync && Boolean(options.config.value.url.trim());
  }

  /** 返回当前连接和页面状态对应的下一次轮询延迟。 */
  function nextDelay() {
    const config = options.config.value;
    if (connected.value === false) return config.disconnected_sync_interval_seconds * 1000;
    if (document.hidden || !options.isForeground()) return config.inactive_sync_interval_seconds * 1000;
    return config.sync_interval_seconds * 1000;
  }

  /** 清理已有定时器。 */
  function clearTimer() {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
  }

  /** 安排下一次增量同步。 */
  function schedule() {
    clearTimer();
    if (!canPoll()) return;
    timer = setTimeout(() => void poll(), nextDelay());
  }

  /** 调用后端 qB 增量接口并应用匹配到的媒体状态。 */
  async function poll() {
    clearTimer();
    if (!canPoll()) return;
    if (polling.value) {
      pendingImmediate = true;
      return;
    }
    polling.value = true;
    try {
      const result = await api.pollQB(rid);
      rid = result.rid;
      connected.value = result.connected;
      options.applyResult(result);
    } catch {
      connected.value = false;
      rid = 0;
    } finally {
      polling.value = false;
      if (pendingImmediate) {
        pendingImmediate = false;
        void poll();
      } else {
        schedule();
      }
    }
  }

  /** 立即请求一次增量状态，可选择从完整响应重新开始。 */
  function trigger(reset = false) {
    if (reset) rid = 0;
    void poll();
  }

  /** 配置、页面或可见性变化后重新安排轮询。 */
  function restart() {
    const currentURL = options.config.value.url.trim();
    if (currentURL !== lastURL) {
      rid = 0;
      connected.value = null;
      lastURL = currentURL;
    }
    if (!canPoll()) {
      clearTimer();
      connected.value = options.config.value.url.trim() ? null : false;
      return;
    }
    schedule();
  }

  watch(
    () => [
      options.config.value.url,
      options.config.value.auto_sync,
      options.config.value.sync_interval_seconds,
      options.config.value.inactive_sync_interval_seconds,
      options.config.value.disconnected_sync_interval_seconds,
      options.isForeground(),
    ],
    restart,
  );

  onMounted(() => {
    document.addEventListener('visibilitychange', restart);
    restart();
    if (canPoll()) trigger(true);
  });
  onBeforeUnmount(() => {
    clearTimer();
    document.removeEventListener('visibilitychange', restart);
  });

  return { connected, polling, trigger };
}
