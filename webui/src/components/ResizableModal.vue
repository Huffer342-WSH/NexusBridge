<!-- 可调宽度弹窗把用户宽度偏好保存在当前浏览器。 -->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import type { CSSProperties } from 'vue';
import { NModal } from 'naive-ui';

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    show: boolean;
    title: string;
    storageKey: string;
    defaultWidth?: number;
    minWidth?: number;
    maxWidth?: number;
    fixedHeight?: number;
  }>(),
  {
    defaultWidth: 640,
    minWidth: 440,
    maxWidth: 1040,
    fixedHeight: undefined,
  },
);

const emit = defineEmits<{
  'update:show': [value: boolean];
}>();

const viewportWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth);

function loadWidth() {
  try {
    const stored = Number(localStorage.getItem(props.storageKey));
    return Number.isFinite(stored) && stored > 0 ? stored : props.defaultWidth;
  } catch {
    return props.defaultWidth;
  }
}

const width = ref(loadWidth());
const maximumWidth = computed(() => Math.max(280, Math.min(props.maxWidth, viewportWidth.value - 28)));
const minimumWidth = computed(() => Math.min(props.minWidth, maximumWidth.value));
const displayedWidth = computed(() => Math.min(maximumWidth.value, Math.max(minimumWidth.value, width.value)));
const dialogStyle = computed<CSSProperties>(() => ({
  width: `${displayedWidth.value}px`,
  maxWidth: 'calc(100vw - 28px)',
  height: props.fixedHeight ? `min(${props.fixedHeight}px, calc(100vh - 28px))` : undefined,
}));
const contentStyle = computed<CSSProperties | undefined>(() =>
  props.fixedHeight
    ? {
        display: 'flex',
        minHeight: 0,
        flex: 1,
        flexDirection: 'column',
        overflow: 'hidden',
      }
    : undefined,
);

let startX = 0;
let startWidth = 0;

function saveWidth() {
  try {
    localStorage.setItem(props.storageKey, String(Math.round(width.value)));
  } catch {
    // 浏览器禁用存储时只影响下次打开，不影响本次拖动。
  }
}

function resizeTo(nextWidth: number) {
  width.value = Math.min(maximumWidth.value, Math.max(minimumWidth.value, nextWidth));
}

function handlePointerMove(event: PointerEvent) {
  resizeTo(startWidth + event.clientX - startX);
}

function stopResize() {
  window.removeEventListener('pointermove', handlePointerMove);
  window.removeEventListener('pointerup', stopResize);
  window.removeEventListener('pointercancel', stopResize);
  document.body.style.removeProperty('cursor');
  document.body.style.removeProperty('user-select');
  saveWidth();
}

function startResize(event: PointerEvent) {
  if (event.button !== 0) return;
  event.preventDefault();
  startX = event.clientX;
  startWidth = displayedWidth.value;
  document.body.style.cursor = 'ew-resize';
  document.body.style.userSelect = 'none';
  window.addEventListener('pointermove', handlePointerMove);
  window.addEventListener('pointerup', stopResize);
  window.addEventListener('pointercancel', stopResize);
}

function resizeWithKeyboard(event: KeyboardEvent) {
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
  event.preventDefault();
  const offset = event.shiftKey ? 32 : 8;
  resizeTo(displayedWidth.value + (event.key === 'ArrowRight' ? offset : -offset));
  saveWidth();
}

function updateViewportWidth() {
  viewportWidth.value = window.innerWidth;
}

onMounted(() => window.addEventListener('resize', updateViewportWidth));
onBeforeUnmount(() => {
  stopResize();
  window.removeEventListener('resize', updateViewportWidth);
});
</script>

<template>
  <NModal
    v-bind="$attrs"
    :show="show"
    preset="card"
    :title="title"
    :style="dialogStyle"
    :content-style="contentStyle"
    class="resizable-modal"
    @update:show="emit('update:show', $event)"
  >
    <slot />
    <template v-if="$slots.footer" #footer><slot name="footer" /></template>
    <template #header-extra>
      <div
        class="dialog-width-handle"
        role="separator"
        aria-label="拖动调整弹窗宽度"
        aria-orientation="vertical"
        tabindex="0"
        title="拖动调整宽度"
        @pointerdown="startResize"
        @keydown="resizeWithKeyboard"
      />
    </template>
  </NModal>
</template>

<style scoped>
.resizable-modal {
  position: relative;
}

.dialog-width-handle {
  position: absolute;
  z-index: 10;
  top: 52px;
  right: -4px;
  bottom: 12px;
  width: 10px;
  cursor: ew-resize;
  touch-action: none;
}

.dialog-width-handle::after {
  position: absolute;
  top: 35%;
  right: 4px;
  width: 2px;
  height: 30%;
  border-radius: 2px;
  background: #d0d5dd;
  content: '';
  transition: background 0.15s ease;
}

.dialog-width-handle:hover::after,
.dialog-width-handle:focus-visible::after {
  background: #667085;
}
</style>
