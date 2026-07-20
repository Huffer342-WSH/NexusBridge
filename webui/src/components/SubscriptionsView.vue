<!-- 订阅工作区集中管理筛选规则、自动订阅、站点周期和批量下载。 -->
<script setup lang="ts">
import {
	CheckCircle2,
	Clock3,
	CloudDownload,
	Database,
	Eye,
	Plus,
	RefreshCw,
	Save,
	Tags,
	Trash2,
} from '@lucide/vue';
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import {
	NAlert,
	NButton,
	NCard,
	NCheckbox,
	NDynamicTags,
	NEmpty,
	NForm,
	NFormItem,
	NIcon,
	NInput,
	NInputNumber,
	NPopover,
	NPopconfirm,
	NRadioButton,
	NRadioGroup,
	NSelect,
	NSpace,
	NSpin,
	NSwitch,
	NTabPane,
	NTabs,
	NTag,
} from 'naive-ui';
import { api } from '../api';
import type {
	BatchDownloadPreview,
	BatchDownloadRequest,
	BatchDownloadResult,
	DownloadOptions,
	FilterRule,
	QBCategoriesResult,
	QBTagsResult,
	RulePreviewItem,
	RulePreviewResult,
	RuleFilterOptions,
	Site,
	SiteSchedule,
	Subscription,
	SubscriptionCandidate,
	SubscriptionPreview,
	SubscriptionPreviewItem,
	SubscriptionRun,
	Torrent,
	TorrentKey,
} from '../types';

const props = defineProps<{
	sites: Site[];
	torrents: Torrent[];
}>();

type WorkspaceTab = 'rules' | 'subscriptions' | 'automation' | 'batch';
type FeedbackType = 'success' | 'warning' | 'error' | 'info';

const activeTab = ref<WorkspaceTab>('rules');
const loading = ref(false);
const action = ref('');
const feedback = ref('');
const feedbackType = ref<FeedbackType>('info');
const rules = ref<FilterRule[]>([]);
const subscriptions = ref<Subscription[]>([]);
const schedules = ref<Record<string, SiteSchedule>>({});
const qbCategories = ref<QBCategoriesResult>({ items: [], stale: false, connected: false });
const qbTags = ref<QBTagsResult>({ items: [], stale: false, connected: false });
const runs = ref<SubscriptionRun[]>([]);
const candidates = ref<SubscriptionCandidate[]>([]);
const rulePreview = ref<RulePreviewResult | null>(null);
const rulePreviewError = ref('');
const rulePreviewLoading = ref(false);
const autoRulePreview = ref(true);
const savedRuleName = ref('');
const facetSiteID = ref('');
const ruleFilterOptions = ref<RuleFilterOptions>({
	site_id: '', site_categories: [], site_tags: [], subtitle_tags: [], promotions: [],
});
const sizeUnit = ref('GB');
const publishedUnit = ref('小时');
let previewTimer: ReturnType<typeof setTimeout> | undefined;
let previewGeneration = 0;
const subscriptionPreview = ref<SubscriptionPreview | null>(null);
const batchPreview = ref<BatchDownloadPreview | null>(null);
const batchResult = ref<BatchDownloadResult | null>(null);
const selectedTorrentKeys = ref<string[]>([]);
const categoryName = ref('');
const categorySavePath = ref('');
const newTags = ref<string[]>([]);
const batchMode = ref<'subscription' | 'custom'>('subscription');
const batchSubscriptionID = ref('');

const sortOptions = [
	{ label: '站点原始顺序', value: 'source_order' },
	{ label: '发布时间', value: 'published_at' },
	{ label: '体积', value: 'size_bytes' },
	{ label: '做种数', value: 'seeders' },
	{ label: '下载数', value: 'leechers' },
	{ label: '完成数', value: 'snatches' },
];

const directionOptions = [
	{ label: '升序', value: 'asc' },
	{ label: '降序', value: 'desc' },
];

const sizeUnitOptions = ['B', 'KB', 'MB', 'GB', 'TB'].map((value) => ({ label: value, value }));
const publishedUnitOptions = [
	{ label: '分钟', value: '分钟' },
	{ label: '小时', value: '小时' },
	{ label: '天', value: '天' },
];
const sizeFactors: Record<string, number> = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4 };
const publishedFactors: Record<string, number> = { 分钟: 1, 小时: 60, 天: 1440 };

const statusType = (status: string): 'success' | 'warning' | 'error' | 'info' | 'default' => {
	if (['sent', 'processed', 'completed', 'success', 'exists'].includes(status)) return 'success';
	if (['failed', 'error'].includes(status)) return 'error';
	if (['unread', 'skipped', 'pending', 'processing'].includes(status)) return 'warning';
	return 'info';
};

function createDownloadOptions(): DownloadOptions {
	return {
		qb_category: '',
		save_path_template: '',
		qb_tags: [],
		filename_template: '',
		paused: false,
		max_concurrent: 0,
		daily_limit: 0,
	};
}

function createRule(): FilterRule {
	return {
		name: '',
		site_ids: [],
		site_categories: [],
		site_tags: [],
		subtitle_tags: [],
		title_expression: '',
		promotions: [],
		min_size: 0,
		max_size: 0,
		min_seeders: 0,
		max_seeders: 0,
		min_leechers: 0,
		max_leechers: 0,
		min_snatches: 0,
		max_snatches: 0,
		published_within_minutes: 0,
		sort_by: 'source_order',
		sort_direction: 'asc',
		action: 'download',
	};
}

function normalizeRule(rule: Partial<FilterRule>): FilterRule {
	return {
		...createRule(),
		...rule,
		site_ids: [...(rule.site_ids ?? [])],
		site_categories: [...(rule.site_categories ?? [])],
		site_tags: [...(rule.site_tags ?? [])],
		subtitle_tags: [...(rule.subtitle_tags ?? [])],
		promotions: [...(rule.promotions ?? [])],
	};
}

function createSubscription(): Subscription {
	return {
		id: `subscription-${Date.now().toString(36)}`,
		name: '',
		enabled: false,
		rule_name: rules.value[0]?.name ?? '',
		site_ids: props.sites[0] ? [props.sites[0].id] : [],
		priority: 0,
		download: createDownloadOptions(),
	};
}

