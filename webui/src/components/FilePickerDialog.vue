<!-- 文件选择浏览器复用文件管理 API，并在浏览器本地记住上次使用的目录。 -->
<script setup lang="ts">
import { ChevronUp, File, Folder, RefreshCw } from '@lucide/vue';
import { computed, ref, watch } from 'vue';
import { NAlert, NButton, NEmpty, NIcon, NInput, NSpin } from 'naive-ui';
import { api } from '../api';
import type { FileBrowseResult, FileEntry } from '../types';
import ResizableModal from './ResizableModal.vue';
import MediaThumbnail from './MediaThumbnail.vue';

type FilePickerMode = 'directory' | 'file';

const props = withDefaults(
  defineProps<{
    show: boolean;
    mode?: FilePickerMode;
    initialPath?: string;
    title?: string;
    selectable?: boolean;
  }>(),
  {
    mode: 'file',
    initialPath: '',
    title: '',
    selectable: true,
  },
);

const emit = defineEmits<{
  'update:show': [value: boolean];
  select: [path: string];
}>();

const lastPathStorageKey = 'nexusbridge:file-picker:last-path';
const browser = ref<FileBrowseResult>({
  path: '',
  is_root: true,
  qb_connected: false,
  entries: [],
});
const pathDraft = ref('');
const selectedPath = ref('');
const browsing = ref(false);
const error = ref('');

const dialogTitle = computed(() => props.title || (props.mode === 'directory' ? '选择本机目录' : '选择本机文件'));
const selectedEntry = computed(() => browser.value.entries.find((entry) => entry.path === selectedPath.value) ?? null);
const selectedValue = computed(() => {
  if (!props.selectable) return '';
  if (props.mode === 'directory') {
    return selectedEntry.value?.is_dir ? selectedEntry.value.path : browser.value.path;
  }
  return selectedEntry.value && !selectedEntry.value.is_dir ? selectedEntry.value.path : '';
});

function loadLastPath() {
  try {
    return localStorage.getItem(lastPathStorageKey)?.trim() ?? '';
  } catch {
    return '';
  }
}

function saveLastPath(path: string) {
  if (!path) return;
  try {
    localStorage.setItem(lastPathStorageKey, path);
  } catch {
    // 浏览器禁用存储时仍允许本次选择，不把界面偏好升级为功能错误。
  }
}

async function browse(path: string, preserveError = false) {
  if (browsing.value) return false;
  browsing.value = true;
  if (!preserveError) error.value = '';
  try {
    const result = await api.browseFiles(path.trim());
    browser.value = result;
    pathDraft.value = result.path;
    selectedPath.value = '';
    saveLastPath(result.path);
    return true;
  } catch (reason) {
    error.value = reason instanceof Error ? reason.message : '无法读取目录';
    return false;
  } finally {
    browsing.value = false;
  }
}

async function initialize() {
  browser.value = {
    path: '',
    is_root: true,
    qb_connected: false,
    entries: [],
  };
  selectedPath.value = '';
  error.value = '';
  const explicitPath = props.initialPath.trim();
  const startPath = explicitPath || loadLastPath();
  pathDraft.value = startPath;
  if (!startPath) {
    await browse('');
    return;
  }
  if (await browse(startPath)) return;
  if (!explicitPath) {
    const rememberedError = error.value;
    if (await browse('')) {
      error.value = `上次使用的目录已无法读取：${rememberedError}`;
    }
  }
}

function selectEntry(entry: FileEntry) {
  if (!props.selectable) return;
  if ((props.mode === 'directory' && entry.is_dir) || (props.mode === 'file' && !entry.is_dir)) {
    selectedPath.value = entry.path;
  }
}

function activateEntry(entry: FileEntry) {
  if (entry.is_dir) {
    void browse(entry.path);
    return;
  }
  if (props.selectable && props.mode === 'file') {
    selectedPath.value = entry.path;
    confirmSelection();
  }
}

function confirmSelection() {
  const path = selectedValue.value;
  if (!path) return;
  saveLastPath(browser.value.path || path);
  emit('select', path);
  emit('update:show', false);
}

