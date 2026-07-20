<script setup lang="ts">
import { KeyRound, RefreshCw, Save } from '@lucide/vue';
import { computed, ref, watch } from 'vue';
import { NAlert, NButton, NCard, NEmpty, NForm, NFormItem, NIcon, NInput, NInputNumber, NSelect, NSpace, NTag, NThing } from 'naive-ui';
import type { FetchSettings, Site, SiteFetchJob, SiteFetchRequest } from '../types';

type CredentialDraft = { user_agent: string; cookie: string };

const props = defineProps<{
  sites: Site[];
  credentials: Record<string, CredentialDraft>;
  actions: Record<string, 'fetch' | 'save' | ''>;
  fetchSettings: FetchSettings;
  fetchJobs: SiteFetchJob[];
}>();

const emit = defineEmits<{
  updateCredential: [siteID: string, patch: Partial<CredentialDraft>];
  save: [site: Site];
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

watch(() => props.fetchSettings.max_pages, (value) => {
	maxPagesDraft.value = value;
});

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
			<NSelect :value="modeFor(site.id)" :options="modeOptions" style="width: 140px" @update:value="(value) => (scanModes[site.id] = value)" />
			<NInputNumber v-if="modeFor(site.id) === 'pages'" :value="pagesFor(site.id)" :min="1" :max="fetchSettings.max_pages" @update:value="(value) => (scanPages[site.id] = value ?? 1)" />
		  </NSpace>
		  <NAlert v-if="jobsBySite.get(site.id)" :type="statusType(jobsBySite.get(site.id)!.status)" :bordered="false">
			<NSpace vertical size="small">
				<span><NTag size="small" :type="statusType(jobsBySite.get(site.id)!.status)">{{ jobsBySite.get(site.id)!.status }}</NTag> 第 {{ jobsBySite.get(site.id)!.current_page || 0 }} 页 / 已完成 {{ jobsBySite.get(site.id)!.pages_fetched }} 页</span>
				<span>新增 {{ jobsBySite.get(site.id)!.inserted }} · 匹配 {{ jobsBySite.get(site.id)!.matched }} · 发送 {{ jobsBySite.get(site.id)!.download_sent }}</span>
				<span v-if="jobsBySite.get(site.id)!.stop_reason">停止原因：{{ jobsBySite.get(site.id)!.stop_reason }}</span>
				<span v-if="jobsBySite.get(site.id)!.error" class="text-break">{{ jobsBySite.get(site.id)!.error }}</span>
			</NSpace>
		  </NAlert>
        </NForm>
      </NCard>
    </NSpace>
    <NCard v-else :bordered="false">
      <NEmpty description="No sites configured" />
    </NCard>
  </section>
</template>