function normalizeSubscription(subscription: Partial<Subscription>): Subscription {
	return {
		...createSubscription(),
		...subscription,
		site_ids: [...(subscription.site_ids ?? [])],
		download: {
			...createDownloadOptions(),
			...(subscription.download ?? {}),
			qb_tags: [...(subscription.download?.qb_tags ?? [])],
		},
	};
}

const ruleDraft = ref<FilterRule>(createRule());
const subscriptionDraft = ref<Subscription>(createSubscription());
const batchOptions = ref<DownloadOptions>(createDownloadOptions());

const siteOptions = computed(() => props.sites.map((site) => ({ label: site.name, value: site.id })));
const ruleOptions = computed(() => rules.value.map((rule) => ({ label: rule.name, value: rule.name })));
const subscriptionOptions = computed(() =>
	subscriptions.value.map((subscription) => ({ label: subscription.name || subscription.id, value: subscription.id })),
);
const categoryOptions = computed(() =>
	qbCategories.value.items.map((category) => ({ label: category.name, value: category.name })),
);
const torrentOptions = computed(() =>
	props.torrents.map((torrent) => ({
		label: `${torrent.title} · ${siteName(torrent.site_id)}`,
		value: torrentKey(torrent.site_id, torrent.id),
	})),
);
const matchedRuleItems = computed(() => rulePreview.value?.items.filter((item) => item.matched) ?? []);
const hasSiteFilters = computed(() =>
	Boolean(ruleDraft.value.site_categories.length || ruleDraft.value.site_tags.length || ruleDraft.value.subtitle_tags.length || ruleDraft.value.promotions.length),
);
const selectedSiteForFacets = computed(() => ruleDraft.value.site_ids.length === 1 ? ruleDraft.value.site_ids[0] : '');
const mergeSelectedOptions = (options: { label: string; value: string }[], selected: string[]) => {
	const values = new Set(options.map((option) => option.value.toLocaleLowerCase()));
	return [...options, ...selected.filter((value) => !values.has(value.toLocaleLowerCase())).map((value) => ({ label: `${value}（历史值）`, value }))];
};
const siteCategoryOptions = computed(() => mergeSelectedOptions(ruleFilterOptions.value.site_categories, ruleDraft.value.site_categories));
const siteTagOptions = computed(() => mergeSelectedOptions(ruleFilterOptions.value.site_tags, ruleDraft.value.site_tags));
const subtitleTagOptions = computed(() => mergeSelectedOptions(ruleFilterOptions.value.subtitle_tags, ruleDraft.value.subtitle_tags));
const promotionOptions = computed(() => mergeSelectedOptions(ruleFilterOptions.value.promotions, ruleDraft.value.promotions));
const minSizeValue = computed({
	get: () => ruleDraft.value.min_size ? ruleDraft.value.min_size / sizeFactors[sizeUnit.value] : null,
	set: (value: number | null) => { ruleDraft.value.min_size = value ? Math.round(value * sizeFactors[sizeUnit.value]) : 0; },
});
const maxSizeValue = computed({
	get: () => ruleDraft.value.max_size ? ruleDraft.value.max_size / sizeFactors[sizeUnit.value] : null,
	set: (value: number | null) => { ruleDraft.value.max_size = value ? Math.round(value * sizeFactors[sizeUnit.value]) : 0; },
});
const publishedWithinValue = computed({
	get: () => ruleDraft.value.published_within_minutes ? ruleDraft.value.published_within_minutes / publishedFactors[publishedUnit.value] : null,
	set: (value: number | null) => { ruleDraft.value.published_within_minutes = value ? Math.round(value * publishedFactors[publishedUnit.value]) : 0; },
});
const categoryRows = computed(() =>
	[...qbCategories.value.items]
		.sort((left, right) => left.name.localeCompare(right.name))
		.map((category) => ({ ...category, depth: Math.max(0, category.path_segments.length - 1) })),
);

function torrentKey(siteID: string, torrentID: string) {
	return `${siteID}::${torrentID}`;
}

function parseTorrentKey(key: string): TorrentKey {
	const delimiter = key.indexOf('::');
	return {
		site_id: delimiter >= 0 ? key.slice(0, delimiter) : '',
		torrent_id: delimiter >= 0 ? key.slice(delimiter + 2) : key,
	};
}

function siteName(siteID: string) {
	return props.sites.find((site) => site.id === siteID)?.name ?? siteID;
}

function torrentTitle(siteID: string, torrentID: string) {
	return props.torrents.find((torrent) => torrent.site_id === siteID && torrent.id === torrentID)?.title ?? torrentID;
}

function setFeedback(type: FeedbackType, message: string) {
	feedbackType.value = type;
	feedback.value = message;
}

