<script setup lang="ts">
import { Clock3, KeyRound, RefreshCw, Save } from '@lucide/vue';
import { computed, ref, watch } from 'vue';
import {
  NAlert,
  NButton,
  NCard,
  NDivider,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  NThing,
  NTimePicker,
} from 'naive-ui';
import type { FetchSettings, Site, SiteAttendance, SiteFetchJob, SiteFetchRequest } from '../types';

type CredentialDraft = { user_agent: string; cookie: string };

const props = defineProps<{
  sites: Site[];
  credentials: Record<string, CredentialDraft>;
  attendances: Record<string, SiteAttendance>;
  actions: Record<string, 'fetch' | 'save' | 'attendance' | ''>;
  fetchSettings: FetchSettings;
  fetchJobs: SiteFetchJob[];
}>();

const emit = defineEmits<{
  updateCredential: [siteID: string, patch: Partial<CredentialDraft>];
  updateAttendance: [siteID: string, patch: Partial<SiteAttendance>];
  save: [site: Site];
  saveAttendance: [siteID: string];
  fetch: [siteID: string, request: SiteFetchRequest];
  saveFetchSettings: [settings: FetchSettings];
}>();

const maxPagesDraft = ref(props.fetchSettings.max_pages);
const scanModes = ref<Record<string, 'incremental' | 'pages'>>({});
const scanPages = ref<Record<string, number>>({});
const modeOptions = [
  { label: '增量扫描', value: 'incremental' },
  { label: '固定页数', value: 'pages' },
];

watch(
  () => props.fetchSettings.max_pages,
  (value) => {
    maxPagesDraft.value = value;
  },
);

const jobsBySite = computed(() => {
  const result = new Map<string, SiteFetchJob>();
  for (const job of props.fetchJobs) {
    if (!result.has(job.site_id)) result.set(job.site_id, job);
  }
  return result;
});

function modeFor(siteID: string) {
  return scanModes.value[siteID] ?? 'incremental';
}

function pagesFor(siteID: string) {
  return scanPages.value[siteID] ?? props.fetchSettings.max_pages;
}

function fetchRequest(siteID: string): SiteFetchRequest {
  const mode = modeFor(siteID);
  return mode === 'pages' ? { mode, pages: pagesFor(siteID) } : { mode };
}

function statusType(status: string) {
  if (status === 'completed') return 'success';
  if (status === 'failed') return 'error';
  if (status === 'completed_with_errors') return 'warning';
  return 'info';
}

