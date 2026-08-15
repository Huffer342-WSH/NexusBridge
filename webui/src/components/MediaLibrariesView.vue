<!-- 媒体库视图管理集合与剧集型节点组成的目录树。 -->
<script setup lang="ts">
import {
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Edit3,
  Film,
  FolderOpen,
  FolderTree,
  Plus,
  RefreshCw,
  Trash2,
  X,
} from '@lucide/vue';
import { computed, onMounted, ref } from 'vue';
import {
  NAlert,
  NButton,
  NCard,
  NEmpty,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NPopconfirm,
  NPopover,
  NSelect,
  NSpace,
  NSpin,
} from 'naive-ui';
import { useRouter } from 'vue-router';
import { api } from '../api';
import type {
  MediaLibraryDetail,
  MediaLibraryKind,
  MediaLibrarySaveRequest,
  MediaLibrarySettingsOverride,
  MediaLibrarySummary,
} from '../types';
import FilePickerDialog from './FilePickerDialog.vue';
import ResizableModal from './ResizableModal.vue';

type SettingChoice = 'inherit' | 'enabled' | 'disabled';
type LibraryRow = MediaLibrarySummary & { depth: number };

const router = useRouter();
const items = ref<MediaLibrarySummary[]>([]);
const loading = ref(true);
const error = ref('');
const action = ref('');
const editorOpen = ref(false);
const editorLoading = ref(false);
const editorError = ref('');
const editingID = ref('');
const editingKind = ref<MediaLibraryKind>('collection');
const pickerOpen = ref(false);
const pickerIndex = ref(0);
const draft = ref<MediaLibrarySaveRequest>(emptyDraft());
const episodeSetting = ref<SettingChoice>('inherit');
const autoDetectSetting = ref<SettingChoice>('inherit');
const expandedIDs = ref(new Set<string>());

const kindOptions = [
  { label: '普通媒体库', value: 'collection' },
  { label: '剧集', value: 'series' },
];
const settingOptions = [
  { label: '继承父媒体库', value: 'inherit' },
  { label: '启用', value: 'enabled' },
  { label: '关闭', value: 'disabled' },
];
const parentOptions = computed(() => {
  const excluded = descendantIDs(editingID.value);
  if (editingID.value) excluded.add(editingID.value);
  return items.value
    .filter((item) => item.kind !== 'series' && !excluded.has(item.id))
    .map((item) => ({ label: item.name, value: item.id }));
});
const rows = computed<LibraryRow[]>(() => {
  const expanded = expandedIDs.value;
  const children = new Map<string, MediaLibrarySummary[]>();
  const known = new Set(items.value.map((item) => item.id));
  for (const item of items.value) {
    const parent = item.parent_id && known.has(item.parent_id) ? item.parent_id : '';
    children.set(parent, [...(children.get(parent) ?? []), item]);
  }
  for (const group of children.values()) {
    group.sort((left, right) => left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }));
  }
  const result: LibraryRow[] = [];
  const append = (parentID: string, depth: number) => {
    for (const item of children.get(parentID) ?? []) {
      result.push({ ...item, depth });
      if (expanded.has(item.id)) append(item.id, depth + 1);
    }
  };
  append('', 0);
  return result;
});

function emptyDraft(): MediaLibrarySaveRequest {
  return { name: '', kind: 'collection', parent_id: '', directories: [''], settings: {} };
}

function descendantIDs(parentID: string) {
  const result = new Set<string>();
  if (!parentID) return result;
  const pending = [parentID];
  while (pending.length) {
    const current = pending.pop();
    for (const item of items.value) {
      if (!current || item.parent_id !== current || result.has(item.id)) continue;
      result.add(item.id);
      pending.push(item.id);
    }
  }
  return result;
}

function settingChoice(value?: boolean): SettingChoice {
  if (value === undefined) return 'inherit';
  return value ? 'enabled' : 'disabled';
}

function settingValue(value: SettingChoice): boolean | undefined {
  if (value === 'inherit') return undefined;
  return value === 'enabled';
}

async function loadLibraries() {
  loading.value = true;
  error.value = '';
  try {
    items.value = await api.getMediaLibraries();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库加载失败';
  } finally {
    loading.value = false;
  }
}

function replaceSummary(detail: MediaLibraryDetail) {
  const index = items.value.findIndex((item) => item.id === detail.id);
  if (index >= 0) items.value[index] = detail;
  else items.value = [...items.value, detail];
}