function formatTime(value?: string) {
	if (!value) return '—';
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatBytes(value: number) {
	if (!value) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
	return `${(value / 1024 ** index).toFixed(index ? 2 : 0)} ${units[index]}`;
}

function selectedLabels(options: { label: string; value: string }[], values: string[]) {
	const labels = new Map(options.map((option) => [option.value, option.label]));
	return values.map((value) => labels.get(value) ?? value).join('、') || '不限';
}

function reasonText(item: RulePreviewItem | SubscriptionPreviewItem) {
	const reasons = [
		...(item.reasons ?? []),
		...('plan' in item ? (item.plan?.reasons ?? []) : []),
	];
	if (reasons.length) return reasons.map((reason) => reason.message || reason.code).join('；');
	return item.matched ? '命中' : '未命中';
}

function qbStateText(state: string) {
	if (state === 'added') return '已添加到 qB';
	if (state === 'not_added') return '未添加到 qB';
	return 'qB 状态未确认';
}

function qbStateType(state: string): 'success' | 'default' | 'warning' {
	if (state === 'added') return 'success';
	if (state === 'not_added') return 'default';
	return 'warning';
}

function updateRuleSites(siteIDs: string[]) {
	if (hasSiteFilters.value && siteIDs.length > 1) {
		ruleDraft.value.site_ids = siteIDs.slice(-1);
		setFeedback('warning', '使用站点分类、标签或促销时只能选择一个站点');
		return;
	}
	ruleDraft.value.site_ids = siteIDs;
}

async function loadRuleFilterOptions(siteID: string) {
	if (!siteID) {
		ruleFilterOptions.value = { site_id: '', site_categories: [], site_tags: [], subtitle_tags: [], promotions: [] };
		return;
	}
	try {
		ruleFilterOptions.value = await api.getRuleFilterOptions(siteID);
	} catch (error) {
		ruleFilterOptions.value = { site_id: siteID, site_categories: [], site_tags: [], subtitle_tags: [], promotions: [] };
		setFeedback('warning', error instanceof Error ? error.message : '站点筛选选项加载失败');
	}
}

function editRule(rule: FilterRule) {
	savedRuleName.value = rule.name;
	facetSiteID.value = rule.site_ids.length === 1 ? rule.site_ids[0] : '';
	ruleDraft.value = normalizeRule(rule);
	void loadRuleFilterOptions(facetSiteID.value);
	scheduleRulePreview();
}

function newRule() {
	savedRuleName.value = '';
	facetSiteID.value = '';
	ruleDraft.value = createRule();
	ruleFilterOptions.value = { site_id: '', site_categories: [], site_tags: [], subtitle_tags: [], promotions: [] };
	scheduleRulePreview();
}

function editSubscription(subscription: Subscription) {
	subscriptionDraft.value = normalizeSubscription(subscription);
	subscriptionPreview.value = null;
	void loadCandidates(subscription.id);
}

function newSubscription() {
	subscriptionDraft.value = createSubscription();
	subscriptionPreview.value = null;
	candidates.value = [];
}

async function refreshWorkspace() {
	loading.value = true;
	feedback.value = '';
	const [rulesResult, subscriptionsResult, categoriesResult, tagsResult, runsResult] = await Promise.allSettled([
		api.getRules(),
		api.getSubscriptions(),
		api.getQBCategories(),
		api.getQBTags(),
		api.getSubscriptionRuns(),
	]);
	const failures: string[] = [];
	if (rulesResult.status === 'fulfilled') {
		rules.value = rulesResult.value.map(normalizeRule);
	} else failures.push(`规则：${String(rulesResult.reason)}`);
	if (subscriptionsResult.status === 'fulfilled') {
		subscriptions.value = subscriptionsResult.value.map(normalizeSubscription);
	} else failures.push(`订阅：${String(subscriptionsResult.reason)}`);
	if (categoriesResult.status === 'fulfilled') qbCategories.value = categoriesResult.value;
	else failures.push(`qB 分类：${String(categoriesResult.reason)}`);
	if (tagsResult.status === 'fulfilled') qbTags.value = tagsResult.value;
	else failures.push(`qB 标签：${String(tagsResult.reason)}`);
	if (runsResult.status === 'fulfilled') runs.value = runsResult.value;
	else failures.push(`运行记录：${String(runsResult.reason)}`);

	const scheduleResults = await Promise.allSettled(props.sites.map((site) => api.getSiteSchedule(site.id)));
	const nextSchedules: Record<string, SiteSchedule> = {};
	props.sites.forEach((site, index) => {
		const result = scheduleResults[index];
		nextSchedules[site.id] = result?.status === 'fulfilled'
			? result.value
			: { site_id: site.id, enabled: false, interval_seconds: 900 };
	});
	schedules.value = nextSchedules;

	if (!ruleDraft.value.name && rules.value[0]) editRule(rules.value[0]);
	if (!subscriptionDraft.value.name && subscriptions.value[0]) editSubscription(subscriptions.value[0]);
	if (!batchSubscriptionID.value && subscriptions.value[0]) batchSubscriptionID.value = subscriptions.value[0].id;
	if (failures.length) setFeedback('warning', `部分自动化数据暂不可用。${failures.join('；')}`);
	loading.value = false;
}

async function saveRule() {
	if (!ruleDraft.value.name.trim()) {
		setFeedback('warning', '规则名称不能为空');
		return;
	}
	action.value = 'save-rule';
	try {
		const originalName = savedRuleName.value;
		const saved = normalizeRule(await (originalName ? api.updateRule(originalName, ruleDraft.value) : api.saveRule(ruleDraft.value)));
		const index = rules.value.findIndex((rule) => rule.name.toLocaleLowerCase() === originalName.toLocaleLowerCase());
		if (index >= 0) rules.value[index] = saved;
		else rules.value.push(saved);
		if (originalName && originalName.toLocaleLowerCase() !== saved.name.toLocaleLowerCase()) {
			subscriptions.value = subscriptions.value.map((subscription) =>
				subscription.rule_name.toLocaleLowerCase() === originalName.toLocaleLowerCase()
					? { ...subscription, rule_name: saved.name }
					: subscription,
			);
			if (subscriptionDraft.value.rule_name.toLocaleLowerCase() === originalName.toLocaleLowerCase()) {
				subscriptionDraft.value.rule_name = saved.name;
			}
		}
		savedRuleName.value = saved.name;
		ruleDraft.value = normalizeRule(saved);
		setFeedback('success', `已保存规则「${saved.name}」`);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '保存规则失败');
	} finally {
		action.value = '';
	}
}

async function removeRule(rule: FilterRule) {
	action.value = `delete-rule:${rule.name}`;
	try {
		await api.deleteRule(rule.name);
		rules.value = rules.value.filter((item) => item.name.toLocaleLowerCase() !== rule.name.toLocaleLowerCase());
		newRule();
		setFeedback('success', `已删除规则「${rule.name}」`);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '删除规则失败');
	} finally {
		action.value = '';
	}
}

async function previewRule(automatic = false) {
	const generation = ++previewGeneration;
	rulePreviewLoading.value = true;
	rulePreviewError.value = '';
	try {
		const result = await api.previewRule({ rule: ruleDraft.value, limit: 50, offset: 0 });
		if (generation !== previewGeneration) return;
		rulePreview.value = result;
		selectedTorrentKeys.value = selectedTorrentKeys.value.filter((key) =>
			rulePreview.value?.items.some((item) => torrentKey(item.torrent.site_id, item.torrent.id) === key),
		);
		if (!automatic) setFeedback('success', `预览完成：命中 ${rulePreview.value.matched}/${rulePreview.value.evaluated}`);
	} catch (error) {
		if (generation !== previewGeneration) return;
		rulePreviewError.value = error instanceof Error ? error.message : '规则预览失败';
		if (!automatic) setFeedback('error', rulePreviewError.value);
	} finally {
		if (generation === previewGeneration) rulePreviewLoading.value = false;
	}
}

