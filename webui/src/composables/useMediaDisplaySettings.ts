/** 媒体展示设置模块负责读取、约束并持久化页面快捷偏好。 */

import { computed, ref, watch } from 'vue';

export type MediaLayout = 'card' | 'list';
export type MediaTitleMode = 'full' | 'truncate' | 'scroll';

export interface MediaDisplaySettings {
  layout: MediaLayout;
  cardMinWidth: number;
  titleMode: MediaTitleMode;
  titleLines: number;
}

/** 媒体展示设置约束供数据校验和界面控件共同使用。 */
export const MEDIA_DISPLAY_LIMITS = {
  cardMinWidth: { min: 280, max: 560, step: 20, default: 420 },
  titleLines: { min: 1, max: 8, step: 1, default: 2 },
} as const;

const storageKey = 'nexusbridge.media.display-settings';
const legacyCardWidthStorageKey = 'nexusbridge.media.card-min-width';

/** 将未知输入转换为合法的展示设置。 */
function normalizeSettings(value?: Partial<MediaDisplaySettings>): MediaDisplaySettings {
  let legacyWidth = Number.NaN;
  try {
    legacyWidth = Number(localStorage.getItem(legacyCardWidthStorageKey));
  } catch {
    // 无法读取本地存储时直接使用默认宽度。
  }
  const cardMinWidth = Number(value?.cardMinWidth ?? legacyWidth);
  const titleLines = Number(value?.titleLines);
  const cardWidthLimits = MEDIA_DISPLAY_LIMITS.cardMinWidth;
  const titleLineLimits = MEDIA_DISPLAY_LIMITS.titleLines;
  return {
    layout: value?.layout === 'list' ? 'list' : 'card',
    cardMinWidth:
      Number.isFinite(cardMinWidth) && cardMinWidth >= cardWidthLimits.min && cardMinWidth <= cardWidthLimits.max
        ? cardMinWidth
        : cardWidthLimits.default,
    titleMode: value?.titleMode === 'full' || value?.titleMode === 'scroll' ? value.titleMode : 'truncate',
    titleLines:
      Number.isFinite(titleLines) && titleLines >= titleLineLimits.min && titleLines <= titleLineLimits.max
        ? titleLines
        : titleLineLimits.default,
  };
}

/** 从本地存储读取媒体展示设置。 */
function loadSettings() {
  try {
    const raw = localStorage.getItem(storageKey);
    return normalizeSettings(raw ? (JSON.parse(raw) as Partial<MediaDisplaySettings>) : undefined);
  } catch {
    return normalizeSettings();
  }
}

/** 提供媒体展示设置、布局类名和 CSS 变量。 */
export function useMediaDisplaySettings() {
  const settings = ref<MediaDisplaySettings>(loadSettings());

  watch(
    settings,
    (value) => {
      try {
        localStorage.setItem(storageKey, JSON.stringify(value));
      } catch {
        // 浏览器禁用本地存储时仍保留当前会话设置。
      }
    },
    { deep: true },
  );

  const layoutClass = computed(() => `media-layout-${settings.value.layout}`);
  const layoutStyle = computed(() => ({
    '--media-card-min-width': `${settings.value.cardMinWidth}px`,
    '--media-title-lines': String(settings.value.titleLines),
  }));

  return { settings, layoutClass, layoutStyle };
}