function openCreate(kind: MediaLibraryKind = 'collection', parentID = '') {
  editingID.value = '';
  editingKind.value = kind;
  draft.value = { ...emptyDraft(), kind, parent_id: parentID };
  episodeSetting.value = 'inherit';
  autoDetectSetting.value = 'inherit';
  editorError.value = '';
  editorOpen.value = true;
}

function openEdit(item: MediaLibrarySummary) {
  editingID.value = item.id;
  editingKind.value = item.kind;
  draft.value = {
    name: item.name,
    kind: item.kind,
    parent_id: item.parent_id ?? '',
    directories: item.directories.map((directory) => directory.path),
    settings: { ...item.settings },
  };
  episodeSetting.value = settingChoice(item.settings.episode_number_detection);
  autoDetectSetting.value = settingChoice(item.settings.auto_detect_series);
  editorError.value = '';
  editorOpen.value = true;
}

function addDirectory() {
  draft.value.directories.push('');
}

function removeDirectory(index: number) {
  if (draft.value.directories.length === 1) draft.value.directories[0] = '';
  else draft.value.directories.splice(index, 1);
}

function browseDirectory(index: number) {
  pickerIndex.value = index;
  pickerOpen.value = true;
}

function selectDirectory(path: string) {
  draft.value.directories[pickerIndex.value] = path;
}

async function saveLibrary() {
  if (editorLoading.value) return;
  editorLoading.value = true;
  editorError.value = '';
  const settings: MediaLibrarySettingsOverride = {};
  const episode = settingValue(episodeSetting.value);
  const autoDetect = settingValue(autoDetectSetting.value);
  if (episode !== undefined) settings.episode_number_detection = episode;
  if (autoDetect !== undefined) settings.auto_detect_series = autoDetect;
  const payload: MediaLibrarySaveRequest = {
    name: draft.value.name.trim(),
    kind: editingID.value ? editingKind.value : draft.value.kind,
    parent_id: draft.value.parent_id ?? '',
    directories: draft.value.directories.map((path) => path.trim()),
    settings,
  };
  try {
    const saved = editingID.value
      ? await api.updateMediaLibrary(editingID.value, payload)
      : await api.createMediaLibrary(payload);
    replaceSummary(saved);
    if (!editingID.value && saved.parent_id) {
      expandedIDs.value = new Set(expandedIDs.value).add(saved.parent_id);
    }
    editorOpen.value = false;
    pickerOpen.value = false;
  } catch (reason) {
    editorError.value = reason instanceof Error ? reason.message : '媒体库保存失败';
  } finally {
    editorLoading.value = false;
  }
}

async function scanLibrary(item: MediaLibrarySummary) {
  if (action.value) return;
  action.value = `scan:${item.id}`;
  error.value = '';
  try {
    replaceSummary(await api.scanMediaLibrary(item.id));
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库扫描失败';
  } finally {
    action.value = '';
  }
}

async function deleteLibrary(item: MediaLibrarySummary) {
  if (action.value) return;
  action.value = `delete:${item.id}`;
  error.value = '';
  try {
    await api.deleteMediaLibrary(item.id);
    items.value = items.value.filter((candidate) => candidate.id !== item.id);
    const next = new Set(expandedIDs.value);
    next.delete(item.id);
    expandedIDs.value = next;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '媒体库删除失败';
  } finally {
    action.value = '';
  }
}

async function openLibrary(item: MediaLibrarySummary) {
  if (item.kind === 'series') {
    await router.push({ name: 'playback-series', params: { series_id: item.id } });
    return;
  }
  await router.push({ name: 'library-detail', params: { library_id: item.id } });
}

function parentName(item: MediaLibrarySummary) {
  return items.value.find((candidate) => candidate.id === item.parent_id)?.name ?? '';
}

function childCount(item: MediaLibrarySummary) {
  return items.value.filter((candidate) => candidate.parent_id === item.id).length;
}

function isExpanded(item: MediaLibrarySummary) {
  return expandedIDs.value.has(item.id);
}

function toggleExpanded(item: MediaLibrarySummary) {
  const next = new Set(expandedIDs.value);
  if (next.has(item.id)) next.delete(item.id);
  else next.add(item.id);
  expandedIDs.value = next;
}

onMounted(loadLibraries);
</script>