function scheduleRulePreview() {
	if (previewTimer) clearTimeout(previewTimer);
	if (!autoRulePreview.value) return;
	previewTimer = setTimeout(() => void previewRule(true), 350);
}

function selectAllMatched() {
	const keys = matchedRuleItems.value.map((item) => torrentKey(item.torrent.site_id, item.torrent.id));
	selectedTorrentKeys.value = Array.from(new Set([...selectedTorrentKeys.value, ...keys]));
}

function toggleTorrent(key: string, checked: boolean) {
	selectedTorrentKeys.value = checked
		? Array.from(new Set([...selectedTorrentKeys.value, key]))
		: selectedTorrentKeys.value.filter((item) => item !== key);
}

async function saveSubscription() {
	const draft = subscriptionDraft.value;
	if (!draft.id.trim() || !draft.name.trim() || !draft.rule_name || draft.site_ids.length === 0) {
		setFeedback('warning', '订阅 ID、名称、筛选规则和站点不能为空');
		return;
	}
	action.value = 'save-subscription';
	try {
		const saved = normalizeSubscription(await api.saveSubscription(draft));
		const index = subscriptions.value.findIndex((subscription) => subscription.id === saved.id);
		if (index >= 0) subscriptions.value[index] = saved;
		else subscriptions.value.push(saved);
		subscriptionDraft.value = saved;
		if (!batchSubscriptionID.value) batchSubscriptionID.value = saved.id;
		setFeedback('success', `已保存订阅「${saved.name}」`);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '保存订阅失败');
	} finally {
		action.value = '';
	}
}

async function removeSubscription(subscription: Subscription) {
	action.value = `delete-subscription:${subscription.id}`;
	try {
		await api.deleteSubscription(subscription.id);
		subscriptions.value = subscriptions.value.filter((item) => item.id !== subscription.id);
		newSubscription();
		setFeedback('success', `已删除订阅「${subscription.name}」`);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '删除订阅失败');
	} finally {
		action.value = '';
	}
}

async function previewSubscription() {
	if (!subscriptions.value.some((subscription) => subscription.id === subscriptionDraft.value.id)) {
		setFeedback('warning', '请先保存订阅，再预览最终下载计划');
		return;
	}
	action.value = 'preview-subscription';
	try {
		subscriptionPreview.value = await api.previewSubscription(subscriptionDraft.value.id);
		setFeedback(
			'success',
			`预览完成：命中 ${subscriptionPreview.value.matched}，可执行 ${subscriptionPreview.value.eligible}`,
		);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '订阅预览失败');
	} finally {
		action.value = '';
	}
}

async function loadCandidates(subscriptionID: string) {
	try {
		candidates.value = await api.getSubscriptionCandidates(subscriptionID);
	} catch (error) {
		candidates.value = [];
		setFeedback('warning', error instanceof Error ? error.message : '候选列表加载失败');
	}
}

async function loadRuns() {
	try {
		runs.value = await api.getSubscriptionRuns();
	} catch (error) {
		setFeedback('warning', error instanceof Error ? error.message : '运行记录加载失败');
	}
}

async function saveSchedule(siteID: string) {
	const schedule = schedules.value[siteID];
	if (!schedule) return;
	action.value = `schedule:${siteID}`;
	try {
		schedules.value[siteID] = await api.saveSiteSchedule(siteID, schedule);
		setFeedback('success', `已保存 ${siteName(siteID)} 的自动检索周期`);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '保存站点周期失败');
	} finally {
		action.value = '';
	}
}

async function refreshQBCategories() {
	action.value = 'qb-categories';
	try {
		qbCategories.value = await api.getQBCategories(true);
		setFeedback(qbCategories.value.stale ? 'warning' : 'success', qbCategories.value.stale ? '已显示分类缓存' : 'qB 分类已同步');
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '同步 qB 分类失败');
	} finally {
		action.value = '';
	}
}

async function createCategory() {
	if (!categoryName.value.trim()) {
		setFeedback('warning', '分类完整名称不能为空');
		return;
	}
	action.value = 'create-category';
	try {
		qbCategories.value = await api.createQBCategory(categoryName.value.trim(), categorySavePath.value.trim());
		categoryName.value = '';
		categorySavePath.value = '';
		setFeedback('success', 'qB 分类已创建');
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '创建 qB 分类失败');
	} finally {
		action.value = '';
	}
}

async function refreshQBTags() {
	action.value = 'qb-tags';
	try {
		qbTags.value = await api.getQBTags(true);
		setFeedback(qbTags.value.stale ? 'warning' : 'success', qbTags.value.stale ? '已显示标签缓存' : 'qB 标签已同步');
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '同步 qB 标签失败');
	} finally {
		action.value = '';
	}
}

async function createTags() {
	if (!newTags.value.length) {
		setFeedback('warning', '至少填写一个标签');
		return;
	}
	action.value = 'create-tags';
	try {
		qbTags.value = await api.createQBTags(newTags.value);
		newTags.value = [];
		setFeedback('success', 'qB 标签已创建');
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '创建 qB 标签失败');
	} finally {
		action.value = '';
	}
}

function buildBatchPayload(): BatchDownloadRequest | null {
	const torrents = selectedTorrentKeys.value.map(parseTorrentKey).filter((key) => key.site_id && key.torrent_id);
	if (!torrents.length) {
		setFeedback('warning', '请先从规则预览或批量页选择种子');
		return null;
	}
	if (batchMode.value === 'subscription') {
		if (!batchSubscriptionID.value) {
			setFeedback('warning', '请选择要快速套用的订阅');
			return null;
		}
		return { torrents, subscription_id: batchSubscriptionID.value };
	}
	return { torrents, options: batchOptions.value };
}

async function previewBatch() {
	const payload = buildBatchPayload();
	if (!payload) return;
	action.value = 'preview-batch';
	try {
		batchPreview.value = await api.previewBatchDownloads(payload);
		batchResult.value = null;
		setFeedback('success', `已生成 ${batchPreview.value.items.length} 个只读下载计划`);
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '批量预览失败');
	} finally {
		action.value = '';
	}
}

