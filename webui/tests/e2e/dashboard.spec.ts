/** 验证仪表盘媒体卡片、qB 状态和设置页基础交互。 */

import { expect, test } from '@playwright/test';

let qbControlActions: string[] = [];
let qbPollRequests = 0;
let exampleQBState = 'downloading';
let sizeIndexRebuilt = false;

test.beforeEach(async ({ page }) => {
	qbControlActions = [];
	qbPollRequests = 0;
	exampleQBState = 'downloading';
	sizeIndexRebuilt = false;
  await page.route('**/api/session', async (route) => {
    await route.fulfill({ json: { requires_login: false } });
  });
  await page.route('**/api/health', async (route) => {
    await route.fulfill({ json: { status: 'ok', addr: '127.0.0.1:8090' } });
  });
  await page.route('**/api/sites', async (route) => {
    await route.fulfill({
      json: [
        {
          id: 'demo',
          name: 'Demo Site',
          base_url: 'https://example.test',
          user_agent: 'NexusBridge E2E',
          has_cookie: true,
        },
      ],
    });
  });
  await page.route('**/api/torrents?*', async (route) => {
    await route.fulfill({
      json: {
        items: [
        {
          id: '1001',
          site_id: 'demo',
          title: 'Example Torrent',
		  cover_url: 'https://example.test/attachments/example.webp',
          category: 'Movies',
          promotion: 'Free',
          seeders: 12,
          leechers: 3,
		  size_bytes: 1073741824,
          download_url: 'https://example.test/download/1001',
		  torrent_file_saved: true,
		  info_hash_v1: '444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4',
		  qb_status: {
			available: true,
			added: true,
			source: 'hash',
			fetched_at: '2026-07-12T13:00:00Z',
			hash: '444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4',
			state: 'downloading',
			progress: 0.42,
			category: 'Movies',
			tags: 'demo,hd',
			save_path: '/downloads/movies',
			download_speed: 1048576,
			upload_speed: 1024,
			size: 1073741824,
			completed: 450971566,
		  },
        },
		{
			id: '1002', site_id: 'demo', title: 'Completed Torrent', category: 'Movies',
			download_url: 'https://example.test/download/1002', torrent_file_saved: true,
			qb_status: {
				available: true, added: true, source: 'hash', fetched_at: '2026-07-12T13:00:00Z',
				hash: 'completed-hash', state: 'uploading', progress: 1, upload_speed: 2048,
			},
		},
		{
			id: '1003', site_id: 'demo', title: 'Pending Torrent', category: 'Movies',
			download_url: 'https://example.test/download/1003', torrent_file_saved: true,
		},
        ],
        total: 3,
        offset: 0,
        limit: 50,
      },
    });
  });
  await page.route('**/api/settings/fetch', async (route) => {
    await route.fulfill({ json: { max_pages: 3 } });
  });
  await page.route('**/api/site-fetch-jobs?*', async (route) => {
    await route.fulfill({ json: [] });
  });
	await page.route('**/api/torrents/demo/1001/cover', async (route) => {
		await route.fulfill({
			contentType: 'image/webp',
			body: Buffer.from('UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA', 'base64'),
		});
	});
  await page.route('**/api/settings/qbittorrent', async (route) => {
    await route.fulfill({
      json: {
        auth_mode: 'uid',
		url: 'http://127.0.0.1:8080',
        api_key: '',
        username: '',
        user_id: '',
        password: '',
        category: '',
        tags: [],
		auto_sync: true,
		sync_interval_seconds: 2,
		inactive_sync_interval_seconds: 30,
		disconnected_sync_interval_seconds: 60,
      },
    });
  });
  await page.route('**/api/settings/llm', async (route) => {
    await route.fulfill({ json: { base_url: '', api_key: '', model: '' } });
  });
  await page.route('**/api/settings/network', async (route) => {
    await route.fulfill({ json: { mode: 'system', proxy_url: '', no_proxy: '' } });
  });
  await page.route('**/api/download-tasks', async (route) => {
    await route.fulfill({ json: [] });
  });
  await page.route('**/api/organize-tasks', async (route) => {
    await route.fulfill({ json: [] });
  });
	await page.route('**/api/qb/sync', async (route) => {
		await route.fulfill({ json: { completed: 0, organize_created: 0, torrent_matched: 1, torrent_updated: 1 } });
	});
	await page.route('**/api/torrents/demo/1001/qb-control', async (route) => {
		const body = route.request().postDataJSON() as { action: 'start' | 'stop' };
		qbControlActions.push(`1001:${body.action}`);
		const staleResponseState = exampleQBState;
		exampleQBState = body.action === 'stop' ? 'stoppedDL' : 'downloading';
		await route.fulfill({
			json: {
				available: true,
				added: true,
				source: 'hash',
				fetched_at: '2026-07-12T13:10:00Z',
				hash: '444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4',
				state: staleResponseState,
				progress: 0.42,
				category: 'Movies',
				tags: 'demo,hd',
				save_path: '/downloads/movies',
				download_speed: body.action === 'stop' ? 0 : 1048576,
				upload_speed: 1024,
			},
		});
	});
	await page.route('**/api/qb/poll?*', async (route) => {
		qbPollRequests += 1;
		await route.fulfill({
			json: {
				rid: qbPollRequests,
				full_update: qbPollRequests === 1,
				connected: true,
				updated: 1,
				removed: 0,
				updates: [{
					site_id: 'demo', torrent_id: '1001',
					qb_status: {
						available: true, added: true, source: 'hash', fetched_at: '2026-07-12T13:15:00Z',
						hash: '444a2b759acbe925ee7ecbf4ebee7ae6fd2a00d4', state: exampleQBState,
						progress: 0.42, size: 1073741824, completed: 450971566,
					},
				}],
			},
		});
	});
	await page.route('**/api/torrents/demo/1002/qb-control', async (route) => {
		const body = route.request().postDataJSON() as { action: 'start' | 'stop' };
		qbControlActions.push(`1002:${body.action}`);
		await route.fulfill({
			json: {
				available: true, added: true, source: 'hash', fetched_at: '2026-07-12T13:10:00Z',
				hash: 'completed-hash', state: body.action === 'stop' ? 'stoppedUP' : 'uploading',
				progress: 1, upload_speed: body.action === 'stop' ? 0 : 2048,
			},
		});
	});
});