function browserTimeZone() {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

function formatTime(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
</script>

<template>
  <section class="view-stack">
    <NCard :bordered="false">
      <div>
        <h2>站点设置</h2>
        <p class="muted">管理站点凭据和分页扫描。任何列表抓取都会匹配启用的订阅，并可能向 qBittorrent 添加任务。</p>
      </div>
    </NCard>

    <NCard title="全局抓取设置" :bordered="false">
      <NForm inline @submit.prevent="emit('saveFetchSettings', { max_pages: maxPagesDraft })">
        <NFormItem label="最大扫描页数">
          <NInputNumber v-model:value="maxPagesDraft" :min="1" :max="100" />
        </NFormItem>
        <NButton type="primary" attr-type="submit">保存</NButton>
      </NForm>
    </NCard>

    <NSpace v-if="sites.length" vertical :size="12">
      <NCard v-for="site in sites" :key="site.id" :bordered="false" class="settings-site-card">
        <NThing :title="site.name">
          <template #avatar>
            <NIcon :component="KeyRound" size="20" />
          </template>
          <template #header-extra>
            <NTag :type="site.has_cookie ? 'success' : 'warning'" size="small" round>
              {{ site.has_cookie ? 'Cookie saved' : 'Cookie required' }}
            </NTag>
          </template>
          <p class="muted text-break">{{ site.base_url }}</p>
        </NThing>

        <NForm class="compact-form" @submit.prevent="emit('save', site)">
          <NFormItem label="User-Agent">
            <NInput
              :value="credentials[site.id]?.user_agent ?? ''"
              @update:value="(value) => emit('updateCredential', site.id, { user_agent: value })"
            />
          </NFormItem>
          <NFormItem label="Cookie">
            <NInput
              :value="credentials[site.id]?.cookie ?? ''"
              type="textarea"
              :autosize="{ minRows: 3, maxRows: 5 }"
              placeholder="name=value; name2=value2"
              @update:value="(value) => emit('updateCredential', site.id, { cookie: value })"
            />
          </NFormItem>
          <NSpace>
            <NButton secondary type="primary" attr-type="submit" :loading="actions[site.id] === 'save'">
              <template #icon>
                <NIcon :component="Save" />
              </template>
              Save
            </NButton>
            <NButton
              secondary
              :disabled="!!actions[site.id]"
              :loading="actions[site.id] === 'fetch'"
              @click="emit('fetch', site.id, fetchRequest(site.id))"
            >
              <template #icon>
                <NIcon :component="RefreshCw" />
              </template>
              开始扫描
            </NButton>
          </NSpace>
          <NSpace align="center">
            <NSelect
              :value="modeFor(site.id)"
              :options="modeOptions"
              style="width: 140px"
              @update:value="(value) => (scanModes[site.id] = value)"
            />
            <NInputNumber
              v-if="modeFor(site.id) === 'pages'"
              :value="pagesFor(site.id)"
              :min="1"
              :max="fetchSettings.max_pages"
              @update:value="(value) => (scanPages[site.id] = value ?? 1)"
            />
          </NSpace>
          <NAlert v-if="jobsBySite.get(site.id)" :type="statusType(jobsBySite.get(site.id)!.status)" :bordered="false">
            <NSpace vertical size="small">
              <span
                ><NTag size="small" :type="statusType(jobsBySite.get(site.id)!.status)">{{
                  jobsBySite.get(site.id)!.status
                }}</NTag>
                第 {{ jobsBySite.get(site.id)!.current_page || 0 }} 页 / 已完成
                {{ jobsBySite.get(site.id)!.pages_fetched }} 页</span
              >
              <span
                >新增 {{ jobsBySite.get(site.id)!.inserted }} · 匹配 {{ jobsBySite.get(site.id)!.matched }} · 发送
                {{ jobsBySite.get(site.id)!.download_sent }}</span
              >
              <span v-if="jobsBySite.get(site.id)!.stop_reason"
                >停止原因：{{ jobsBySite.get(site.id)!.stop_reason }}</span
              >
              <span v-if="jobsBySite.get(site.id)!.error" class="text-break">{{ jobsBySite.get(site.id)!.error }}</span>
            </NSpace>
          </NAlert>
        </NForm>

        <NDivider />
        <section v-if="attendances[site.id]" class="attendance-settings">
          <div class="attendance-heading">
            <div>
              <strong>自动签到</strong>
              <p class="muted">每天使用当前站点 Cookie 访问签到页面。</p>
            </div>
            <NSwitch
              :value="attendances[site.id].enabled"
              @update:value="(value) => emit('updateAttendance', site.id, { enabled: value })"
            />
          </div>
          <NForm class="compact-form attendance-form" @submit.prevent="emit('saveAttendance', site.id)">
            <NFormItem label="每日签到时间">
              <NTimePicker
                :formatted-value="attendances[site.id].time_of_day"
                format="HH:mm"
                value-format="HH:mm"
                :clearable="false"
                @update:formatted-value="
                  (value) => emit('updateAttendance', site.id, { time_of_day: value ?? '09:00' })
                "
              />
            </NFormItem>
            <NFormItem label="时区">
              <NSpace align="center">
                <NTag size="small">{{ attendances[site.id].timezone }}</NTag>
                <NButton
                  size="tiny"
                  secondary
                  attr-type="button"
                  @click="emit('updateAttendance', site.id, { timezone: browserTimeZone() })"
                >
                  使用当前浏览器时区
                </NButton>
              </NSpace>
            </NFormItem>
            <div class="attendance-status muted">
              <span>上次：{{ formatTime(attendances[site.id].last_run_at) }}</span>
              <span>下次：{{ formatTime(attendances[site.id].next_run_at) }}</span>
            </div>
            <NAlert v-if="attendances[site.id].last_error" type="warning" :bordered="false" class="text-break">
              {{ attendances[site.id].last_error }}
            </NAlert>
            <NButton type="primary" secondary attr-type="submit" :loading="actions[site.id] === 'attendance'">
              <template #icon>
                <NIcon :component="Clock3" />
              </template>
              保存签到设置
            </NButton>
          </NForm>
        </section>
      </NCard>
    </NSpace>
    <NCard v-else :bordered="false">
      <NEmpty description="No sites configured" />
    </NCard>
  </section>
</template>
