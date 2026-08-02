<!-- 日志页读取进程内最近日志，并提供卡片与纯文本两种只读视图。 -->
<script setup lang="ts">
import { NCard, NEmpty, NRadioButton, NRadioGroup, NSelect, NTag } from 'naive-ui';
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { LogEntry, LogSnapshot } from '../types';

type LogViewMode = 'cards' | 'text';
type LogLevelKind = 'debug' | 'info' | 'warn' | 'error';
type ConnectionState = 'connecting' | 'live' | 'reconnecting';

const viewModeStorageKey = 'nexusbridge.logs.view-mode';
const limitOptions = [200, 500, 1000, 2000].map((value) => ({ label: `最近 ${value} 条`, value }));

const snapshot = ref<LogSnapshot>({ items: [], total: 0, capacity: 2000 });
const viewMode = ref<LogViewMode>(storedViewMode());
const limit = ref(500);
const ready = ref(false);
const connectionState = ref<ConnectionState>('connecting');
let logSource: EventSource | undefined;

const cardEntries = computed(() => [...snapshot.value.items].reverse());
const summary = computed(() => {
  const shown = snapshot.value.items.length;
  return `显示 ${shown} 条，缓冲区共 ${snapshot.value.total}/${snapshot.value.capacity} 条`;
});
const connectionLabel = computed(() => {
  if (connectionState.value === 'live') return '实时连接';
  if (connectionState.value === 'reconnecting') return '正在重连';
  return '正在连接';
});

function storedViewMode(): LogViewMode {
  try {
    return window.localStorage.getItem(viewModeStorageKey) === 'text' ? 'text' : 'cards';
  } catch {
    return 'cards';
  }
}

function persistViewMode(mode: LogViewMode) {
  try {
    window.localStorage.setItem(viewModeStorageKey, mode);
  } catch {
    // 浏览器禁用本地存储时仍允许本次会话切换视图。
  }
}

function levelKind(level: string): LogLevelKind {
  const normalized = level.toUpperCase();
  if (normalized.startsWith('ERROR')) return 'error';
  if (normalized.startsWith('WARN')) return 'warn';
  if (normalized.startsWith('DEBUG')) return 'debug';
  return 'info';
}

function tagType(level: string): 'default' | 'info' | 'warning' | 'error' {
  switch (levelKind(level)) {
    case 'error':
      return 'error';
    case 'warn':
      return 'warning';
    case 'debug':
      return 'default';
    default:
      return 'info';
  }
}

function parseEvent<T>(event: Event): T | undefined {
  try {
    return JSON.parse((event as MessageEvent<string>).data) as T;
  } catch {
    return undefined;
  }
}

function connectLogStream() {
  logSource?.close();
  ready.value = false;
  connectionState.value = 'connecting';
  const source = new EventSource(`/api/logs/stream?limit=${limit.value}`);
  logSource = source;
  source.onopen = () => {
    if (logSource !== source) return;
    connectionState.value = 'live';
  };
  source.onerror = () => {
    if (logSource !== source) return;
    connectionState.value = 'reconnecting';
  };
  source.addEventListener('snapshot', (event) => {
    if (logSource !== source) return;
    const next = parseEvent<LogSnapshot>(event);
    if (!next) return;
    snapshot.value = next;
    ready.value = true;
  });
  source.addEventListener('log', (event) => {
    if (logSource !== source) return;
    const entry = parseEvent<LogEntry>(event);
    if (!entry) return;
    const items = [...snapshot.value.items, entry].slice(-limit.value);
    snapshot.value = {
      items,
      total: Math.min(snapshot.value.capacity, snapshot.value.total + 1),
      capacity: snapshot.value.capacity,
    };
    ready.value = true;
  });
}

function closeLogStream() {
  if (logSource) {
    logSource.close();
    logSource = undefined;
  }
}

watch(viewMode, persistViewMode);
watch(limit, connectLogStream);

onMounted(connectLogStream);
onBeforeUnmount(closeLogStream);
</script>