async function executeBatch() {
	const payload = buildBatchPayload();
	if (!payload) return;
	action.value = 'execute-batch';
	try {
		batchResult.value = await api.executeBatchDownloads(payload);
		setFeedback(
			batchResult.value.failed ? 'warning' : 'success',
			`批量执行完成：发送 ${batchResult.value.sent}，已存在 ${batchResult.value.exists}，失败 ${batchResult.value.failed}`,
		);
		await loadRuns();
	} catch (error) {
		setFeedback('error', error instanceof Error ? error.message : '批量执行失败');
	} finally {
		action.value = '';
	}
}

function openBatchFromRule() {
	selectAllMatched();
	activeTab.value = 'batch';
}

watch(ruleDraft, scheduleRulePreview, { deep: true });
watch(autoRulePreview, (enabled) => {
	if (enabled) scheduleRulePreview();
	else if (previewTimer) clearTimeout(previewTimer);
});
watch(selectedSiteForFacets, async (siteID) => {
	if (facetSiteID.value && facetSiteID.value !== siteID) {
		ruleDraft.value.site_categories = [];
		ruleDraft.value.site_tags = [];
		ruleDraft.value.subtitle_tags = [];
		ruleDraft.value.promotions = [];
	}
	facetSiteID.value = siteID;
	await loadRuleFilterOptions(siteID);
});

onMounted(async () => {
	await refreshWorkspace();
	await nextTick();
	scheduleRulePreview();
});
onBeforeUnmount(() => {
	if (previewTimer) clearTimeout(previewTimer);
	previewGeneration++;
});
</script>

