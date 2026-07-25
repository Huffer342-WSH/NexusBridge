<!-- 文件管理器浏览本机目录，并为丢失 qB 任务提供只读预览和显式恢复。 -->
<script setup lang="ts">
import {
	ArrowLeft,
	ArrowRight,
	ChevronUp,
	File,
	Folder,
	FolderOpen,
	MoreHorizontal,
	RefreshCw,
	RotateCcw,
	Search,
} from '@lucide/vue';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import {
	NAlert,
	NButton,
	NCard,
	NDropdown,
	NEmpty,
	NForm,
	NFormItem,
	NIcon,
	NInput,
	NInputNumber,
	NModal,
	NRadioButton,
	NRadioGroup,
	NSelect,
	NSpace,
	NSpin,
	NSwitch,
	NTag,
} from 'naive-ui';
import { useRoute, useRouter } from 'vue-router';
import { api } from '../api';
import type {
	FileBrowseResult,
	FileEntry,
	QBCategory,
	RecoveryMatch,
	RecoveryBatchItemResult,
	RecoveryBatchResult,
	RecoveryPreview,
	RecoveryResult,
	RecoveryScanItem,
	RecoveryScanResult,
	RecoverySearchMode,
	Site,
	TorrentSizeIndexStatus,
} from '../types';

const props = defineProps<{ sites: Site[] }>();
const emit = defineEmits<{ message: [value: string] }>();

type RecoveryMode = Exclude<RecoverySearchMode, 'url'> | 'url';

const route = useRoute();
const router = useRouter();
const browser = ref<FileBrowseResult>({ path: '', is_root: true, qb_connected: false, entries: [] });
const pathDraft = ref('');
const browsing = ref(false);
const browseError = ref('');
const menuVisible = ref(false);
const menuX = ref(0);
const menuY = ref(0);
const contextEntry = ref<FileEntry | null>(null);
const backHistory = ref<string[]>([]);
const forwardHistory = ref<string[]>([]);
const consumedNavigationButtons = new Set<number>();
let pendingRoutePath: string | undefined;
let routePathInitialized = false;

const recoveryOpen = ref(false);
const recoveryTarget = ref<FileEntry | null>(null);
const recoveryMode = ref<RecoveryMode>('database_then_site');
const recoverySiteID = ref<string | null>(null);
const torrentURL = ref('');
const recoveryCategory = ref<string | null>(null);
const categories = ref<QBCategory[]>([]);
const recoveryPreview = ref<RecoveryPreview | null>(null);
const selectedMatchKey = ref('');
const previewing = ref(false);
const recovering = ref(false);
const recoveryActioning = ref(false);
const recoveryError = ref('');
const recoveryResult = ref<RecoveryResult | null>(null);

const scanOpen = ref(false);
const scanning = ref(false);
const scanDepth = ref(1);
const scanWebSearch = ref(false);
const scanUsedWebSearch = ref(false);
const scanResult = ref<RecoveryScanResult | null>(null);
const scanError = ref('');
const batchRecovering = ref(false);
const batchRecoveryResult = ref<RecoveryBatchResult | null>(null);
const batchRecoveryError = ref('');
const batchRecoveryActioning = ref('');
const sizeIndex = ref<TorrentSizeIndexStatus | null>(null);
const sizeIndexLoading = ref(false);
const sizeIndexError = ref('');

const siteOptions = computed(() => props.sites.map((site) => ({ label: site.name, value: site.id })));
const categoryOptions = computed(() => [
	{ label: '自动判断 / qB 默认分类', value: '' },
	...categories.value.map((category) => ({
		label: category.save_path ? `${category.name} · ${category.save_path}` : category.name,
		value: category.name,
	})),
]);
const matchOptions = computed(() => recoveryPreview.value?.matches ?? []);
const selectedMatch = computed<RecoveryMatch | null>(() => {
	if (!recoveryPreview.value) return null;
	if (selectedMatchKey.value) {
		return recoveryPreview.value.matches.find((match) => matchKey(match) === selectedMatchKey.value) ?? null;
	}
	return recoveryPreview.value.matches.length === 1 ? recoveryPreview.value.matches[0] : null;
});
const scanCandidates = computed(() => scanResult.value?.items.filter((item) => item.status !== 'none') ?? []);
const matchedScanItems = computed(() => scanResult.value?.items.filter((item) => item.status === 'matched' && item.preview.matches.length === 1) ?? []);
const canGoBack = computed(() => backHistory.value.length > 0 && !browsing.value);
const canGoForward = computed(() => forwardHistory.value.length > 0 && !browsing.value);
const sizeIndexReady = computed(() => !!sizeIndex.value && sizeIndex.value.pending === 0);
const recoveryNeedsIndex = computed(() => recoveryMode.value === 'database' || recoveryMode.value === 'database_then_site');