function formatBytes(value = 0) {
  if (!value) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

watch(
  () => props.show,
  (show) => {
    if (show) void initialize();
  },
  { immediate: true },
);
</script>

<template>
  <ResizableModal
    :show="show"
    :title="dialogTitle"
    storage-key="nexusbridge:dialog-width:file-picker"
    :default-width="720"
    :min-width="560"
    :fixed-height="660"
    @update:show="emit('update:show', $event)"
  >
    <div class="file-picker-dialog-body">
      <div class="file-picker-toolbar">
        <NButton
          aria-label="上一级"
          title="上一级"
          :disabled="!browser.parent || browsing"
          @click="browse(browser.parent || '')"
        >
          <template #icon><NIcon :component="ChevronUp" /></template>
        </NButton>
        <NInput v-model:value="pathDraft" placeholder="输入作为起点的本机绝对目录" @keyup.enter="browse(pathDraft)" />
        <NButton type="primary" :loading="browsing" @click="browse(pathDraft)">打开</NButton>
        <NButton aria-label="刷新" title="刷新" :loading="browsing" @click="browse(browser.path)">
          <template #icon><NIcon :component="RefreshCw" /></template>
        </NButton>
      </div>

      <NAlert v-if="error" type="error" :bordered="false">{{ error }}</NAlert>
      <p class="file-picker-hint">
        {{
          !selectable
            ? '当前为只读浏览；双击目录可进入，不能在这里替换剧集目录。'
            : mode === 'directory'
              ? '单击目录可选中，双击进入；未选中子目录时将选择当前目录。'
              : '单击文件可选中，双击文件立即确认；双击目录可进入。'
        }}
      </p>

      <div class="file-picker-list">
        <NSpin :show="browsing">
          <NEmpty v-if="!browser.entries.length" description="此位置没有可显示的条目" />
          <button
            v-for="entry in browser.entries"
            :key="entry.path"
            type="button"
            class="file-picker-entry"
            :class="{
              selected: selectable && selectedPath === entry.path,
              selectable: selectable && ((mode === 'directory' && entry.is_dir) || (mode === 'file' && !entry.is_dir)),
            }"
            @click="selectEntry(entry)"
            @dblclick="activateEntry(entry)"
          >
            <MediaThumbnail
              v-if="entry.media_type === 'video' || entry.media_type === 'image'"
              :src="entry.thumbnail_url"
              :alt="`${entry.name} 缩略图`"
              :media-type="entry.media_type"
              size="compact"
            />
            <NIcon
              v-else
              :component="entry.is_dir ? Folder : File"
              :class="entry.is_dir ? 'folder-icon' : 'file-icon'"
            />
            <span>
              <strong>{{ entry.name }}</strong>
              <small>{{ entry.path }}</small>
            </span>
            <small>{{ entry.is_dir ? '目录' : formatBytes(entry.size) }}</small>
          </button>
        </NSpin>
      </div>
    </div>

    <template #footer>
      <div class="file-picker-footer">
        <span :title="selectable ? selectedValue : browser.path">
          {{ selectable ? selectedValue || '尚未选择文件' : browser.path || '文件系统根目录' }}
        </span>
        <div>
          <NButton @click="emit('update:show', false)">{{ selectable ? '取消' : '关闭' }}</NButton>
          <NButton v-if="selectable" type="primary" :disabled="browsing || !selectedValue" @click="confirmSelection">
            {{ mode === 'directory' ? '选择目录' : '选择文件' }}
          </NButton>
        </div>
      </div>
    </template>
  </ResizableModal>
</template>

<style scoped>
.file-picker-dialog-body {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
}

.file-picker-toolbar {
  flex: none;
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto auto;
  gap: 8px;
}

.file-picker-hint {
  margin: 10px 0;
  color: #667085;
  font-size: 13px;
}

.file-picker-list {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  border: 1px solid #e4e7ec;
  border-radius: 8px;
}

.file-picker-entry {
  display: grid;
  width: 100%;
  grid-template-columns: 56px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  border: 0;
  border-bottom: 1px solid #eef2f6;
  min-height: 52px;
  padding: 6px 12px;
  color: #344054;
  background: #fff;
  text-align: left;
}

.file-picker-entry:last-child {
  border-bottom: 0;
}

.file-picker-entry:hover {
  background: #f8fafc;
}

.file-picker-entry.selectable {
  cursor: pointer;
}

.file-picker-entry.selected {
  background: #eff6ff;
  box-shadow: inset 3px 0 #2563eb;
}

.file-picker-entry > :deep(.n-icon) {
  justify-self: center;
}

.file-picker-entry > span:nth-child(2) {
  display: grid;
  min-width: 0;
}

.file-picker-entry strong,
.file-picker-entry small,
.file-picker-footer > span {
  overflow-wrap: anywhere;
}

.file-picker-entry small,
.file-picker-footer > span {
  color: #667085;
  font-size: 12px;
}

.folder-icon {
  color: #f59e0b;
}

.file-icon {
  color: #64748b;
}

.file-picker-footer {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-picker-footer > span {
  min-width: 0;
  flex: 1;
}

.file-picker-footer > div {
  display: flex;
  flex: none;
  gap: 8px;
}

@media (max-width: 640px) {
  .file-picker-toolbar {
    grid-template-columns: auto minmax(0, 1fr) auto;
  }

  .file-picker-toolbar > :nth-child(4) {
    display: none;
  }

  .file-picker-footer {
    align-items: stretch;
    flex-direction: column;
  }

  .file-picker-footer > div {
    justify-content: flex-end;
  }
}
</style>