<template>
  <section class="library-view">
    <div class="library-heading">
      <div>
        <h1>媒体库</h1>
        <p>像目录一样组织任意层级；点击媒体库查看它的下级和扫描媒体。</p>
      </div>
      <NSpace>
        <NButton secondary :loading="loading" @click="loadLibraries">
          <template #icon><NIcon :component="RefreshCw" /></template>
          刷新
        </NButton>
        <NButton type="primary" @click="openCreate()">
          <template #icon><NIcon :component="Plus" /></template>
          新增媒体库
        </NButton>
      </NSpace>
    </div>

    <NAlert type="info" :bordered="false">
      当前雏形先建立层级、目录和设置继承。目录监控、自动识别子剧集与字幕预处理将在后续阶段接入。
    </NAlert>
    <NAlert v-if="error" type="error" :bordered="false">{{ error }}</NAlert>

    <NSpin :show="loading">
      <NEmpty v-if="!loading && !items.length" description="还没有媒体库，请先添加一个目录集合或剧集" />
      <div v-else class="library-tree">
        <NCard
          v-for="item in rows"
          :key="item.id"
          :bordered="false"
          class="library-row"
          :style="{ '--library-depth': item.depth }"
        >
          <div class="library-row-main">
            <div class="library-indent" />
            <button
              v-if="childCount(item)"
              class="library-expander"
              type="button"
              :aria-label="isExpanded(item) ? `收起 ${item.name}` : `展开 ${item.name}`"
              :aria-expanded="isExpanded(item)"
              @click="toggleExpanded(item)"
            >
              <NIcon :component="isExpanded(item) ? ChevronDown : ChevronRight" size="18" />
            </button>
            <span v-else class="library-expander-spacer" />
            <NIcon :component="item.kind === 'series' ? Film : FolderTree" size="22" />
            <button class="library-title" type="button" @click="openLibrary(item)">
              <strong>{{ item.name }}</strong>
              <small>
                {{ item.available_video_count }} / {{ item.video_count }} 个媒体可用 · {{ childCount(item) }} 个下级
                <template v-if="parentName(item)"> · 属于 {{ parentName(item) }}</template>
              </small>
            </button>
            <NSpace>
              <NButton v-if="item.kind !== 'series'" size="small" secondary @click="openCreate('collection', item.id)">
                新增下级
              </NButton>
              <NButton size="small" secondary @click="openEdit(item)">
                <template #icon><NIcon :component="Edit3" /></template>
                编辑
              </NButton>
              <NButton
                size="small"
                secondary
                :loading="action === `scan:${item.id}`"
                :disabled="Boolean(action)"
                @click="scanLibrary(item)"
              >
                <template #icon><NIcon :component="RefreshCw" /></template>
                重扫
              </NButton>
              <NPopconfirm positive-text="仅删除配置" negative-text="取消" @positive-click="deleteLibrary(item)">
                <template #trigger>
                  <NButton size="small" type="error" secondary :disabled="Boolean(action)">
                    <template #icon><NIcon :component="Trash2" /></template>
                    删除
                  </NButton>
                </template>
                有子媒体库的节点不能删除；不会删除目录或媒体文件。
              </NPopconfirm>
            </NSpace>
          </div>
          <div class="library-row-details">
            <span v-for="directory in item.directories" :key="directory.path" :title="directory.path">
              <NIcon :component="FolderOpen" size="15" />{{ directory.path }}
            </span>
          </div>
        </NCard>
      </div>
    </NSpin>

    <ResizableModal
      v-model:show="editorOpen"
      :title="editingID ? '编辑媒体库' : '新增媒体库'"
      storage-key="nexusbridge:dialog-width:media-library-editor"
      :default-width="620"
      :min-width="500"
      :max-width="920"
    >
      <NAlert v-if="editorError" type="error" :bordered="false" class="library-editor-error">
        {{ editorError }}
      </NAlert>
      <NForm @submit.prevent="saveLibrary">
        <NFormItem label="名称">
          <NInput v-model:value="draft.name" placeholder="例如：视频 / 彻夜之歌 第二季" />
        </NFormItem>
        <NFormItem label="用途">
          <NSelect v-model:value="draft.kind" :options="kindOptions" :disabled="Boolean(editingID)" />
        </NFormItem>
        <NFormItem label="父媒体库">
          <NSelect
            v-model:value="draft.parent_id"
            :options="parentOptions"
            clearable
            placeholder="不选择表示根媒体库"
          />
        </NFormItem>
        <NFormItem label="目录">
          <div class="library-directory-editor">
            <div v-for="(_, index) in draft.directories" :key="index" class="library-directory-field">
              <NInput v-model:value="draft.directories[index]" placeholder="本机绝对目录路径" />
              <div class="library-directory-actions">
                <NButton secondary @click="browseDirectory(index)">
                  <template #icon><NIcon :component="FolderOpen" /></template>
                  选择
                </NButton>
                <NButton quaternary circle :aria-label="`删除第 ${index + 1} 个目录`" @click="removeDirectory(index)">
                  <template #icon><NIcon :component="X" /></template>
                </NButton>
              </div>
            </div>
            <NButton block secondary @click="addDirectory">
              <template #icon><NIcon :component="Plus" /></template>
              添加目录
            </NButton>
          </div>
        </NFormItem>
        <NFormItem label="集数识别">
          <NSelect v-model:value="episodeSetting" :options="settingOptions" />
        </NFormItem>
        <NFormItem label="自动发现子剧集">
          <NSelect v-model:value="autoDetectSetting" :options="settingOptions" />
        </NFormItem>
        <div class="library-editor-footer">
          <NPopover trigger="click" placement="top-start">
            <template #trigger>
              <NButton quaternary circle aria-label="查看媒体库填写提示">
                <template #icon><NIcon :component="CircleAlert" /></template>
              </NButton>
            </template>
            <div class="library-editor-help">
              普通媒体库可以包含下级；剧集不能作为父媒体库。子目录必须位于父目录内。继承项会沿父链取最近的显式设置；自动发现目前只保存设置，不执行后台任务。
            </div>
          </NPopover>
          <div class="library-editor-actions">
            <NButton @click="editorOpen = false">取消</NButton>
            <NButton type="primary" attr-type="submit" :loading="editorLoading">保存</NButton>
          </div>
        </div>
      </NForm>
    </ResizableModal>

    <FilePickerDialog
      v-model:show="pickerOpen"
      mode="directory"
      title="选择媒体库目录"
      :initial-path="draft.directories[pickerIndex] ?? ''"
      selectable
      @select="selectDirectory"
    />
  </section>