const recoveryModeOptions = [
	{ label: '先数据库，再搜索站点', value: 'database_then_site' },
	{ label: '仅本地数据库', value: 'database' },
	{ label: '仅站点网页', value: 'site' },
];

function matchKey(match: RecoveryMatch) {
	return `${match.torrent.site_id}:${match.torrent.id}`;
}

function formatBytes(value = 0) {
	if (!value) return '—';
	const units = ['B', 'KB', 'MB', 'GB', 'TB'];
	let size = value;
	let unit = 0;
	while (size >= 1024 && unit < units.length - 1) {
		size /= 1024;
		unit++;
	}
	return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function formatDate(value?: string) {
	return value ? new Date(value).toLocaleString() : '—';
}

function routePath() {
	const value = route.query.path;
	return String(Array.isArray(value) ? value[0] ?? '' : value ?? '').trim();
}

function replacePathURL(currentPath: string) {
	if (route.name !== 'files') return;
	const query = currentPath ? { path: currentPath } : {};
	const target = { name: 'files', query };
	if (router.resolve(target).fullPath !== route.fullPath) {
		void router.replace(target);
	}
}

async function browse(path: string, recordHistory = true) {
	if (browsing.value) return false;
	browsing.value = true;
	browseError.value = '';
	try {
		const previousPath = browser.value.path;
		const result = await api.browseFiles(path.trim());
		browser.value = result;
		pathDraft.value = result.path;
		if (pendingRoutePath === undefined) replacePathURL(result.path);
		if (pendingRoutePath === undefined && recordHistory && previousPath && previousPath !== result.path) {
			backHistory.value.push(previousPath);
			forwardHistory.value = [];
		}
		return true;
	} catch (error) {
		browseError.value = error instanceof Error ? error.message : '无法读取目录';
		return false;
	} finally {
		browsing.value = false;
		if (pendingRoutePath !== undefined) {
			const target = pendingRoutePath;
			pendingRoutePath = undefined;
			if (target !== browser.value.path) void browse(target, false);
		}
	}
}

watch(() => route.query.path, () => {
	if (route.name !== 'files') return;
	const target = routePath();
	if (!routePathInitialized) {
		routePathInitialized = true;
		void browse(target, false);
		return;
	}
	if (target === browser.value.path) return;
	if (browsing.value) {
		pendingRoutePath = target;
		return;
	}
	void browse(target, false);
}, { immediate: true });

async function goBack() {
	const target = backHistory.value.at(-1);
	if (!target || browsing.value) return;
	const current = browser.value.path;
	if (await browse(target, false)) {
		backHistory.value.pop();
		if (current && current !== target) forwardHistory.value.push(current);
	}
}

async function goForward() {
	const target = forwardHistory.value.at(-1);
	if (!target || browsing.value) return;
	const current = browser.value.path;
	if (await browse(target, false)) {
		forwardHistory.value.pop();
		if (current && current !== target) backHistory.value.push(current);
	}
}

function handleNavigationMouseButton(event: MouseEvent) {
	if (event.button !== 3 && event.button !== 4) return;
	if (event.type === 'mousedown') {
		consumedNavigationButtons.delete(event.button);
		const hasInternalTarget = event.button === 3
			? backHistory.value.length > 0
			: forwardHistory.value.length > 0;
		if (!hasInternalTarget) return;
		consumedNavigationButtons.add(event.button);
	}
	if (!consumedNavigationButtons.has(event.button)) return;
	event.preventDefault();
	event.stopPropagation();
	if (event.type !== 'mousedown') return;
	if (event.button === 3) void goBack();
	else void goForward();
}

function activateEntry(entry: FileEntry) {
	if (entry.is_dir) void browse(entry.path);
}

function showContextMenu(event: MouseEvent, entry: FileEntry) {
	event.preventDefault();
	contextEntry.value = entry;
	menuX.value = event.clientX;
	menuY.value = event.clientY;
	menuVisible.value = false;
	requestAnimationFrame(() => (menuVisible.value = true));
}

function handleContextAction(key: string) {
	menuVisible.value = false;
	const entry = contextEntry.value;
	if (!entry) return;
	if (key === 'open' && entry.is_dir) {
		void browse(entry.path);
	} else if (key === 'recover') {
		void openRecovery(entry);
	}
}

async function loadCategories() {
	if (categories.value.length) return;
	try {
		categories.value = (await api.getQBCategories()).items;
	} catch {
		categories.value = [];
	}
}

async function loadSizeIndex() {
	sizeIndexError.value = '';
	try {
		sizeIndex.value = await api.getTorrentSizeIndexStatus();
	} catch (error) {
		sizeIndexError.value = error instanceof Error ? error.message : '无法读取 torrent 大小索引状态';
	}
}

async function rebuildSizeIndex() {
	if (sizeIndexLoading.value) return;
	sizeIndexLoading.value = true;
	sizeIndexError.value = '';
	try {
		sizeIndex.value = await api.rebuildTorrentSizeIndex();
		emit('message', `大小索引重建完成：${sizeIndex.value.indexed}/${sizeIndex.value.total}`);
	} catch (error) {
		sizeIndexError.value = error instanceof Error ? error.message : '重建 torrent 大小索引失败';
	} finally {
		sizeIndexLoading.value = false;
	}
}

async function openRecovery(entry: FileEntry, existingPreview?: RecoveryPreview) {
	recoveryTarget.value = entry;
	recoveryMode.value = existingPreview?.search_mode === 'database' ? 'database' : 'database_then_site';
	recoverySiteID.value = null;
	torrentURL.value = '';
	recoveryCategory.value = existingPreview?.category ?? null;
	recoveryPreview.value = existingPreview ?? null;
	selectedMatchKey.value = existingPreview?.matches.length === 1 ? matchKey(existingPreview.matches[0]) : '';
	recoveryError.value = '';
	recoveryResult.value = null;
	recoveryOpen.value = true;
	await loadCategories();
}

async function previewRecovery() {
	if (!recoveryTarget.value) return;
	if (recoveryNeedsIndex.value && !sizeIndexReady.value) {
		recoveryError.value = '数据库大小索引尚未就绪，请先手动重建一次；仅站点网页和直接 URL 不受影响。';
		return;
	}
	previewing.value = true;
	recoveryError.value = '';
	recoveryResult.value = null;
	recoveryPreview.value = null;
	selectedMatchKey.value = '';
	try {
		const siteIDs = recoverySiteID.value ? [recoverySiteID.value] : undefined;
		const preview = await api.previewRecovery({
			path: recoveryTarget.value.path,
			site_ids: siteIDs,
			search_mode: recoveryMode.value === 'url' ? undefined : recoveryMode.value,
			torrent_url: recoveryMode.value === 'url' ? torrentURL.value.trim() : undefined,
			category: recoveryCategory.value || undefined,
		});
		recoveryPreview.value = preview;
		recoveryCategory.value = preview.category ?? recoveryCategory.value;
		if (preview.matches.length === 1) selectedMatchKey.value = matchKey(preview.matches[0]);
	} catch (error) {
		recoveryError.value = error instanceof Error ? error.message : '恢复预览失败';
	} finally {
		previewing.value = false;
	}
}

async function recover() {
	if (!recoveryTarget.value || !selectedMatch.value) return;
	recovering.value = true;
	recoveryError.value = '';
	try {
		const match = selectedMatch.value;
		const result = await api.recoverFolder({
			path: recoveryTarget.value.path,
			site_id: recoveryMode.value === 'url' ? recoverySiteID.value ?? undefined : match.torrent.site_id,
			torrent_id: recoveryMode.value === 'url' ? undefined : match.torrent.id,
			search_mode: recoveryMode.value === 'url' ? undefined : recoveryMode.value,
			torrent_url: recoveryMode.value === 'url' ? torrentURL.value.trim() : undefined,
			category: recoveryCategory.value || undefined,
		});
		recoveryResult.value = result;
		if (result.started) {
			emit('message', `校验通过并已开始做种：${result.match.original_name}，分类 ${result.category || '默认'}`);
			recoveryOpen.value = false;
			await browse(browser.value.path);
		} else {
			recoveryError.value = result.verification_error || '强制校验未确认完成，任务已保持暂停';
			await browse(browser.value.path);
		}
	} catch (error) {
		recoveryError.value = error instanceof Error ? error.message : '恢复失败';
	} finally {
		recovering.value = false;
	}
}

async function controlFailedRecovery(action: 'start' | 'delete') {
	const hash = recoveryResult.value?.qb_status.hash;
	if (!hash || recoveryActioning.value) return;
	recoveryActioning.value = true;
	try {
		await api.controlRecoveryTorrent(hash, action);
		emit('message', action === 'start' ? `已按用户选择启动任务 ${hash}` : `已删除任务 ${hash}，文件保持不变`);
		recoveryOpen.value = false;
		await browse(browser.value.path);
	} catch (error) {
		recoveryError.value = error instanceof Error ? error.message : '处理校验失败任务失败';
	} finally {
		recoveryActioning.value = false;
	}
}

async function scanCurrentDirectory() {
	if (!browser.value.path) return;
	if (!sizeIndexReady.value) {
		scanOpen.value = true;
		scanError.value = '数据库大小索引尚未就绪，请先手动重建一次。';
		return;
	}
	scanOpen.value = true;
	scanning.value = true;
	scanError.value = '';
	batchRecoveryError.value = '';
	batchRecoveryResult.value = null;
	scanResult.value = null;
	try {
		scanResult.value = await api.scanRecoveryCandidates({
			path: browser.value.path,
			search_mode: scanWebSearch.value ? 'database_then_site' : 'database',
			max_depth: scanDepth.value,
			limit: 200,
		});
		scanUsedWebSearch.value = scanWebSearch.value;
	} catch (error) {
		scanError.value = error instanceof Error ? error.message : '目录扫描失败';
	} finally {
		scanning.value = false;
	}
}

async function recoverMatchedScanItems() {
	if (!matchedScanItems.value.length || batchRecovering.value) return;
	batchRecovering.value = true;
	batchRecoveryError.value = '';
	batchRecoveryResult.value = null;
	try {
		batchRecoveryResult.value = await api.recoverFolders({
			web_search: scanUsedWebSearch.value,
			items: matchedScanItems.value.map((item) => {
				const match = item.preview.matches[0];
				return {
					path: item.path,
					site_id: match.torrent.site_id,
					torrent_id: match.torrent.id,
					category: item.preview.category || undefined,
				};
			}),
		});
		const result = batchRecoveryResult.value;
		emit('message', `批量恢复完成：成功 ${result.recovered}，需处理 ${result.needs_attention}，失败 ${result.failed}，跳过 ${result.skipped}`);
		await browse(browser.value.path, false);
	} catch (error) {
		batchRecoveryError.value = error instanceof Error ? error.message : '批量恢复失败';
	} finally {
		batchRecovering.value = false;
	}
}

async function controlBatchRecovery(item: RecoveryBatchItemResult, action: 'start' | 'delete') {
	const hash = item.result?.qb_status.hash;
	if (!hash || batchRecoveryActioning.value) return;
	batchRecoveryActioning.value = `${hash}:${action}`;
	try {
		await api.controlRecoveryTorrent(hash, action);
		if (item.result) {
			item.result.can_start = false;
			item.result.can_delete = false;
		}
		item.error = action === 'start' ? '已按用户选择启动任务' : '已删除 qB 任务，磁盘文件保持不变';
		emit('message', item.error);
		await browse(browser.value.path, false);
	} catch (error) {
		batchRecoveryError.value = error instanceof Error ? error.message : '处理批量恢复任务失败';
	} finally {
		batchRecoveryActioning.value = '';
	}
}

function recoverScanItem(item: RecoveryScanItem) {
	scanOpen.value = false;
	void openRecovery({
		name: item.name,
		path: item.path,
		is_dir: item.is_dir,
		qb_tasks: [],
	}, item.preview);
}

onMounted(() => {
	window.addEventListener('mousedown', handleNavigationMouseButton, true);
	window.addEventListener('mouseup', handleNavigationMouseButton, true);
	window.addEventListener('auxclick', handleNavigationMouseButton, true);
	void loadSizeIndex();
});

onBeforeUnmount(() => {
	window.removeEventListener('mousedown', handleNavigationMouseButton, true);
	window.removeEventListener('mouseup', handleNavigationMouseButton, true);
	window.removeEventListener('auxclick', handleNavigationMouseButton, true);
});
</script>

<template>
	<section class="file-manager-page">
		<div class="file-toolbar">
			<NButton class="file-back-button" aria-label="返回" title="返回（鼠标后退侧键）" :disabled="!canGoBack" data-testid="file-back" @click="goBack">
				<template #icon><NIcon :component="ArrowLeft" /></template>
			</NButton>
			<NButton class="file-forward-button" aria-label="前进" title="前进（鼠标前进侧键）" :disabled="!canGoForward" data-testid="file-forward" @click="goForward">
				<template #icon><NIcon :component="ArrowRight" /></template>
			</NButton>
			<NButton class="file-up-button" :disabled="!browser.parent || browsing" @click="browse(browser.parent || '')">
				<template #icon><NIcon :component="ChevronUp" /></template>
				上一级
			</NButton>
			<NInput v-model:value="pathDraft" class="file-path-input" placeholder="输入本机完整目录路径" @keyup.enter="browse(pathDraft)" />
			<NButton class="file-open-button" type="primary" :loading="browsing" @click="browse(pathDraft)">打开</NButton>
			<NButton class="file-refresh-button" :loading="browsing" aria-label="刷新" @click="browse(browser.path, false)">
				<template #icon><NIcon :component="RefreshCw" /></template>
			</NButton>
			<NButton class="file-scan-button" :disabled="!browser.path" @click="scanCurrentDirectory">
				<template #icon><NIcon :component="Search" /></template>
				扫描丢失任务
			</NButton>
		</div>

		<NAlert v-if="sizeIndexError" type="error" :bordered="false">
			{{ sizeIndexError }}
		</NAlert>
		<NAlert v-else-if="sizeIndex" :type="sizeIndexReady && !sizeIndex.failed ? 'success' : 'warning'" :bordered="false" class="size-index-alert">
			<NSpace align="center" justify="space-between">
				<span>
					Torrent 大小索引 {{ sizeIndex.indexed }}/{{ sizeIndex.total }}
					<span v-if="sizeIndex.pending">，还有 {{ sizeIndex.pending }} 个待处理</span>
					<span v-if="sizeIndex.failed">，其中 {{ sizeIndex.failed }} 个解析失败</span>。
					旧数据只需手动重建一次，之后保存和删除 torrent 时会自动维护。
				</span>
				<NButton size="small" :loading="sizeIndexLoading" @click="rebuildSizeIndex">手动重建</NButton>
			</NSpace>
		</NAlert>

		<NAlert v-if="browseError" type="error" :bordered="false">{{ browseError }}</NAlert>
		<NAlert v-else-if="browser.qb_error" type="warning" :bordered="false">
			文件仍可浏览，但暂时无法标记 qB 任务：{{ browser.qb_error }}
		</NAlert>

		<NCard :bordered="false" class="file-card">
			<div class="file-header file-row">
				<span>名称</span><span>大小</span><span>修改时间</span><span>qB 归属</span><span></span>
			</div>
			<NSpin :show="browsing">
				<NEmpty v-if="!browser.entries.length" description="此位置没有可显示的条目" />
				<div
					v-for="entry in browser.entries"
					:key="entry.path"
					class="file-row file-entry"
					:data-testid="`file-entry-${entry.name}`"
					@dblclick="activateEntry(entry)"
					@contextmenu="showContextMenu($event, entry)"
				>
					<div class="file-name">
						<NIcon :component="entry.is_dir ? Folder : File" :class="entry.is_dir ? 'folder-icon' : 'file-icon'" />
						<span :title="entry.path">{{ entry.name }}</span>
					</div>
					<span>{{ entry.is_dir ? '—' : formatBytes(entry.size) }}</span>
					<span>{{ formatDate(entry.modified_at) }}</span>
					<div class="qb-badges">
						<NTag v-if="!entry.qb_tasks.length" size="small" :bordered="false">未关联</NTag>
						<NTag v-for="task in entry.qb_tasks" :key="task.hash" size="small" type="success">
							{{ task.category || '无分类' }} · {{ task.state }}
						</NTag>
					</div>
					<NButton quaternary circle aria-label="更多操作" @click="showContextMenu($event, entry)">
						<template #icon><NIcon :component="MoreHorizontal" /></template>
					</NButton>
				</div>
			</NSpin>
		</NCard>

		<NDropdown
			trigger="manual"
			:show="menuVisible"
			:x="menuX"
			:y="menuY"
			:options="[
				{ label: '打开目录', key: 'open', disabled: !contextEntry?.is_dir },
				{ label: '尝试恢复 qB 任务', key: 'recover', disabled: !!contextEntry?.qb_tasks.length },
			]"
			@clickoutside="menuVisible = false"
			@select="handleContextAction"
		/>

		<NModal v-model:show="recoveryOpen" preset="card" title="尝试恢复 qB 任务" class="recovery-modal">
			<NAlert type="info" :bordered="false" class="modal-alert">
				目标：{{ recoveryTarget?.path }}。预览只读取目录和 torrent；点击“确认恢复”后才会写入 qB。
			</NAlert>
			<NAlert v-if="recoveryError" type="error" :bordered="false" class="modal-alert">{{ recoveryError }}</NAlert>
			<NAlert v-if="recoveryResult && !recoveryResult.started" type="warning" :bordered="false" class="modal-alert">
				任务只添加了一次并保持暂停。强制校验尝试 {{ recoveryResult.recheck_attempts }} 次，当前状态 {{ recoveryResult.qb_status.state }}；可选择仍然启动，或仅删除 qB 任务并保留文件。
			</NAlert>
			<NForm label-placement="top">
				<NFormItem label="候选来源">
					<NRadioGroup v-model:value="recoveryMode">
						<NRadioButton v-for="option in recoveryModeOptions" :key="option.value" :value="option.value">{{ option.label }}</NRadioButton>
						<NRadioButton value="url">直接 torrent URL</NRadioButton>
					</NRadioGroup>
				</NFormItem>
				<NFormItem label="站点（可选；留空时遍历全部站点或按 URL 自动识别）">
					<NSelect v-model:value="recoverySiteID" clearable filterable :options="siteOptions" />
				</NFormItem>
				<NFormItem v-if="recoveryMode === 'url'" label="Torrent 下载 URL">
					<NInput v-model:value="torrentURL" placeholder="https://tracker.example/download.php?id=..." />
				</NFormItem>
				<NFormItem label="qB 分类">
					<NSelect v-model:value="recoveryCategory" clearable filterable :options="categoryOptions" />
				</NFormItem>
			</NForm>

			<NSpace>
				<NButton type="primary" :loading="previewing" :disabled="(recoveryMode === 'url' && !torrentURL.trim()) || (recoveryNeedsIndex && !sizeIndexReady)" data-testid="preview-recovery" @click="previewRecovery">
					<template #icon><NIcon :component="Search" /></template>
					预览匹配
				</NButton>
				<NTag v-if="recoveryPreview" :type="recoveryPreview.matches.length === 1 ? 'success' : recoveryPreview.matches.length ? 'warning' : 'default'">
					可恢复候选 {{ recoveryPreview.matches.length }} 个
				</NTag>
			</NSpace>

			<div v-if="matchOptions.length" class="match-list">
				<label v-for="match in matchOptions" :key="matchKey(match)" class="match-option">
					<input v-model="selectedMatchKey" type="radio" :value="matchKey(match)" />
					<span><strong>{{ match.original_name }}</strong><small>{{ match.torrent.site_id }}/{{ match.torrent.id }} · {{ match.file_count }} 文件 · {{ match.source }}</small></span>
				</label>
			</div>

			<template #footer>
				<NSpace justify="end">
					<NButton v-if="recoveryResult?.can_delete" type="error" secondary :loading="recoveryActioning" @click="controlFailedRecovery('delete')">删除任务，保留文件</NButton>
					<NButton v-if="recoveryResult?.can_start" type="warning" :loading="recoveryActioning" @click="controlFailedRecovery('start')">仍然开始任务</NButton>
					<NButton :disabled="recovering" @click="recoveryOpen = false">取消</NButton>
					<NButton v-if="!recoveryResult" type="primary" :loading="recovering" :disabled="!selectedMatch" data-testid="confirm-recovery" @click="recover">
						<template #icon><NIcon :component="RotateCcw" /></template>
						确认恢复
					</NButton>
				</NSpace>
			</template>
		</NModal>

		<NModal v-model:show="scanOpen" preset="card" title="扫描可能丢失的任务" class="scan-modal">
			<div class="scan-toolbar">
				<span>递归深度</span>
				<NInputNumber v-model:value="scanDepth" :min="1" :max="5" />
				<span class="scan-web-option">
					<NSwitch v-model:value="scanWebSearch" :disabled="scanning || batchRecovering" data-testid="scan-web-search" />
					启用网页搜索
				</span>
				<NButton :loading="scanning" @click="scanCurrentDirectory">重新扫描</NButton>
			</div>
			<NAlert v-if="scanWebSearch" type="warning" :bordered="false" class="modal-alert">
				数据库未匹配的条目会继续搜索已配置站点，目录较大时可能产生较多网页请求。
			</NAlert>
			<NAlert v-if="scanError" type="error" :bordered="false">{{ scanError }}</NAlert>
			<NAlert v-if="batchRecoveryError" type="error" :bordered="false">{{ batchRecoveryError }}</NAlert>
			<NSpin :show="scanning">
				<p v-if="scanResult" class="scan-summary">
					已评估 {{ scanResult.evaluated }} 项，唯一匹配 {{ scanResult.matched }} 项<span v-if="scanResult.truncated">，已达到上限</span>。
				</p>
				<NSpace v-if="scanResult && !batchRecoveryResult" align="center" class="scan-batch-actions">
					<NButton type="primary" :loading="batchRecovering" :disabled="!matchedScanItems.length" data-testid="recover-scan-batch" @click="recoverMatchedScanItems">
						<template #icon><NIcon :component="RotateCcw" /></template>
						自动恢复 {{ matchedScanItems.length }} 项
					</NButton>
					<span>点击后串行执行全部唯一候选，不再逐项确认。</span>
				</NSpace>
				<NAlert v-if="batchRecoveryResult" :type="batchRecoveryResult.failed || batchRecoveryResult.needs_attention ? 'warning' : 'success'" :bordered="false" class="modal-alert">
					批量恢复完成：成功 {{ batchRecoveryResult.recovered }}，需处理 {{ batchRecoveryResult.needs_attention }}，失败 {{ batchRecoveryResult.failed }}，跳过 {{ batchRecoveryResult.skipped }}。
				</NAlert>
				<div v-if="batchRecoveryResult" class="batch-result-list">
					<div v-for="item in batchRecoveryResult.items" :key="`${item.path}:${item.site_id}:${item.torrent_id}`" class="batch-result-item">
						<span><strong>{{ item.path }}</strong><small>{{ item.error || `${item.site_id}/${item.torrent_id}` }}</small></span>
						<NTag :type="item.status === 'recovered' ? 'success' : item.status === 'needs_attention' ? 'warning' : 'error'">{{ item.status }}</NTag>
						<NSpace v-if="item.result?.can_start || item.result?.can_delete" size="small">
							<NButton v-if="item.result.can_delete" size="tiny" type="error" secondary :loading="batchRecoveryActioning === `${item.result.qb_status.hash}:delete`" @click="controlBatchRecovery(item, 'delete')">删除任务</NButton>
							<NButton v-if="item.result.can_start" size="tiny" type="warning" :loading="batchRecoveryActioning === `${item.result.qb_status.hash}:start`" @click="controlBatchRecovery(item, 'start')">仍然开始</NButton>
						</NSpace>
					</div>
				</div>
				<NEmpty v-if="scanResult && !scanCandidates.length" description="未发现可恢复候选" />
				<div class="scan-list">
					<button v-for="item in scanCandidates" :key="item.path" type="button" class="scan-item" @click="recoverScanItem(item)">
						<NIcon :component="item.is_dir ? FolderOpen : File" />
						<span><strong>{{ item.name }}</strong><small>{{ item.path }}</small></span>
						<NTag :type="item.status === 'matched' ? 'success' : item.status === 'ambiguous' ? 'warning' : 'error'">{{ item.status }}</NTag>
					</button>
				</div>
			</NSpin>
		</NModal>
	</section>
