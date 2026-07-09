<!-- 媒体快捷设置以右下角浮动按钮管理布局与卡片标题展示。 -->
<script setup lang="ts">
import { LayoutGrid, List, SlidersHorizontal } from '@lucide/vue';
import { NButton, NButtonGroup, NIcon, NPopover, NSelect, NSlider } from 'naive-ui';
import { MEDIA_DISPLAY_LIMITS } from '../composables/useMediaDisplaySettings';
import type { MediaDisplaySettings, MediaLayout, MediaTitleMode } from '../composables/useMediaDisplaySettings';

const props = defineProps<{ modelValue: MediaDisplaySettings }>();
const emit = defineEmits<{ 'update:modelValue': [value: MediaDisplaySettings] }>();

const titleModeOptions = [
	{ label: '完整展示', value: 'full' },
	{ label: '截断', value: 'truncate' },
	{ label: '滚动', value: 'scroll' },
];

/** 更新单个展示设置并生成新对象。 */
function updateSetting<K extends keyof MediaDisplaySettings>(key: K, value: MediaDisplaySettings[K]) {
	emit('update:modelValue', { ...props.modelValue, [key]: value });
}

/** 更新媒体布局模式。 */
function updateLayout(value: MediaLayout) {
	updateSetting('layout', value);
}

/** 更新卡片标题展示模式。 */
function updateTitleMode(value: MediaTitleMode) {
	updateSetting('titleMode', value);
}
</script>

<template>
	<div class="media-quick-settings">
		<NPopover trigger="click" placement="top-end" :width="320">
			<template #trigger>
				<NButton type="primary" circle size="large" aria-label="打开媒体快捷设置" class="media-settings-trigger">
					<template #icon><NIcon :component="SlidersHorizontal" /></template>
				</NButton>
			</template>
			<div class="media-settings-panel">
				<div>
					<strong>展示形式</strong>
					<NButtonGroup class="media-layout-buttons">
						<NButton :type="modelValue.layout === 'list' ? 'primary' : 'default'" @click="updateLayout('list')">
							<template #icon><NIcon :component="List" /></template>列表
						</NButton>
						<NButton :type="modelValue.layout === 'card' ? 'primary' : 'default'" @click="updateLayout('card')">
							<template #icon><NIcon :component="LayoutGrid" /></template>卡片
						</NButton>
					</NButtonGroup>
				</div>
				<template v-if="modelValue.layout === 'card'">
					<label>
						<span>卡片最小宽度 {{ modelValue.cardMinWidth }}px</span>
						<NSlider
							:value="modelValue.cardMinWidth"
							:min="MEDIA_DISPLAY_LIMITS.cardMinWidth.min"
							:max="MEDIA_DISPLAY_LIMITS.cardMinWidth.max"
							:step="MEDIA_DISPLAY_LIMITS.cardMinWidth.step"
							@update:value="updateSetting('cardMinWidth', $event)"
						/>
					</label>
					<label>
						<span>标题展示</span>
						<NSelect :value="modelValue.titleMode" :options="titleModeOptions" @update:value="updateTitleMode" />
					</label>
					<label v-if="modelValue.titleMode !== 'full'">
						<span>{{ modelValue.titleMode === 'truncate' ? '截断行数' : '滚动区域行数' }}：{{ modelValue.titleLines }}</span>
						<NSlider
							:value="modelValue.titleLines"
							:min="MEDIA_DISPLAY_LIMITS.titleLines.min"
							:max="MEDIA_DISPLAY_LIMITS.titleLines.max"
							:step="MEDIA_DISPLAY_LIMITS.titleLines.step"
							@update:value="updateSetting('titleLines', $event)"
						/>
					</label>
				</template>
			</div>
		</NPopover>
	</div>
</template>
