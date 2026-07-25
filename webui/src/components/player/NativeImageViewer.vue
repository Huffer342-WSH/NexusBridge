<!-- 原生图片查看器提供适应画布、缩放、重置和全屏。 -->
<script setup lang="ts">
import { Maximize, Minus, RotateCcw, Plus } from '@lucide/vue';
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { NButton, NIcon, NSpace } from 'naive-ui';

defineProps<{
  source: string;
  mimeType: string;
  name: string;
  poster?: string;
  title: string;
}>();

const emit = defineEmits<{
  error: [message: string];
  autoplayBlocked: [];
}>();

const host = ref<HTMLElement | null>(null);
const image = ref<HTMLImageElement | null>(null);
const fitWidth = ref(0);
const fitHeight = ref(0);
const viewportWidth = ref(0);
const viewportHeight = ref(0);
const zoomLevel = ref(1);
const pan = reactive({ x: 0, y: 0 });
const dragging = ref(false);
let resizeObserver: ResizeObserver | undefined;
let resizeFrame = 0;
let dragPointer = -1;
let dragStart = { x: 0, y: 0, panX: 0, panY: 0 };

const canPan = computed(
  () =>
    fitWidth.value * zoomLevel.value > viewportWidth.value + 1 ||
    fitHeight.value * zoomLevel.value > viewportHeight.value + 1,
);

const imageStyle = computed(() => ({
  width: `${fitWidth.value}px`,
  height: `${fitHeight.value}px`,
  left: `calc(50% + ${pan.x}px)`,
  top: `calc(50% + ${pan.y}px)`,
  transform: `translate(-50%, -50%) scale(${zoomLevel.value})`,
}));

function clampPan() {
  const maxX = Math.max(0, (fitWidth.value * zoomLevel.value - viewportWidth.value) / 2);
  const maxY = Math.max(0, (fitHeight.value * zoomLevel.value - viewportHeight.value) / 2);
  pan.x = Math.min(maxX, Math.max(-maxX, pan.x));
  pan.y = Math.min(maxY, Math.max(-maxY, pan.y));
}

function fitImage() {
  const hostElement = host.value;
  const imageElement = image.value;
  if (!hostElement || !imageElement?.naturalWidth || !imageElement.naturalHeight) return;

  viewportWidth.value = hostElement.clientWidth;
  viewportHeight.value = hostElement.clientHeight;
  const fitScale = Math.min(
    viewportWidth.value / imageElement.naturalWidth,
    viewportHeight.value / imageElement.naturalHeight,
    1,
  );
  fitWidth.value = imageElement.naturalWidth * fitScale;
  fitHeight.value = imageElement.naturalHeight * fitScale;
  zoomLevel.value = 1;
  pan.x = 0;
  pan.y = 0;
}

function scheduleFit() {
  cancelAnimationFrame(resizeFrame);
  resizeFrame = requestAnimationFrame(fitImage);
}

function zoom(delta: number) {
  zoomLevel.value = Math.min(8, Math.max(0.25, Number((zoomLevel.value + delta).toFixed(2))));
  clampPan();
}

function handleWheel(event: WheelEvent) {
  zoom(event.deltaY < 0 ? 0.2 : -0.2);
}

function startPan(event: PointerEvent) {
  if (event.button !== 0 || !canPan.value) return;
  dragging.value = true;
  dragPointer = event.pointerId;
  dragStart = {
    x: event.clientX,
    y: event.clientY,
    panX: pan.x,
    panY: pan.y,
  };
  host.value?.setPointerCapture(event.pointerId);
}

function movePan(event: PointerEvent) {
  if (!dragging.value || event.pointerId !== dragPointer) return;
  pan.x = dragStart.panX + event.clientX - dragStart.x;
  pan.y = dragStart.panY + event.clientY - dragStart.y;
  clampPan();
}

function stopPan(event: PointerEvent) {
  if (event.pointerId !== dragPointer) return;
  dragging.value = false;
  if (host.value?.hasPointerCapture(event.pointerId)) {
    host.value.releasePointerCapture(event.pointerId);
  }
  dragPointer = -1;
}

function enterFullscreen() {
  void host.value?.requestFullscreen();
}

onMounted(() => {
  resizeObserver = new ResizeObserver(scheduleFit);
  if (host.value) resizeObserver.observe(host.value);
});

onBeforeUnmount(() => {
  resizeObserver?.disconnect();
  cancelAnimationFrame(resizeFrame);
});
</script>

<template>
  <div
    ref="host"
    class="image-viewer"
    :class="{ 'is-pannable': canPan, 'is-dragging': dragging }"
    @wheel.prevent="handleWheel"
    @pointerdown="startPan"
    @pointermove="movePan"
    @pointerup="stopPan"
    @pointercancel="stopPan"
    @dblclick="fitImage"
  >
    <img
      ref="image"
      :src="source"
      :alt="title"
      :style="imageStyle"
      draggable="false"
      @load="fitImage"
      @error="emit('error', '浏览器无法加载或解码这个图片源文件')"
    />
    <NSpace class="image-viewer-controls" size="small" align="center" @pointerdown.stop @dblclick.stop>
      <NButton circle secondary aria-label="缩小" @click="zoom(-0.25)">
        <template #icon><NIcon :component="Minus" /></template>
      </NButton>
      <span class="image-viewer-zoom">{{ Math.round(zoomLevel * 100) }}%</span>
      <NButton circle secondary aria-label="适应画布" @click="fitImage">
        <template #icon><NIcon :component="RotateCcw" /></template>
      </NButton>
      <NButton circle secondary aria-label="放大" @click="zoom(0.25)">
        <template #icon><NIcon :component="Plus" /></template>
      </NButton>
      <NButton circle secondary aria-label="全屏" @click="enterFullscreen">
        <template #icon><NIcon :component="Maximize" /></template>
      </NButton>
    </NSpace>
  </div>
</template>

<style scoped>
.image-viewer {
  position: relative;
  isolation: isolate;
  width: 100%;
  height: 100%;
  overflow: hidden;
  contain: layout paint;
  background: #020617;
  cursor: default;
  touch-action: none;
  user-select: none;
}

.image-viewer.is-pannable {
  cursor: grab;
}

.image-viewer.is-dragging {
  cursor: grabbing;
}

.image-viewer img {
  position: absolute;
  display: block;
  max-width: none;
  max-height: none;
  pointer-events: none;
  object-fit: fill;
  transition:
    left 80ms ease-out,
    top 80ms ease-out,
    transform 120ms ease;
  transform-origin: center;
  will-change: left, top, transform;
}

.image-viewer.is-dragging img {
  transition: none;
}

.image-viewer-controls {
  position: absolute;
  right: 16px;
  bottom: 16px;
  z-index: 10;
  max-width: calc(100% - 32px);
  padding: 6px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.72);
  backdrop-filter: blur(10px);
  cursor: default;
}

.image-viewer-zoom {
  min-width: 44px;
  color: rgba(255, 255, 255, 0.88);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

@media (max-width: 520px) {
  .image-viewer-controls {
    right: 10px;
    bottom: 10px;
    max-width: calc(100% - 20px);
  }
}
</style>