</template>

<style scoped>
.library-view,
.library-directory-editor {
  display: grid;
  gap: 14px;
}

.library-directory-editor {
  width: 100%;
}

.library-tree {
  display: grid;
  gap: 0;
}

.library-heading,
.library-row-main,
.library-editor-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.library-heading {
  justify-content: space-between;
}

.library-heading h1,
.library-heading p {
  margin: 0;
}

.library-heading p {
  margin-top: 4px;
  color: #667085;
}

.library-row :deep(.n-card__content) {
  display: grid;
  gap: 10px;
}

.library-row + .library-row {
  border-top: 1px solid #edf0f4;
}

.library-indent {
  width: calc(var(--library-depth) * 28px);
  flex: none;
}

.library-expander,
.library-expander-spacer {
  display: grid;
  width: 28px;
  height: 28px;
  flex: none;
  place-items: center;
}

.library-expander {
  border: 0;
  border-radius: 4px;
  padding: 0;
  color: #667085;
  background: transparent;
  cursor: pointer;
}

.library-expander:hover {
  background: #f2f4f7;
}

.library-title {
  display: grid;
  min-width: 0;
  flex: 1;
  border: 0;
  padding: 0;
  color: inherit;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.library-title small,
.library-row-details {
  color: #667085;
}

.library-row-details {
  display: grid;
  gap: 5px;
  padding-left: calc(var(--library-depth) * 28px + 70px);
  font-size: 12px;
}

.library-row-details span {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.library-directory-field {
  display: grid;
  gap: 8px;
}

.library-directory-field .n-input {
  min-width: 0;
  width: 100%;
}

.library-directory-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.library-editor-error {
  margin-bottom: 12px;
}

.library-editor-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 16px;
}

.library-editor-help {
  max-width: 380px;
  line-height: 1.6;
}

@media (max-width: 760px) {
  .library-heading,
  .library-row-main {
    align-items: stretch;
    flex-direction: column;
  }

  .library-indent {
    display: none;
  }

  .library-row-details {
    padding-left: 0;
  }
}
</style>