test('renders media and settings pages with API data', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('heading', { name: 'NexusBridge' })).toBeVisible();
  await expect(page.getByRole('heading', { name: '媒体' })).toBeVisible();
  await expect(page.getByText('Example Torrent')).toBeVisible();
	await expect(page.getByText('/downloads/movies')).toBeVisible();
	await expect(page.getByRole('button', { name: '暂停 Example Torrent' }).locator('.lucide-square')).toBeVisible();
	await expect(page.getByRole('button', { name: '暂停 Completed Torrent' }).locator('.lucide-arrow-up')).toBeVisible();
	const firstStatus = page.locator('.card-status-control').first();
	const initialPosterBox = await page.locator('.media-poster').first().boundingBox();
	const overlayBox = await firstStatus.boundingBox();
	expect(initialPosterBox).not.toBeNull();
	expect(overlayBox).not.toBeNull();
	expect(overlayBox!.width).toBeLessThanOrEqual(40);
	expect(overlayBox!.x).toBeGreaterThanOrEqual(initialPosterBox!.x);
	expect(overlayBox!.y).toBeGreaterThanOrEqual(initialPosterBox!.y);
	expect(overlayBox!.x + overlayBox!.width).toBeLessThanOrEqual(initialPosterBox!.x + initialPosterBox!.width);
	expect(overlayBox!.y + overlayBox!.height).toBeLessThanOrEqual(initialPosterBox!.y + initialPosterBox!.height);
	await expect(firstStatus.getByText('430 MB / 1.0 GB')).not.toBeVisible();
	await firstStatus.hover();
	await expect(firstStatus.getByText('42%')).toBeVisible();
	await expect(firstStatus.getByText('430 MB / 1.0 GB')).toBeVisible();
	const expandedBox = await firstStatus.boundingBox();
	expect(expandedBox!.width).toBeGreaterThan(overlayBox!.width);
	expect(expandedBox!.x + expandedBox!.width).toBeLessThanOrEqual(initialPosterBox!.x + initialPosterBox!.width);
	await expect(firstStatus).toHaveCSS('color', 'rgb(29, 78, 216)');
	await page.getByRole('button', { name: '暂停 Example Torrent' }).click();
	await expect.poll(() => qbControlActions).toEqual(['1001:stop']);
	await expect(page.locator('.n-message')).toContainText('qB 任务已暂停');
	await expect(page.locator('.message-alert')).toHaveCount(0);
	await expect(page.getByRole('button', { name: '恢复 Example Torrent' })).toBeVisible();
	await page.getByRole('button', { name: '恢复 Example Torrent' }).click();
	await expect.poll(() => qbControlActions).toEqual(['1001:stop', '1001:start']);
	await expect(page.getByRole('button', { name: '暂停 Example Torrent' })).toBeVisible();
	const completedStatus = page.locator('.card-status-control').nth(1);
	await completedStatus.hover();
	await expect(completedStatus.getByText('做种中')).toBeVisible();
	await page.getByRole('button', { name: '暂停 Completed Torrent' }).click();
	await expect.poll(() => qbControlActions).toEqual(['1001:stop', '1001:start', '1002:stop']);
	await expect(completedStatus.getByText('已完成')).toBeVisible();
	await expect(page.getByRole('button', { name: '下载 Pending Torrent' })).toBeVisible();
	await expect(page.getByRole('button', { name: '恢复 Completed Torrent' }).locator('.lucide-check')).toBeVisible();
	await expect(page.getByRole('button', { name: '下载 Pending Torrent' }).locator('.lucide-arrow-down')).toBeVisible();
	await expect.poll(() => qbPollRequests).toBeGreaterThan(0);
	const cover = page.getByRole('img', { name: 'Example Torrent' });
	await expect(cover).toHaveAttribute('src', '/api/torrents/demo/1001/cover');
	await expect.poll(() => cover.evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0);

	await page.getByRole('button', { name: '打开媒体快捷设置' }).click();
	const slider = page.getByRole('slider').first();
	for (let index = 0; index < 7; index += 1) {
		await slider.press('ArrowRight');
	}
	await expect(slider).toHaveAttribute('aria-valuenow', '560');
	await expect.poll(() => page.evaluate(() => JSON.parse(localStorage.getItem('nexusbridge.media.display-settings') ?? '{}').cardMinWidth)).toBe(560);
	await page.getByRole('button', { name: '列表' }).click();
	await expect(page.locator('.media-list')).toHaveClass(/media-layout-list/);
	const posterBox = await page.locator('.media-poster').first().boundingBox();
	const bodyBox = await page.locator('.media-body').first().boundingBox();
	const listLayout = await page.locator('.media-poster').first().evaluate((poster) => {
		const element = poster.parentElement!;
		const style = getComputedStyle(element);
		return { className: element.className, display: style.display, direction: style.flexDirection };
	});
	expect(listLayout).toEqual({ className: 'n-card-content', display: 'flex', direction: 'row' });
	expect(posterBox).not.toBeNull();
	expect(bodyBox).not.toBeNull();
	expect(posterBox!.x).toBeLessThan(bodyBox!.x);

	await page.getByRole('button', { name: '同步 qB' }).click();
	await expect(page.getByText(/qB 同步：匹配 1/)).toBeVisible();

	await page.evaluate(() => {
		(window as typeof window & { __openedQB?: string }).open = ((url?: string | URL) => {
			(window as typeof window & { __openedQB?: string }).__openedQB = String(url ?? '');
			return null;
		}) as typeof window.open;
	});
	await page.getByRole('button', { name: '打开 qB WebUI' }).click();
	await expect.poll(() => page.evaluate(() => (window as typeof window & { __openedQB?: string }).__openedQB)).toBe('http://127.0.0.1:8080');

  await page.getByRole('link', { name: '设置', exact: true }).click();
  await expect(page).toHaveURL(/\/settings\/sites$/);
  await expect(page.getByRole('heading', { name: '站点设置' })).toBeVisible();
  await expect(page.getByText('Demo Site')).toBeVisible();

  await page.getByRole('link', { name: 'LLM' }).last().click();
  await expect(page).toHaveURL(/\/settings\/llm$/);
  await expect(page.getByRole('heading', { name: 'LLM 设置' })).toBeVisible();

  await page.getByRole('link', { name: 'qBittorrent' }).last().click();
  await expect(page).toHaveURL(/\/settings\/qbittorrent$/);
  await expect(page.getByRole('heading', { name: 'qBittorrent 设置' })).toBeVisible();
	await expect(page.getByText('状态自动刷新')).toBeVisible();
	await expect(page.getByRole('switch')).toBeChecked();
	await expect(page.getByText('已连接')).toBeVisible();
	await expect(page.locator('.qb-poll-settings input')).toHaveCount(3);
});

