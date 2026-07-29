<!-- 剧集视图管理多个本机目录组成的有序视频集合。 -->
<script setup lang="ts">
import { Edit3, Film, FolderOpen, Play, Plus, RefreshCw, Trash2, X } from '@lucide/vue';
import { onMounted, ref } from 'vue';
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
  NSpace,
  NSpin,
  NTag,
} from 'naive-ui';
import { useRouter } from 'vue-router';
import { api } from '../api';
import type { SeriesDetail, SeriesSaveRequest, SeriesSummary } from '../types';
import FilePickerDialog from './FilePickerDialog.vue';
import ResizableModal from './ResizableModal.vue';

const router = useRouter();
const items = ref<SeriesSummary[]>([]);
const loading = ref(true);
const error = ref('');
const action = ref('');
const editorOpen = ref(false);
const editorLoading = ref(false);
const editorError = ref('');
const editingID = ref('');
const draft = ref<SeriesSaveRequest>({ name: '', directories: [''] });
const editingDirectoryIndex = ref<number | null>(0);
const pickerOpen = ref(false);
const pickerDirectoryIndex = ref(0);
const pickerInitialPath = ref('');
const pickerSelectable = ref(true);

async function loadSeries() {
  loading.value = true;
  error.value = '';
  try {
    items.value = await api.getSeries();
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '剧集加载失败';
  } finally {
    loading.value = false;
  }
}

function replaceSummary(detail: SeriesDetail) {
  const index = items.value.findIndex((item) => item.id === detail.id);
  if (index >= 0) {
    items.value[index] = detail;
  } else {
    items.value = [...items.value, detail].sort((left, right) =>
      left.name.localeCompare(right.name, undefined, { sensitivity: 'base' }),
    );
  }
}

function openCreate() {
  editingID.value = '';
  draft.value = { name: '', directories: [''] };
  editingDirectoryIndex.value = 0;
  pickerOpen.value = false;
  editorError.value = '';
  editorOpen.value = true;
}

async function openEdit(item: SeriesSummary) {
  editingID.value = item.id;
  draft.value = { name: item.name, directories: item.directories.map((directory) => directory.path) };
  editingDirectoryIndex.value = null;
  pickerOpen.value = false;
  editorError.value = '';
  editorOpen.value = true;
  try {
    const detail = await api.getSeriesDetail(item.id);
    draft.value = { name: detail.name, directories: detail.directories.map((directory) => directory.path) };
  } catch (reason) {
    editorError.value = reason instanceof Error ? reason.message : '剧集详情加载失败';
  }
}

function addDirectory() {
  if (editingDirectoryIndex.value !== null) {
    const currentPath = draft.value.directories[editingDirectoryIndex.value].trim();
    if (!currentPath) return;
    draft.value.directories[editingDirectoryIndex.value] = currentPath;
  }
  draft.value.directories.push('');
  editingDirectoryIndex.value = draft.value.directories.length - 1;
}

function removeDirectory(index: number) {
  if (draft.value.directories.length === 1) {
    draft.value.directories[0] = '';
    editingDirectoryIndex.value = 0;
    return;
  }
  draft.value.directories.splice(index, 1);
  if (editingDirectoryIndex.value === index) {
    editingDirectoryIndex.value = null;
  } else if (editingDirectoryIndex.value !== null && editingDirectoryIndex.value > index) {
    editingDirectoryIndex.value--;
  }
}

function editDirectory(index: number) {
  editingDirectoryIndex.value = index;
}

function finishDirectory(index: number) {
  const path = draft.value.directories[index].trim();
  if (!path) return;
  draft.value.directories[index] = path;
  if (editingDirectoryIndex.value === index) editingDirectoryIndex.value = null;
}

function browseDirectory(index: number, selectable: boolean) {
  pickerDirectoryIndex.value = index;
  pickerInitialPath.value = draft.value.directories[index].trim();
  pickerSelectable.value = selectable;
  pickerOpen.value = true;
}

function selectDirectory(path: string) {
  const index = pickerDirectoryIndex.value;
  if (index < 0 || index >= draft.value.directories.length) return;
  draft.value.directories[index] = path;
  editingDirectoryIndex.value = null;
}

async function saveSeries() {
  if (editorLoading.value) return;
  editorLoading.value = true;
  editorError.value = '';
  const payload = {
    name: draft.value.name.trim(),
    directories: draft.value.directories.map((path) => path.trim()),
  };
  try {
    const saved = editingID.value ? await api.updateSeries(editingID.value, payload) : await api.createSeries(payload);
    replaceSummary(saved);
    pickerOpen.value = false;
    editorOpen.value = false;
  } catch (reason) {
    editorError.value = reason instanceof Error ? reason.message : '剧集保存失败';
  } finally {
    editorLoading.value = false;
  }
}

async function scanSeries(item: SeriesSummary) {
  if (action.value) return;
  action.value = `scan:${item.id}`;
  error.value = '';
  try {
    replaceSummary(await api.scanSeries(item.id));
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '剧集扫描失败';
  } finally {
    action.value = '';
  }
}

