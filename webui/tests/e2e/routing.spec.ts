/** 验证页面路由直达、刷新、历史导航和未知地址处理。 */

import { expect, test, type Page } from '@playwright/test';

async function mockAppShell(page: Page) {
	await page.route('**/api/**', (route) => route.fulfill({
		status: 501,
		json: { error: `unmocked API request: ${route.request().url()}` },
	}));
	await page.route('**/api/session', (route) => route.fulfill({
		json: { mode: 'local', requires_login: false },
	}));
	await page.route('**/api/health', (route) => route.fulfill({
		json: { status: 'ok', addr: '127.0.0.1:8090' },
	}));
	await page.route('**/api/sites', (route) => route.fulfill({
		json: [{ id: 'demo', name: 'Demo Site', base_url: 'https://example.test', has_cookie: false }],
	}));
	await page.route('**/api/torrents?*', (route) => {
		const url = new URL(route.request().url());
		const offset = Number(url.searchParams.get('offset') ?? 0);
		const limit = Number(url.searchParams.get('limit') ?? 50);
		return route.fulfill({ json: { items: [], total: 200, offset, limit } });
	});
	await page.route('**/api/settings/fetch', (route) => route.fulfill({
		json: { max_pages: 3 },
	}));
	await page.route('**/api/site-fetch-jobs?*', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/settings/qbittorrent', (route) => route.fulfill({
		json: {
			auth_mode: 'uid',
			url: 'http://127.0.0.1:8080',
			api_key: '',
			username: '',
			user_id: '',
			password: '',
			category: '',
			tags: [],
			auto_sync: false,
			sync_interval_seconds: 5,
			inactive_sync_interval_seconds: 30,
			disconnected_sync_interval_seconds: 60,
		},
	}));
	await page.route('**/api/settings/llm', (route) => route.fulfill({
		json: { base_url: '', api_key: '', model: '' },
	}));
	await page.route('**/api/settings/network', (route) => route.fulfill({
		json: { mode: 'system', proxy_url: '', no_proxy: '' },
	}));
	await page.route('**/api/download-tasks', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/organize-tasks', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/qb/poll?*', (route) => route.fulfill({
		json: { rid: 1, full_update: true, connected: true, updated: 0, removed: 0, updates: [] },
	}));
}

test.beforeEach(async ({ page }) => {
	await mockAppShell(page);
});

test('uses clean URLs for redirects, navigation history, titles, and unknown routes', async ({ page }) => {
	await page.goto('/');
	await expect(page).toHaveURL(/\/media$/);
	await expect(page).toHaveTitle('媒体库 - NexusBridge');

	await page.getByRole('link', { name: '任务', exact: true }).click();
	await expect(page).toHaveURL(/\/tasks$/);
	await expect(page).toHaveTitle('任务 - NexusBridge');

	await page.getByRole('link', { name: '网络代理', exact: true }).click();
	await expect(page).toHaveURL(/\/settings\/network$/);
	await expect(page.getByRole('heading', { name: '网络代理' })).toBeVisible();
	await expect(page).toHaveTitle('设置 / 网络代理 - NexusBridge');

	await page.goBack();
	await expect(page).toHaveURL(/\/tasks$/);
	await page.goForward();
	await expect(page).toHaveURL(/\/settings\/network$/);

	await page.goto('/settings');
	await expect(page).toHaveURL(/\/settings\/sites$/);
	await expect(page.getByRole('heading', { name: '站点设置' })).toBeVisible();

	await page.goto('/unknown/page');
	await expect(page).toHaveURL(/\/unknown\/page$/);
	await expect(page.getByText('当前地址没有对应的 NexusBridge 页面。')).toBeVisible();
	await expect(page).toHaveTitle('页面不存在 - NexusBridge');
	await page.getByRole('link', { name: '返回媒体库' }).click();
	await expect(page).toHaveURL(/\/media$/);
});

test('keeps media filters and pagination in the URL across reloads', async ({ page }) => {
	await page.goto('/media?page=3&page_size=20&site=demo&q=movie&pinned=0');
	await expect(page.locator('.site-filter')).toContainText('Demo Site');
	await expect(page.getByPlaceholder('搜索标题、分类或站点')).toHaveValue('movie');
	await expect(page.getByRole('switch')).not.toBeChecked();
	await expect(page.locator('.n-pagination-item--active')).toHaveText('3');

	await page.reload();
	await expect(page).toHaveURL(/\/media\?/);
	await expect(page.locator('.site-filter')).toContainText('Demo Site');
	await expect(page.getByPlaceholder('搜索标题、分类或站点')).toHaveValue('movie');
	await expect(page.getByRole('switch')).not.toBeChecked();
	await expect(page.locator('.n-pagination-item--active')).toHaveText('3');

	await page.getByPlaceholder('搜索标题、分类或站点').fill('updated');
	await expect.poll(() => new URL(page.url()).searchParams.get('q')).toBe('updated');
	await expect.poll(() => new URL(page.url()).searchParams.get('page')).toBeNull();
	await page.getByRole('switch').click();
	await expect.poll(() => new URL(page.url()).searchParams.get('pinned')).toBeNull();

	await page.goto('/media?page=0&page_size=999&pinned=unexpected');
	await expect(page).toHaveURL(/\/media$/);
	await expect(page.getByRole('switch')).toBeChecked();
});

test('keeps direct routes on reload and releases unused file navigation buttons', async ({ page }) => {
	await page.route('**/api/files/browse', (route) => {
		const body = route.request().postDataJSON() as { path?: string };
		const path = body.path || 'C:/';
		return route.fulfill({
			json: {
				path,
				is_root: path === 'C:/',
				qb_connected: true,
				entries: [],
			},
		});
	});
	await page.route('**/api/qb/recovery/index', (route) => route.fulfill({
		json: { version: 1, total: 0, indexed: 0, pending: 0, failed: 0 },
	}));

	await page.goto(`/files?path=${encodeURIComponent('C:/Media/Films')}`);
	const pathInput = page.getByPlaceholder('输入本机完整目录路径');
	await expect(pathInput).toHaveValue('C:/Media/Films');
	await page.reload();
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Media/Films');
	await expect(pathInput).toHaveValue('C:/Media/Films');
	await expect(page).toHaveTitle('文件与恢复 - NexusBridge');
	await pathInput.fill('C:/Media/Shows');
	await pathInput.press('Enter');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Media/Shows');

	await page.goto('/settings/network');
	await page.reload();
	await expect(page).toHaveURL(/\/settings\/network$/);
	await expect(page.getByRole('heading', { name: '网络代理' })).toBeVisible();

	await page.goto('/media');
	await page.getByRole('link', { name: '文件', exact: true }).click();
	const prevented = await page.evaluate(() => {
		const event = new MouseEvent('mousedown', { button: 3, bubbles: true, cancelable: true });
		window.dispatchEvent(event);
		return event.defaultPrevented;
	});
	expect(prevented).toBe(false);
});