test('file manager keeps its own back and forward history for mouse side buttons', async ({ page }) => {
	await page.route('**/api/files/browse', async (route) => {
		const body = route.request().postDataJSON() as { path?: string };
		const path = body.path || 'C:/';
		const fixtures: Record<string, { parent?: string; entries: Array<{ name: string; path: string }> }> = {
			'C:/': { entries: [{ name: 'Folder A', path: 'C:/Folder A' }] },
			'C:/Folder A': { parent: 'C:/', entries: [{ name: 'Folder B', path: 'C:/Folder A/Folder B' }] },
			'C:/Folder A/Folder B': { parent: 'C:/Folder A', entries: [] },
		};
		const fixture = fixtures[path] ?? fixtures['C:/'];
		await route.fulfill({
			json: {
				path,
				parent: fixture.parent,
				is_root: path === 'C:/',
				qb_connected: true,
				entries: fixture.entries.map((entry) => ({ ...entry, is_dir: true, qb_tasks: [] })),
			},
		});
	});
	await page.route(/\/api\/qb\/recovery\/index(?:\/rebuild)?$/, async (route) => {
		if (route.request().method() === 'POST') sizeIndexRebuilt = true;
		await route.fulfill({
			json: sizeIndexRebuilt
				? { version: 1, total: 3, indexed: 3, pending: 0, failed: 0, processed: 1 }
				: { version: 1, total: 3, indexed: 2, pending: 1, failed: 0 },
		});
	});

	await page.goto('/');
	await page.getByRole('link', { name: '文件', exact: true }).click();
	await expect(page.getByText(/Torrent 大小索引 2\/3/)).toBeVisible();
	await page.getByRole('button', { name: '手动重建' }).click();
	await expect(page.getByText(/Torrent 大小索引 3\/3/)).toBeVisible();
	const pathInput = page.getByPlaceholder('输入本机完整目录路径');
	await expect(pathInput).toHaveValue('C:/');
	await page.getByTestId('file-entry-Folder A').dblclick();
	await expect(pathInput).toHaveValue('C:/Folder A');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Folder A');
	await page.getByTestId('file-entry-Folder B').dblclick();
	await expect(pathInput).toHaveValue('C:/Folder A/Folder B');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Folder A/Folder B');

	const backPrevented = await page.evaluate(() => {
		const event = new MouseEvent('mousedown', { button: 3, bubbles: true, cancelable: true });
		window.dispatchEvent(event);
		return event.defaultPrevented;
	});
	expect(backPrevented).toBe(true);
	await expect(pathInput).toHaveValue('C:/Folder A');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Folder A');

	const forwardPrevented = await page.evaluate(() => {
		const event = new MouseEvent('mousedown', { button: 4, bubbles: true, cancelable: true });
		window.dispatchEvent(event);
		return event.defaultPrevented;
	});
	expect(forwardPrevented).toBe(true);
	await expect(pathInput).toHaveValue('C:/Folder A/Folder B');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Folder A/Folder B');

	await page.getByTestId('file-back').click();
	await expect(pathInput).toHaveValue('C:/Folder A');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Folder A');
	await page.getByTestId('file-forward').click();
	await expect(pathInput).toHaveValue('C:/Folder A/Folder B');
	await expect.poll(() => new URL(page.url()).searchParams.get('path')).toBe('C:/Folder A/Folder B');
});