async function deleteSeries(item: SeriesSummary) {
  if (action.value) return;
  action.value = `delete:${item.id}`;
  error.value = '';
  try {
    await api.deleteSeries(item.id);
    items.value = items.value.filter((candidate) => candidate.id !== item.id);
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '剧集删除失败';
  } finally {
    action.value = '';
  }
}

async function playSeries(item: SeriesSummary) {
  if (item.available_video_count <= 0) return;
  await router.push({ name: 'playback-series', params: { series_id: item.id } });
}

function formatDate(value?: string) {
  if (!value) return '尚未扫描';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

onMounted(loadSeries);
</script>

<template>
  <section class="series-view">
    <div class="series-heading">
      <div>
        <h1>剧集</h1>
        <p>把多个本机目录组织为一个连续的视频选集。</p>
      </div>
      <NSpace>
        <NButton secondary :loading="loading" @click="loadSeries">
          <template #icon><NIcon :component="RefreshCw" /></template>
          刷新列表
        </NButton>
        <NButton type="primary" @click="openCreate">
          <template #icon><NIcon :component="Plus" /></template>
          新增剧集
        </NButton>
      </NSpace>
    </div>

    <NAlert v-if="error" type="error" :bordered="false">{{ error }}</NAlert>
    <NSpin :show="loading">
      <NEmpty v-if="!loading && !items.length" description="还没有剧集，请先添加包含视频的目录" />
      <div v-else class="series-grid">
        <NCard v-for="item in items" :key="item.id" :bordered="false" class="series-card">
          <div class="series-card-heading">
            <button
              type="button"
              class="series-title-button"
              :disabled="item.available_video_count <= 0"
              @click="playSeries(item)"
            >
              <NIcon :component="Film" size="22" />
              <span>
                <strong>{{ item.name }}</strong>
                <small>{{ item.available_video_count }} / {{ item.video_count }} 个视频可用</small>
              </span>
              <NIcon v-if="item.available_video_count > 0" :component="Play" size="18" />
            </button>
            <NTag :type="item.scan_errors.length ? 'warning' : item.available_video_count ? 'success' : 'default'">
              {{ item.scan_errors.length ? '部分目录不可用' : item.available_video_count ? '可播放' : '无可用视频' }}
            </NTag>
          </div>

          <div class="series-directories">
            <div v-for="directory in item.directories" :key="directory.path" class="series-directory">
              <NIcon :component="FolderOpen" size="17" />
              <span :title="directory.path">{{ directory.path }}</span>
              <NTag v-if="!directory.available" size="small" type="warning">不可用</NTag>
            </div>
          </div>

          <NAlert v-if="item.scan_errors.length" type="warning" :bordered="false">
            <div v-for="scanError in item.scan_errors" :key="scanError" class="series-scan-error">
              {{ scanError }}
            </div>
          </NAlert>

          <div class="series-card-footer">
            <small>最近扫描：{{ formatDate(item.last_scanned_at) }}</small>
            <NSpace>
              <NButton size="small" secondary @click="openEdit(item)">
                <template #icon><NIcon :component="Edit3" /></template>
                编辑
              </NButton>
              <NButton
                size="small"
                secondary
                :loading="action === `scan:${item.id}`"
                :disabled="Boolean(action)"
                @click="scanSeries(item)"
              >
                <template #icon><NIcon :component="RefreshCw" /></template>
                重扫
              </NButton>
              <NPopconfirm positive-text="仅删除剧集" negative-text="取消" @positive-click="deleteSeries(item)">
                <template #trigger>
                  <NButton size="small" type="error" secondary :disabled="Boolean(action)">
                    <template #icon><NIcon :component="Trash2" /></template>
                    删除
                  </NButton>
                </template>
                只删除剧集配置和扫描缓存，不会删除目录或任何源文件。
              </NPopconfirm>
            </NSpace>
          </div>
        </NCard>
      </div>
    </NSpin>

    <ResizableModal
      v-model:show="editorOpen"
      :title="editingID ? '编辑剧集' : '新增剧集'"
      storage-key="nexusbridge:dialog-width:series-editor"
      :default-width="600"
      :min-width="500"
      :max-width="900"
    >
      <NAlert v-if="editorError" type="error" :bordered="false" class="series-editor-error">
        {{ editorError }}
      </NAlert>
      <NForm class="dialog-form" @submit.prevent="saveSeries">
        <NFormItem label="名称">
          <NInput v-model:value="draft.name" placeholder="例如：某部动画 / 某系列" />
        </NFormItem>
        <NFormItem label="目录（按当前顺序排列）">
          <div class="series-directory-editor">
            <div v-for="(_, index) in draft.directories" :key="index" class="series-directory-field">
              <template v-if="editingDirectoryIndex === index">
                <NInput
                  v-model:value="draft.directories[index]"
                  class="series-directory-input"
                  placeholder="输入本机绝对目录路径"
                  autofocus
                  @keyup.enter="finishDirectory(index)"
                />
                <div class="series-directory-actions">
                  <NButton secondary size="small" @click="browseDirectory(index, true)">
                    <template #icon><NIcon :component="FolderOpen" /></template>
                    选择
                  </NButton>
                  <NButton size="small" :disabled="!draft.directories[index].trim()" @click="finishDirectory(index)">
                    完成
                  </NButton>
                  <NButton
                    quaternary
                    circle
                    size="small"
                    :aria-label="`移除第 ${index + 1} 个目录`"
                    @click="removeDirectory(index)"
                  >
                    <template #icon><NIcon :component="X" /></template>
                  </NButton>
                </div>
              </template>
              <template v-else>
                <button
                  type="button"
                  class="series-directory-path"
                  title="双击修改目录"
                  @dblclick="editDirectory(index)"
                  @keyup.enter="editDirectory(index)"
                >
                  {{ draft.directories[index] }}
                </button>
                <div class="series-directory-actions">
                  <NButton secondary size="small" @click="browseDirectory(index, false)">
                    <template #icon><NIcon :component="FolderOpen" /></template>
                    浏览
                  </NButton>
                  <NButton
                    quaternary
                    circle
                    size="small"
                    :aria-label="`移除第 ${index + 1} 个目录`"
                    @click="removeDirectory(index)"
                  >
                    <template #icon><NIcon :component="X" /></template>
                  </NButton>
                </div>
              </template>
            </div>
            <NButton class="series-add-directory" secondary @click="addDirectory">
              <template #icon><NIcon :component="Plus" /></template>
              添加目录
            </NButton>
            <small class="series-directory-help">已完成的路径会完整展示；双击路径可重新输入。</small>
          </div>
        </NFormItem>
        <NAlert type="info" :bordered="false">
          保存后会立即递归扫描视频；不会启动常驻目录监控，也不会修改目录内容。
        </NAlert>
        <div class="series-editor-actions">
          <NButton @click="editorOpen = false">取消</NButton>
          <NButton type="primary" attr-type="submit" :loading="editorLoading">保存并扫描</NButton>
        </div>
      </NForm>
    </ResizableModal>

    <FilePickerDialog
      v-model:show="pickerOpen"
      mode="directory"
      :title="pickerSelectable ? '选择剧集目录' : '浏览剧集目录'"
      :initial-path="pickerInitialPath"
      :selectable="pickerSelectable"
      @select="selectDirectory"
    />
  </section>
</template>

<style scoped>
.series-view {
  display: grid;
  gap: 16px;
}

.series-heading,
.series-card-heading,
.series-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.series-heading p,
.series-card-footer small,
.series-title-button small {
  margin-top: 4px;
  color: #667085;
  font-size: 13px;
}

.series-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(420px, 100%), 1fr));
  gap: 14px;
}

