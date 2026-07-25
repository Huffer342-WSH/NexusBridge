/** 站点抓取任务轮询模块负责合并进度、有限退避和生命周期清理。 */

import { onBeforeUnmount, type Ref } from 'vue';
import { api } from '../api';
import type { SiteFetchJob } from '../types';

interface SiteFetchJobPollingOptions {
  jobs: Ref<SiteFetchJob[]>;
  onCompleted: (siteID: string) => Promise<void> | void;
  maxRecords?: number;
}

/** 创建站点抓取任务轮询器。 */
export function useSiteFetchJobs(options: SiteFetchJobPollingOptions) {
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const failures = new Map<string, number>();
  const maxRecords = options.maxRecords ?? 100;

  /** 合并任务快照并限制内存中的历史记录数量。 */
  function merge(items: SiteFetchJob[]) {
    const byID = new Map(options.jobs.value.map((job) => [job.id, job]));
    for (const job of items) byID.set(job.id, job);
    options.jobs.value = Array.from(byID.values())
      .sort((left, right) => right.created_at.localeCompare(left.created_at))
      .slice(0, maxRecords);
  }

  function clearTimer(jobID: string) {
    const timer = timers.get(jobID);
    if (timer !== undefined) clearTimeout(timer);
    timers.delete(jobID);
  }

  function clear(jobID: string) {
    clearTimer(jobID);
    failures.delete(jobID);
  }

  function schedule(siteID: string, jobID: string, delay: number) {
    clearTimer(jobID);
    timers.set(
      jobID,
      setTimeout(() => void poll(siteID, jobID), delay),
    );
  }

  async function poll(siteID: string, jobID: string) {
    clearTimer(jobID);
    try {
      const jobs = await api.getSiteFetchJobs(siteID, 20);
      merge(jobs);
      failures.delete(jobID);
      const job = jobs.find((item) => item.id === jobID);
      if (job && (job.status === 'queued' || job.status === 'running')) {
        schedule(siteID, jobID, 1000);
        return;
      }
      clear(jobID);
      await options.onCompleted(siteID);
    } catch {
      const count = (failures.get(jobID) ?? 0) + 1;
      failures.set(jobID, count);
      if (count >= 5) {
        clear(jobID);
        return;
      }
      schedule(siteID, jobID, Math.min(2000 * 2 ** (count - 1), 30000));
    }
  }

  /** 开始或重置指定任务的轮询。 */
  function monitor(siteID: string, jobID: string) {
    schedule(siteID, jobID, 0);
  }

  onBeforeUnmount(() => {
    for (const timer of timers.values()) clearTimeout(timer);
    timers.clear();
    failures.clear();
  });

  return { merge, monitor };
}