test('file manager scans once and automatically recovers unique candidates with optional web search', async ({ page }) => {
	const scanModes: string[] = [];
	let batchPayload: { web_search?: boolean; items: Array<{ path: string; site_id: string; torrent_id: string }> } | null = null;
	await page.route('**/api/files/browse', async (route) => {
		await route.fulfill({
			json: {
				path: 'C:/Downloads',
				is_root: false,
				qb_connected: true,
				entries: [{ name: 'Lost Task', path: 'C:/Downloads/Lost Task', is_dir: true, qb_tasks: [] }],
			},
		});
	});
	await page.route('**/api/qb/recovery/index', async (route) => {
		await route.fulfill({ json: { version: 1, total: 1, indexed: 1, pending: 0, failed: 0 } });
	});
	await page.route('**/api/qb/recovery/scan', async (route) => {
		const body = route.request().postDataJSON() as { search_mode: string };
		scanModes.push(body.search_mode);
		await route.fulfill({
			json: {
				path: 'C:/Downloads', evaluated: 1, matched: 1, truncated: false,
				items: [{
					path: 'C:/Downloads/Lost Task', name: 'Lost Task', is_dir: true, status: 'matched',
					preview: {
						path: 'C:/Downloads/Lost Task', folder_name: 'Lost Task', save_path: 'C:/Downloads',
						source: 'database', search_mode: body.search_mode, evaluated: 1, category: 'Recovered',
						matches: [{
							torrent: { id: '42', site_id: 'demo', title: 'Lost Task' }, source: 'database',
							original_name: 'Lost Task', save_path: 'C:/Downloads', root_folder: false,
							info_hash_v1: '1111111111111111111111111111111111111111', file_count: 1,
							total_size: 1024, match_method: 'size', mapping_complete: true,
						}],
					},
				}],
			},
		});
	});
	await page.route('**/api/qb/recovery/batch', async (route) => {
		batchPayload = route.request().postDataJSON() as typeof batchPayload;
		await route.fulfill({
			status: 202,
			json: {
				attempted: 1, recovered: 1, needs_attention: 0, failed: 0, skipped: 0,
				items: [{ path: 'C:/Downloads/Lost Task', site_id: 'demo', torrent_id: '42', status: 'recovered' }],
			},
		});
	});

	await page.goto('/');
	await page.getByRole('link', { name: '文件', exact: true }).click();
	await page.getByRole('button', { name: '扫描丢失任务' }).click();
	await expect.poll(() => scanModes).toEqual(['database']);
	await expect(page.getByRole('button', { name: '自动恢复 1 项' })).toBeVisible();

	await page.getByTestId('scan-web-search').click();
	await page.getByRole('button', { name: '重新扫描' }).click();
	await expect.poll(() => scanModes).toEqual(['database', 'database_then_site']);
	await page.getByRole('button', { name: '自动恢复 1 项' }).click();
	await expect.poll(() => batchPayload).toEqual({
		web_search: true,
		items: [{ path: 'C:/Downloads/Lost Task', site_id: 'demo', torrent_id: '42', category: 'Recovered' }],
	});
	await expect(page.getByText('批量恢复完成：成功 1，需处理 0，失败 0，跳过 0。')).toBeVisible();
});