<template>
	<section class="view-stack subscriptions-workspace" data-testid="subscriptions-view">
		<header class="workspace-header">
			<div>
				<h2>订阅与筛选</h2>
				<p class="muted">先用数据库预览确认命中结果，再显式启用站点周期和自动订阅。</p>
			</div>
			<NButton :loading="loading" @click="refreshWorkspace">
				<template #icon><NIcon :component="RefreshCw" /></template>
				刷新自动化数据
			</NButton>
		</header>

		<NAlert v-if="feedback" :type="feedbackType" closable @close="feedback = ''">
			{{ feedback }}
		</NAlert>

		<NSpin :show="loading">
			<NTabs v-model:value="activeTab" type="segment" animated>
				<NTabPane name="rules" tab="筛选规则">
					<div class="rules-workspace-grid">
						<NCard title="规则" size="small">
							<template #header-extra>
								<NButton size="small" @click="newRule">
									<template #icon><NIcon :component="Plus" /></template>新建
								</NButton>
							</template>
							<div v-if="rules.length" class="entity-list" data-testid="rule-list">
								<button
									v-for="rule in rules"
									:key="rule.name"
									type="button"
									:class="{ active: savedRuleName.toLocaleLowerCase() === rule.name.toLocaleLowerCase() }"
									@click="editRule(rule)"
								>
									<span><strong>{{ rule.name }}</strong><small>{{ rule.site_ids.map(siteName).join('、') || '全部站点' }}</small></span>
								</button>
							</div>
							<NEmpty v-else description="尚无筛选规则" />
						</NCard>

						<NCard title="规则编辑器" size="small" data-testid="rule-editor">
							<NForm label-placement="top" class="compact-form">
								<NFormItem label="规则名称（唯一标识）"><NInput v-model:value="ruleDraft.name" placeholder="例如：近期免费 ASMR" /></NFormItem>
								<NFormItem label="站点（多个值为 OR）">
									<NSelect :value="ruleDraft.site_ids" multiple clearable :options="siteOptions" @update:value="updateRuleSites" />
									<small v-if="hasSiteFilters" class="muted">已使用站点来源条件，因此只能保留一个站点。</small>
								</NFormItem>
								<div class="form-grid form-grid-2">
									<NFormItem label="站点分类（多个值为 OR）"><NSelect v-model:value="ruleDraft.site_categories" multiple clearable filterable :disabled="!selectedSiteForFacets" :options="siteCategoryOptions" /></NFormItem>
									<NFormItem label="促销（多个值为 OR）"><NSelect v-model:value="ruleDraft.promotions" multiple clearable filterable :disabled="!selectedSiteForFacets" :options="promotionOptions" /></NFormItem>
									<NFormItem label="站点标签（必须全部具备）"><NSelect v-model:value="ruleDraft.site_tags" multiple clearable filterable :disabled="!selectedSiteForFacets" :options="siteTagOptions" /></NFormItem>
									<NFormItem label="标签（来自副标题，必须全部具备）"><NSelect v-model:value="ruleDraft.subtitle_tags" multiple clearable filterable :disabled="!selectedSiteForFacets" :options="subtitleTagOptions" /></NFormItem>
								</div>
								<NFormItem label="主标题逻辑表达式">
									<NInput v-model:value="ruleDraft.title_expression" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder='A&B&C|D|E&!F&(!H|!I)' />
									<small class="muted">优先级：! 高于 &，& 高于 |；支持括号。包含运算符的关键词用双引号，例如 "A&B"。</small>
								</NFormItem>
								<div class="metric-pairs">
									<NFormItem label="体积范围">
										<div class="range-input-row"><NInputNumber v-model:value="minSizeValue" :min="0" placeholder="最小" /><span>至</span><NInputNumber v-model:value="maxSizeValue" :min="0" placeholder="最大" /><NSelect v-model:value="sizeUnit" :options="sizeUnitOptions" /></div>
									</NFormItem>
									<NFormItem label="做种数量"><div class="range-input-row"><NInputNumber v-model:value="ruleDraft.min_seeders" :min="0" placeholder="最少" /><span>至</span><NInputNumber v-model:value="ruleDraft.max_seeders" :min="0" placeholder="最多" /></div></NFormItem>
									<NFormItem label="下载数量"><div class="range-input-row"><NInputNumber v-model:value="ruleDraft.min_leechers" :min="0" placeholder="最少" /><span>至</span><NInputNumber v-model:value="ruleDraft.max_leechers" :min="0" placeholder="最多" /></div></NFormItem>
									<NFormItem label="完成数量"><div class="range-input-row"><NInputNumber v-model:value="ruleDraft.min_snatches" :min="0" placeholder="最少" /><span>至</span><NInputNumber v-model:value="ruleDraft.max_snatches" :min="0" placeholder="最多" /></div></NFormItem>
									<NFormItem label="发布时间"><div class="range-input-row published-range"><span>最近</span><NInputNumber v-model:value="publishedWithinValue" :min="0" placeholder="不限" /><NSelect v-model:value="publishedUnit" :options="publishedUnitOptions" /></div></NFormItem>
								</div>
								<div class="form-grid form-grid-2">
									<NFormItem label="排序字段"><NSelect v-model:value="ruleDraft.sort_by" :options="sortOptions" /></NFormItem>
									<NFormItem label="排序方向"><NSelect v-model:value="ruleDraft.sort_direction" :options="directionOptions" /></NFormItem>
								</div>
								<NSpace justify="space-between">
									<NPopconfirm v-if="savedRuleName" @positive-click="removeRule(ruleDraft)">
										<template #trigger>
											<NButton type="error" secondary :loading="action === `delete-rule:${ruleDraft.name}`">
												<template #icon><NIcon :component="Trash2" /></template>删除
											</NButton>
										</template>
										被订阅引用的规则不能删除，确定继续？
									</NPopconfirm>
									<NSpace>
										<NButton type="primary" :loading="action === 'save-rule'" data-testid="save-rule" @click="saveRule">
											<template #icon><NIcon :component="Save" /></template>保存规则
										</NButton>
									</NSpace>
								</NSpace>
							</NForm>
						</NCard>

						<NCard class="preview-card rule-preview-side" title="草稿预览" size="small" data-testid="rule-preview-results">
						<template #header-extra>
							<NSpace>
								<span class="muted">自动</span><NSwitch v-model:value="autoRulePreview" size="small" />
								<NButton size="small" :loading="rulePreviewLoading" data-testid="preview-rule" @click="previewRule(false)"><template #icon><NIcon :component="Eye" /></template>刷新</NButton>
							</NSpace>
						</template>
						<NAlert v-if="rulePreviewError" type="error">{{ rulePreviewError }}</NAlert>
						<template v-if="rulePreview">
							<div class="preview-summary">
								<NTag type="success">命中 {{ rulePreview.matched }}/{{ rulePreview.evaluated }}</NTag>
								<NButton size="small" @click="selectAllMatched">选择全部命中</NButton>
								<NButton size="small" type="primary" @click="openBatchFromRule">批量下载</NButton>
							</div>
						<div class="result-table rule-preview-results">
							<NPopover v-for="item in rulePreview.items" :key="torrentKey(item.torrent.site_id, item.torrent.id)" trigger="hover" placement="left" :show-arrow="false">
								<template #trigger><div class="result-row" :class="{ matched: item.matched }">
								<NCheckbox
									:checked="selectedTorrentKeys.includes(torrentKey(item.torrent.site_id, item.torrent.id))"
									:disabled="!item.matched"
									@update:checked="toggleTorrent(torrentKey(item.torrent.site_id, item.torrent.id), $event)"
								/>
								<div><strong>{{ item.torrent.title }}</strong><small>{{ siteName(item.torrent.site_id) }} · {{ item.torrent.category || '无分类' }}</small></div>
								<div class="preview-status-tags"><NTag :type="item.matched ? 'success' : 'default'">{{ item.matched ? '命中' : '排除' }}</NTag><NTag size="small" :type="qbStateType(item.qb_state)">{{ qbStateText(item.qb_state) }}</NTag></div>
								<p>{{ reasonText(item) }}</p>
								</div></template>
								<div class="rule-hover-details">
									<strong>筛选详情</strong>
									<span>主标题：{{ item.torrent.title }}</span><span>表达式：{{ ruleDraft.title_expression || '不限' }}</span>
									<span>站点分类：{{ item.torrent.category || '无' }} / 规则 {{ selectedLabels(siteCategoryOptions, ruleDraft.site_categories) }}</span>
									<span>站点标签：{{ item.torrent.tag_ids?.join('、') || '无' }} / 规则 {{ selectedLabels(siteTagOptions, ruleDraft.site_tags) }}</span>
									<span>副标题标签：{{ item.torrent.tags?.join('、') || '无' }} / 规则 {{ selectedLabels(subtitleTagOptions, ruleDraft.subtitle_tags) }}</span>
									<span>促销：{{ item.torrent.promotion || '普通' }} / 规则 {{ selectedLabels(promotionOptions, ruleDraft.promotions) }}</span>
									<span>体积：{{ formatBytes(item.torrent.size_bytes || 0) }}；做种 {{ item.torrent.seeders || 0 }}；下载 {{ item.torrent.leechers || 0 }}；完成 {{ item.torrent.snatches || 0 }}</span>
									<span>发布时间：{{ formatTime(item.torrent.published_at) }}；{{ qbStateText(item.qb_state) }}</span>
									<span>结论：{{ reasonText(item) }}</span>
								</div>
							</NPopover>
						</div>
						</template>
						<NEmpty v-else-if="!rulePreviewLoading" description="修改左侧规则后会自动生成数据库预览" />
					</NCard>
					</div>
				</NTabPane>

				<NTabPane name="subscriptions" tab="自动订阅">
					<div class="workspace-two-pane">
						<NCard title="订阅" size="small">
							<template #header-extra><NButton size="small" @click="newSubscription"><template #icon><NIcon :component="Plus" /></template>新建</NButton></template>
							<div v-if="subscriptions.length" class="entity-list" data-testid="subscription-list">
								<button
									v-for="subscription in subscriptions"
									:key="subscription.id"
									type="button"
									:class="{ active: subscriptionDraft.id === subscription.id }"
									@click="editSubscription(subscription)"
								>
									<span><strong>{{ subscription.name || subscription.id }}</strong><small>优先级 {{ subscription.priority }}</small></span>
									<NTag size="small" :type="subscription.enabled ? 'success' : 'default'">{{ subscription.enabled ? '启用' : '关闭' }}</NTag>
								</button>
							</div>
							<NEmpty v-else description="尚无自动订阅" />
						</NCard>

						<NCard title="订阅编辑器" size="small" data-testid="subscription-editor">
							<NForm label-placement="top" class="compact-form">
								<div class="form-grid form-grid-3">
									<NFormItem label="订阅 ID"><NInput v-model:value="subscriptionDraft.id" /></NFormItem>
									<NFormItem label="名称"><NInput v-model:value="subscriptionDraft.name" /></NFormItem>
									<NFormItem label="自动执行"><NSwitch v-model:value="subscriptionDraft.enabled"><template #checked>启用</template><template #unchecked>关闭</template></NSwitch></NFormItem>
									<NFormItem label="筛选规则"><NSelect v-model:value="subscriptionDraft.rule_name" :options="ruleOptions" /></NFormItem>
									<NFormItem label="优先级"><NInputNumber v-model:value="subscriptionDraft.priority" /></NFormItem>
									<NFormItem label="站点"><NSelect v-model:value="subscriptionDraft.site_ids" multiple :options="siteOptions" /></NFormItem>
								</div>
								<div class="form-grid form-grid-2">
									<NFormItem label="qB 分类（完整名称）">
										<NSelect v-model:value="subscriptionDraft.download.qb_category" clearable filterable tag :options="categoryOptions" />
									</NFormItem>
									<NFormItem label="保存路径模板"><NInput v-model:value="subscriptionDraft.download.save_path_template" placeholder="D:/PT/{{site_name}}" /></NFormItem>
									<NFormItem label="qB 标签"><NDynamicTags v-model:value="subscriptionDraft.download.qb_tags" /></NFormItem>
									<NFormItem label="任务名称模板"><NInput v-model:value="subscriptionDraft.download.filename_template" placeholder="{{title}}" /></NFormItem>
									<NFormItem label="最大并发（0 不限）"><NInputNumber v-model:value="subscriptionDraft.download.max_concurrent" :min="0" /></NFormItem>
									<NFormItem label="每日配额（0 不限）"><NInputNumber v-model:value="subscriptionDraft.download.daily_limit" :min="0" /></NFormItem>
									<NFormItem label="添加后暂停"><NSwitch v-model:value="subscriptionDraft.download.paused" /></NFormItem>
								</div>
								<p class="template-help muted">可用占位符：site_id、site_name、torrent_id、category、category_query、rule_name、subscription_name、title、detail_title、subtitle；使用双花括号包裹。</p>
								<NSpace justify="space-between">
									<NPopconfirm v-if="subscriptions.some((item) => item.id === subscriptionDraft.id)" @positive-click="removeSubscription(subscriptionDraft)">
										<template #trigger><NButton type="error" secondary><template #icon><NIcon :component="Trash2" /></template>删除</NButton></template>
										确定删除此订阅？
									</NPopconfirm>
									<NSpace>
										<NButton :loading="action === 'preview-subscription'" data-testid="preview-subscription" @click="previewSubscription"><template #icon><NIcon :component="Eye" /></template>预览</NButton>
										<NButton type="primary" :loading="action === 'save-subscription'" data-testid="save-subscription" @click="saveSubscription"><template #icon><NIcon :component="Save" /></template>保存订阅</NButton>
									</NSpace>
								</NSpace>
							</NForm>
						</NCard>
					</div>

					<NCard v-if="subscriptionPreview" title="最终下载计划预览" size="small" class="preview-card" data-testid="subscription-preview-results">
						<template #header-extra><NTag type="success">可执行 {{ subscriptionPreview.eligible }}/{{ subscriptionPreview.evaluated }}</NTag></template>
						<div class="plan-list">
							<article v-for="item in subscriptionPreview.items" :key="torrentKey(item.torrent.site_id, item.torrent.id)" class="plan-item">
								<header><strong>{{ item.torrent.title }}</strong><NTag :type="item.eligible ? 'success' : 'warning'">{{ item.eligible ? '可执行' : '阻塞' }}</NTag></header>
								<div class="plan-grid">
									<span>分类：{{ item.plan.category || 'qB 默认' }}</span><span>路径：{{ item.plan.save_path || '分类/默认路径' }}</span>
									<span>名称：{{ item.plan.rename || item.plan.original_name || '原始名称' }}</span><span>标签：{{ item.plan.tags.join(', ') || '无' }}</span>
								</div>
								<p class="muted">{{ reasonText(item) }}</p>
							</article>
						</div>
					</NCard>

					<NCard title="未读候选" size="small">
						<div v-if="candidates.length" class="audit-list">
							<div v-for="candidate in candidates" :key="torrentKey(candidate.site_id, candidate.torrent_id)">
								<span><strong>{{ torrentTitle(candidate.site_id, candidate.torrent_id) }}</strong><small>{{ siteName(candidate.site_id) }} · 顺序 {{ candidate.source_order }}</small></span>
								<NTag :type="statusType(candidate.status)">{{ candidate.status }}</NTag>
								<small>{{ candidate.reason_code || formatTime(candidate.updated_at) }}</small>
							</div>
						</div>
						<NEmpty v-else description="当前订阅没有候选" />
					</NCard>
				</NTabPane>

				<NTabPane name="automation" tab="周期与 qB 资源">
					<div class="automation-grid">
						<NCard title="站点自动检索" size="small">
							<div class="schedule-list">
								<article v-for="site in sites" :key="site.id">
									<header><div><strong>{{ site.name }}</strong><small>{{ site.id }}</small></div><NSwitch v-if="schedules[site.id]" v-model:value="schedules[site.id].enabled" /></header>
									<NFormItem v-if="schedules[site.id]" label="间隔（秒，60–86400）">
										<NInputNumber v-model:value="schedules[site.id].interval_seconds" :min="60" :max="86400" :step="60" />
									</NFormItem>
									<div v-if="schedules[site.id]" class="schedule-meta muted">
										<span>上次：{{ formatTime(schedules[site.id].last_run_at) }}</span><span>下次：{{ formatTime(schedules[site.id].next_run_at) }}</span>
									</div>
									<NAlert v-if="schedules[site.id]?.last_error" type="warning" :bordered="false">{{ schedules[site.id].last_error }}</NAlert>
									<NButton size="small" :loading="action === `schedule:${site.id}`" @click="saveSchedule(site.id)"><template #icon><NIcon :component="Clock3" /></template>保存周期</NButton>
								</article>
							</div>
						</NCard>

						<NCard title="qB 分类" size="small">
							<template #header-extra><NButton size="small" :loading="action === 'qb-categories'" @click="refreshQBCategories"><template #icon><NIcon :component="RefreshCw" /></template>同步</NButton></template>
							<NAlert v-if="qbCategories.stale || qbCategories.error" :type="qbCategories.error ? 'warning' : 'info'" :bordered="false">{{ qbCategories.error || 'qB 离线，当前展示最近一次缓存' }}</NAlert>
							<div class="category-tree" data-testid="qb-category-tree">
								<div v-for="category in categoryRows" :key="category.name" :style="{ paddingLeft: `${category.depth * 18 + 10}px` }">
									<NIcon :component="Database" /><span><strong>{{ category.path_segments.at(-1) || category.name }}</strong><small>{{ category.name }} · {{ category.save_path || '默认路径' }}</small></span>
								</div>
							</div>
							<NForm label-placement="top" class="resource-create-form">
								<NFormItem label="完整分类名"><NInput v-model:value="categoryName" placeholder="PT/ASMR" /></NFormItem>
								<NFormItem label="保存路径"><NInput v-model:value="categorySavePath" /></NFormItem>
								<NButton type="primary" :loading="action === 'create-category'" data-testid="create-qb-category" @click="createCategory"><template #icon><NIcon :component="Plus" /></template>创建完整分类</NButton>
							</NForm>
						</NCard>

						<NCard title="qB 标签" size="small">
							<template #header-extra><NButton size="small" :loading="action === 'qb-tags'" @click="refreshQBTags"><template #icon><NIcon :component="RefreshCw" /></template>同步</NButton></template>
							<div class="tag-cloud"><NTag v-for="tag in qbTags.items" :key="tag" type="info">{{ tag }}</NTag><span v-if="!qbTags.items.length" class="muted">尚无标签</span></div>
							<NFormItem label="新标签"><NDynamicTags v-model:value="newTags" /></NFormItem>
							<NButton type="primary" :loading="action === 'create-tags'" @click="createTags"><template #icon><NIcon :component="Tags" /></template>创建标签</NButton>
						</NCard>
					</div>

					<NCard title="最近运行" size="small">
						<template #header-extra><NButton size="small" @click="loadRuns"><template #icon><NIcon :component="RefreshCw" /></template>刷新</NButton></template>
						<div v-if="runs.length" class="audit-list" data-testid="subscription-runs">
							<div v-for="run in runs" :key="run.id">
								<span><strong>{{ run.trigger }} · {{ run.subscription_id || run.site_id || '批量' }}</strong><small>{{ formatTime(run.started_at) }}</small></span>
								<NTag :type="statusType(run.status)">{{ run.status }}</NTag>
								<small>发送 {{ run.sent }} · 已有 {{ run.exists }} · 跳过 {{ run.skipped }} · 失败 {{ run.failed }}</small>
							</div>
						</div>
						<NEmpty v-else description="暂无订阅运行记录" />
					</NCard>
				</NTabPane>

				<NTabPane name="batch" tab="批量下载">
					<div class="batch-grid">
						<NCard title="选择数据库种子" size="small">
							<NSelect v-model:value="selectedTorrentKeys" multiple filterable clearable :options="torrentOptions" placeholder="也可先在规则预览中多选" data-testid="batch-torrents" />
							<p class="muted batch-hint">已选择 {{ selectedTorrentKeys.length }} 项。预览不会写入任务或修改 qB，仅执行只读校验。</p>
						</NCard>
						<NCard title="下载配置来源" size="small">
							<NRadioGroup v-model:value="batchMode" name="batch-mode">
								<NRadioButton value="subscription">快速套用订阅</NRadioButton><NRadioButton value="custom">临时配置</NRadioButton>
							</NRadioGroup>
							<NForm v-if="batchMode === 'subscription'" label-placement="top"><NFormItem label="订阅"><NSelect v-model:value="batchSubscriptionID" :options="subscriptionOptions" /></NFormItem></NForm>
							<NForm v-else label-placement="top" class="compact-form">
								<div class="form-grid form-grid-2">
									<NFormItem label="qB 分类"><NSelect v-model:value="batchOptions.qb_category" clearable filterable tag :options="categoryOptions" /></NFormItem>
									<NFormItem label="保存路径模板"><NInput v-model:value="batchOptions.save_path_template" /></NFormItem>
									<NFormItem label="qB 标签"><NDynamicTags v-model:value="batchOptions.qb_tags" /></NFormItem>
									<NFormItem label="任务名称模板"><NInput v-model:value="batchOptions.filename_template" /></NFormItem>
									<NFormItem label="添加后暂停"><NSwitch v-model:value="batchOptions.paused" /></NFormItem>
								</div>
							</NForm>
							<NSpace justify="end">
								<NButton :loading="action === 'preview-batch'" data-testid="preview-batch" @click="previewBatch"><template #icon><NIcon :component="Eye" /></template>预览计划</NButton>
								<NPopconfirm @positive-click="executeBatch">
									<template #trigger><NButton type="primary" :loading="action === 'execute-batch'" data-testid="execute-batch"><template #icon><NIcon :component="CloudDownload" /></template>确认执行</NButton></template>
									将按当前选择重新校验并发送到 qB，确定继续？
								</NPopconfirm>
							</NSpace>
						</NCard>
					</div>

					<NCard v-if="batchPreview" title="批量计划预览" size="small" data-testid="batch-preview-results">
						<div class="plan-list">
							<article v-for="item in batchPreview.items" :key="torrentKey(item.torrent.site_id, item.torrent.id)" class="plan-item">
								<header><strong>{{ item.torrent.title }}</strong><NTag :type="item.eligible ? 'success' : 'warning'">{{ item.eligible ? '可发送' : '阻塞' }}</NTag></header>
								<div class="plan-grid"><span>分类：{{ item.plan.category || '默认' }}</span><span>路径：{{ item.plan.save_path || '默认' }}</span><span>名称：{{ item.plan.rename || item.plan.original_name || '原始名称' }}</span><span>标签：{{ item.plan.tags.join(', ') || '无' }}</span></div>
								<p class="muted">{{ reasonText(item) }}</p>
							</article>
						</div>
					</NCard>
					<NAlert v-if="batchResult" :type="batchResult.failed ? 'warning' : 'success'" data-testid="batch-result">
						<template #icon><NIcon :component="CheckCircle2" /></template>
						已尝试 {{ batchResult.attempted }}，发送 {{ batchResult.sent }}，已存在 {{ batchResult.exists }}，跳过 {{ batchResult.skipped }}，失败 {{ batchResult.failed }}。
					</NAlert>
				</NTabPane>
			</NTabs>
		</NSpin>
	</section>
</template>