<template>
  <section class="view-stack logs-view">
    <NCard :bordered="false" class="logs-toolbar-card">
      <div class="logs-toolbar">
        <div class="logs-heading">
          <h2>运行日志</h2>
          <p>{{ summary }} · 新日志实时推送</p>
        </div>

        <div class="logs-controls">
          <NRadioGroup v-model:value="viewMode" size="small">
            <NRadioButton value="cards">卡片消息</NRadioButton>
            <NRadioButton value="text">纯文本</NRadioButton>
          </NRadioGroup>
          <NSelect v-model:value="limit" class="log-limit" :options="limitOptions" size="small" />
          <NTag :type="connectionState === 'live' ? 'success' : 'warning'" size="small" round>
            {{ connectionLabel }}
          </NTag>
        </div>
      </div>
    </NCard>

    <NEmpty v-if="!snapshot.items.length" :description="ready ? '当前还没有运行日志' : '正在连接实时日志流'" />

    <div v-else-if="viewMode === 'cards'" class="log-card-list">
      <article
        v-for="(entry, index) in cardEntries"
        :key="`${entry.timestamp}-${index}-${entry.text}`"
        class="log-message-card"
        :class="`log-level-${levelKind(entry.level)}`"
      >
        <header>
          <time>{{ entry.timestamp }}</time>
          <NTag :type="tagType(entry.level)" size="small" round>{{ entry.level }}</NTag>
          <span class="log-module">{{ entry.module }}</span>
        </header>
        <p>{{ entry.message }}</p>
        <div v-if="entry.fields.length" class="log-fields">
          <code v-for="field in entry.fields" :key="field">{{ field }}</code>
        </div>
      </article>
    </div>

    <div v-else class="log-text-panel" role="log" aria-label="运行日志纯文本">
      <div
        v-for="(entry, index) in snapshot.items"
        :key="`${entry.timestamp}-${index}-${entry.text}`"
        class="log-text-line"
        :class="`log-level-${levelKind(entry.level)}`"
      >
        {{ entry.text }}
      </div>
    </div>
  </section>
</template>

<style scoped>
.logs-view {
  min-width: 0;
}

.logs-toolbar-card {
  position: sticky;
  top: 0;
  z-index: 2;
}

.logs-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
}

.logs-heading h2,
.logs-heading p {
  margin: 0;
}

.logs-heading h2 {
  font-size: 18px;
}

.logs-heading p {
  margin-top: 5px;
  color: #64748b;
  font-size: 12px;
}

.logs-controls {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.log-limit {
  width: 122px;
}

.log-card-list {
  display: grid;
  gap: 9px;
}

.log-message-card {
  border: 1px solid #dbe2ea;
  border-left: 4px solid #2563eb;
  border-radius: 8px;
  padding: 12px 14px;
  background: #fff;
  box-shadow: 0 4px 14px rgba(15, 23, 42, 0.04);
}

.log-message-card.log-level-debug {
  border-left-color: #64748b;
  background: #f8fafc;
}

.log-message-card.log-level-warn {
  border-left-color: #d97706;
  background: #fffbeb;
}

.log-message-card.log-level-error {
  border-left-color: #dc2626;
  background: #fef2f2;
}

.log-message-card header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  color: #64748b;
  font-size: 12px;
}

.log-module {
  border-radius: 999px;
  padding: 2px 8px;
  color: #334155;
  background: rgba(148, 163, 184, 0.16);
  font-weight: 600;
}

.log-message-card p {
  margin: 9px 0 0;
  color: #172033;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.log-fields {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 9px;
}

.log-fields code {
  border-radius: 4px;
  padding: 3px 6px;
  color: #475569;
  background: rgba(148, 163, 184, 0.14);
  font-size: 12px;
  overflow-wrap: anywhere;
}

.log-text-panel {
  min-height: 360px;
  max-height: calc(100vh - 210px);
  overflow: auto;
  border: 1px solid #1e293b;
  border-radius: 8px;
  padding: 13px 15px;
  color: #bfdbfe;
  background: #0f172a;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', monospace;
  font-size: 12px;
  line-height: 1.65;
}

.log-text-line {
  min-width: max-content;
  white-space: pre;
}

.log-text-line.log-level-debug {
  color: #94a3b8;
}

.log-text-line.log-level-info {
  color: #93c5fd;
}

.log-text-line.log-level-warn {
  color: #fbbf24;
}

.log-text-line.log-level-error {
  color: #fca5a5;
}

@media (max-width: 760px) {
  .logs-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .logs-controls {
    justify-content: flex-start;
  }

  .log-text-panel {
    max-height: calc(100vh - 300px);
  }
}
</style>
