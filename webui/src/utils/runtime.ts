/** 运行环境适配模块隔离浏览器与 Wails 桌面专属能力。 */

interface WailsRuntimeWindow extends Window {
  _wails?: { environment?: { OS?: string } };
}

/** 判断当前页面是否运行在 Wails 桌面 WebView 中。 */
export function isWailsDesktop(): boolean {
  return Boolean((window as WailsRuntimeWindow)._wails?.environment?.OS);
}

/** 使用当前运行环境的默认方式打开外部链接。 */
export async function openExternalURL(url: string): Promise<void> {
  if (isWailsDesktop()) {
    const { Browser } = await import('@wailsio/runtime');
    await Browser.OpenURL(url);
    return;
  }
  window.open(url, '_blank', 'noopener,noreferrer');
}