.series-card {
  min-width: 0;
}

.series-card :deep(.n-card__content),
.series-directories,
.series-directory-editor {
  display: grid;
  gap: 12px;
}

.series-directory-editor {
  gap: 0;
  width: 100%;
}

.series-title-button {
  display: flex;
  min-width: 0;
  flex: 1;
  align-items: center;
  gap: 10px;
  border: 0;
  padding: 0;
  color: #101828;
  background: transparent;
  text-align: left;
  cursor: pointer;
}

.series-title-button:disabled {
  cursor: default;
}

.series-title-button > span {
  display: grid;
  min-width: 0;
  flex: 1;
}

.series-title-button strong,
.series-directory span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.series-directories {
  margin-top: 14px;
}

.series-directory {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
}

.series-directory span {
  min-width: 0;
  flex: 1;
  color: #475467;
  font-size: 13px;
}

.series-scan-error {
  overflow-wrap: anywhere;
}

.series-card-footer {
  margin-top: 14px;
  align-items: flex-end;
}

.series-editor-error {
  margin-bottom: 12px;
}

.series-directory-field {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 8px;
  border-bottom: 1px solid #e4e7ec;
  padding: 9px 0;
}

.series-directory-field:first-child {
  border-top: 1px solid #e4e7ec;
}

.series-directory-input {
  min-width: 0;
  flex: 1;
}

.series-directory-path {
  min-width: 0;
  flex: 1;
  border: 0;
  padding: 2px;
  color: #344054;
  background: transparent;
  line-height: 1.55;
  overflow-wrap: anywhere;
  text-align: left;
  white-space: normal;
  cursor: text;
}

.series-directory-actions {
  display: flex;
  flex: none;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
}

.series-directory-help {
  margin-top: 8px;
  color: #667085;
  font-size: 12px;
}

.series-add-directory {
  margin-top: 12px;
}

.series-editor-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

@media (max-width: 720px) {
  .series-heading,
  .series-card-footer {
    align-items: stretch;
    flex-direction: column;
  }

  .series-card-footer :deep(.n-space) {
    justify-content: flex-end;
  }
}
</style>