</template>

<style scoped>
.file-manager-page { display: grid; gap: 14px; }
.file-toolbar { display: grid; grid-template-areas: "back forward up path open refresh scan"; grid-template-columns: auto auto auto minmax(220px, 1fr) auto auto auto; gap: 8px; align-items: center; }
.file-back-button { grid-area: back; }
.file-forward-button { grid-area: forward; }
.file-up-button { grid-area: up; }
.file-path-input { grid-area: path; }
.file-open-button { grid-area: open; }
.file-refresh-button { grid-area: refresh; }
.file-scan-button { grid-area: scan; }
.file-card { overflow: hidden; }
.file-row { display: grid; grid-template-columns: minmax(260px, 2fr) 110px 180px minmax(200px, 1fr) 42px; gap: 12px; align-items: center; min-height: 48px; padding: 0 12px; }
.file-header { color: #64748b; font-size: 12px; font-weight: 700; border-bottom: 1px solid #e2e8f0; }
.file-entry { border-bottom: 1px solid #eef2f7; cursor: default; transition: background .15s ease; }
.file-entry:hover { background: #f8fafc; }
.file-name { display: flex; min-width: 0; align-items: center; gap: 10px; font-weight: 600; }
.file-name span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.folder-icon { color: #f59e0b; }
.file-icon { color: #64748b; }
.qb-badges { display: flex; flex-wrap: wrap; gap: 6px; }
.recovery-modal { width: min(760px, calc(100vw - 32px)); }
.scan-modal { width: min(860px, calc(100vw - 32px)); }
.modal-alert { margin-bottom: 14px; }
.size-index-alert :deep(.n-alert-body__content) { width: 100%; }
.match-list, .scan-list { display: grid; gap: 8px; margin-top: 16px; }
.match-option { display: flex; gap: 10px; align-items: flex-start; padding: 12px; border: 1px solid #e2e8f0; border-radius: 8px; cursor: pointer; }
.match-option span, .scan-item span { display: grid; min-width: 0; flex: 1; text-align: left; }
.match-option small, .scan-item small { color: #64748b; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.scan-toolbar { display: flex; gap: 10px; align-items: center; margin-bottom: 12px; }
.scan-web-option { display: inline-flex; gap: 8px; align-items: center; }
.scan-summary { color: #475569; }
.scan-batch-actions { margin-bottom: 12px; color: #64748b; }
.scan-item { display: flex; width: 100%; gap: 10px; align-items: center; padding: 12px; background: #fff; border: 1px solid #e2e8f0; border-radius: 8px; cursor: pointer; }
.scan-item:hover { border-color: #2563eb; background: #f8fafc; }
.batch-result-list { display: grid; gap: 8px; margin-bottom: 14px; }
.batch-result-item { display: flex; gap: 10px; align-items: center; padding: 10px 12px; border: 1px solid #e2e8f0; border-radius: 8px; }
.batch-result-item > span { display: grid; min-width: 0; flex: 1; }
.batch-result-item small { color: #64748b; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
@media (max-width: 900px) {
	.file-toolbar { grid-template-areas: "back forward up refresh refresh" "path path path path open" "scan scan scan scan scan"; grid-template-columns: auto auto auto minmax(0, 1fr) auto; }
	.file-header { display: none; }
	.file-row { grid-template-columns: minmax(0, 1fr) auto; gap: 4px 10px; padding: 10px; }
	.file-row > :nth-child(2), .file-row > :nth-child(3) { display: none; }
	.qb-badges { grid-column: 1; }
}
</style>
