/** 验证规则预览、订阅配置、qB 资源和批量下载的前端闭环。 */

import { expect, test } from '@playwright/test';

let savedRuleRequests = 0;
let savedSubscriptionRequests = 0;

const torrent = {
	id: '1001',
	site_id: 'demo',
	title: 'ASMR Free Torrent',
	category: 'ASMR',
	category_query: '401',
	tags: ['hires'],
	tag_ids: ['11'],
	promotion: 'Free',
	promotion_class: 'pro_free',
	size_bytes: 1024,
	seeders: 20,
	leechers: 2,
	snatches: 30,
	source_order: 1,
	torrent_file_saved: true,
};

const rule = {
	name: '近期免费 ASMR',
	site_ids: ['demo'],
	site_categories: ['ASMR'],
	site_tags: ['11'],
	subtitle_tags: ['hires'],
	title_expression: 'ASMR&!Blocked',
	promotions: ['pro_free'],
	min_size: 0,
	max_size: 0,
	min_seeders: 0,
	max_seeders: 0,
	min_leechers: 0,
	max_leechers: 0,
	min_snatches: 0,
	max_snatches: 0,
	published_within_minutes: 1440,
	sort_by: 'source_order',
	sort_direction: 'asc',
	action: 'download',
};

const download = {
	qb_category: 'PT/ASMR',
	save_path_template: 'D:/PT/{{site_name}}',
	qb_tags: ['nexusbridge'],
	filename_template: '{{title}}',
	paused: false,
	max_concurrent: 2,
	daily_limit: 5,
};

const subscription = {
	id: 'subscription-asmr',
	name: 'ASMR 自动订阅',
	enabled: false,
	rule_name: rule.name,
	site_ids: ['demo'],
	priority: 100,
	download,
};

const planItem = {
	torrent,
	matched: true,
	eligible: true,
	reasons: [{ code: 'matched', message: '全部筛选条件通过' }],
	plan: {
		category: 'PT/ASMR',
		save_path: 'D:/PT/Demo Site',
		tags: ['nexusbridge', '11'],
		rename: 'ASMR Free Torrent',
		original_name: 'original-name',
		paused: false,
		auto_tmm: false,
		reasons: [],
	},
};

test.beforeEach(async ({ page }) => {
	savedRuleRequests = 0;
	savedSubscriptionRequests = 0;
	await page.route('**/api/session', (route) => route.fulfill({ json: { mode: 'local', requires_login: false } }));
	await page.route('**/api/health', (route) => route.fulfill({ json: { status: 'ok', addr: '127.0.0.1:8090' } }));
	await page.route('**/api/sites', (route) => route.fulfill({
		json: [{ id: 'demo', name: 'Demo Site', base_url: 'https://example.test', has_cookie: true }],
	}));
	await page.route('**/api/torrents?*', (route) => route.fulfill({
		json: { items: [torrent], total: 1, offset: 0, limit: 50 },
	}));
	await page.route('**/api/settings/fetch', (route) => route.fulfill({ json: { max_pages: 3 } }));
	await page.route('**/api/site-fetch-jobs?*', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/settings/qbittorrent', (route) => route.fulfill({
		json: {
			auth_mode: 'uid', url: 'http://127.0.0.1:8080', api_key: '', username: '', user_id: '', password: '',
			category: '', tags: [], auto_sync: false, sync_interval_seconds: 5,
			inactive_sync_interval_seconds: 30, disconnected_sync_interval_seconds: 60,
		},
	}));
	await page.route('**/api/settings/llm', (route) => route.fulfill({ json: { base_url: '', api_key: '', model: '' } }));
	await page.route('**/api/settings/network', (route) => route.fulfill({
		json: { mode: 'system', proxy_url: '', no_proxy: '' },
	}));
	await page.route('**/api/download-tasks', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/organize-tasks', (route) => route.fulfill({ json: [] }));
	await page.route('**/api/qb/poll?*', (route) => route.fulfill({
		json: { rid: 1, full_update: true, connected: true, updated: 0, removed: 0, updates: [] },
	}));

	await page.route('**/api/rules', async (route) => {
		if (route.request().method() === 'POST') savedRuleRequests += 1;
		await route.fulfill({ json: route.request().method() === 'POST' ? route.request().postDataJSON() : [rule] });
	});
	await page.route('**/api/rules/*', async (route) => {
		if (route.request().method() === 'PUT') savedRuleRequests += 1;
		await route.fulfill({ json: route.request().postDataJSON() });
	});
	await page.route('**/api/sites/demo/filter-options', (route) => route.fulfill({
		json: {
			site_id: 'demo',
			site_categories: [{ value: 'ASMR', label: 'ASMR' }],
			site_tags: [{ value: '11', label: '11' }],
			subtitle_tags: [{ value: 'hires', label: 'hires' }],
			promotions: [{ value: 'pro_free', label: '免费' }],
		},
	}));
	await page.route('**/api/rules/preview', (route) => route.fulfill({
		json: { evaluated: 1, matched: 1, truncated: false, items: [{ torrent, matched: true, qb_state: 'added', reasons: [{ code: 'matched', message: '全部筛选条件通过' }] }] },
	}));
	await page.route('**/api/subscriptions', async (route) => {
		if (route.request().method() === 'POST') savedSubscriptionRequests += 1;
		await route.fulfill({ json: route.request().method() === 'POST' ? route.request().postDataJSON() : [subscription] });
	});
	await page.route('**/api/subscriptions/subscription-asmr/preview', (route) => route.fulfill({
		json: { subscription_id: subscription.id, evaluated: 1, matched: 1, eligible: 1, items: [planItem] },
	}));
	await page.route('**/api/subscriptions/subscription-asmr/run-once', (route) => route.fulfill({
		json: {
			id: 'run-2', subscription_id: subscription.id, site_id: 'demo', trigger: 'run-once', status: 'completed',
			fetched: 1, inserted: 1, matched: 1, attempted: 1, sent: 1, exists: 0, failed: 0, skipped: 0,
			started_at: '2026-07-15T08:00:00Z', finished_at: '2026-07-15T08:00:01Z',
		},
	}));
	await page.route('**/api/subscriptions/subscription-asmr/candidates', (route) => route.fulfill({
		json: [{
			site_id: 'demo', torrent_id: '1001', subscription_id: subscription.id, rule_name: rule.name,
			status: 'unread', source_order: 1, reason_code: 'daily_limit',
			created_at: '2026-07-15T07:00:00Z', updated_at: '2026-07-15T07:00:00Z',
		}],
	}));
	await page.route('**/api/subscription-runs', (route) => route.fulfill({
		json: [{
			id: 'run-1', subscription_id: subscription.id, site_id: 'demo', trigger: 'scheduled', status: 'completed',
			fetched: 1, inserted: 1, matched: 1, attempted: 0, sent: 0, exists: 0, failed: 0, skipped: 1,
			started_at: '2026-07-15T07:00:00Z', finished_at: '2026-07-15T07:00:01Z',
		}],
	}));
	await page.route('**/api/sites/demo/schedule', async (route) => route.fulfill({
		json: route.request().method() === 'POST'
			? route.request().postDataJSON()
			: { site_id: 'demo', enabled: false, interval_seconds: 900 },
	}));
	await page.route('**/api/qb/categories*', async (route) => {
		const items = route.request().method() === 'POST'
			? [
				{ name: 'PT/ASMR', save_path: 'D:/PT/ASMR', path_segments: ['PT', 'ASMR'] },
				{ name: 'PT/Music', save_path: 'D:/PT/Music', path_segments: ['PT', 'Music'] },
			]
			: [{ name: 'PT/ASMR', save_path: 'D:/PT/ASMR', path_segments: ['PT', 'ASMR'] }];
		await route.fulfill({ json: { items, stale: false, connected: true, synced_at: '2026-07-15T08:00:00Z' } });
	});
	await page.route('**/api/qb/tags*', (route) => route.fulfill({
		json: { items: ['nexusbridge'], stale: false, connected: true, synced_at: '2026-07-15T08:00:00Z' },
	}));
	await page.route('**/api/downloads/batch/preview', (route) => route.fulfill({ json: { items: [planItem] } }));
	await page.route('**/api/downloads/batch', (route) => route.fulfill({
		json: { attempted: 1, sent: 1, exists: 0, failed: 0, skipped: 0, tasks: [] },
	}));
});

test('previews a rule and subscription, manages qB resources, and executes a batch', async ({ page }) => {
	await page.goto('/');
	await page.getByRole('link', { name: '订阅', exact: true }).click();
	await expect(page).toHaveURL(/\/subscriptions$/);
	await expect(page.getByRole('heading', { name: '订阅与筛选' })).toBeVisible();
	await expect(page.getByTestId('rule-list')).toContainText('近期免费 ASMR');

	await page.getByTestId('save-rule').click();
	await expect.poll(() => savedRuleRequests).toBe(1);
	await page.getByTestId('preview-rule').click();
	await expect(page.getByTestId('rule-preview-results')).toContainText('ASMR Free Torrent');
	await expect(page.getByTestId('rule-preview-results')).toContainText('全部筛选条件通过');
	await page.getByRole('button', { name: '选择全部命中' }).click();

	await page.locator('.n-tabs-tab').filter({ hasText: '自动订阅' }).click();
	await expect(page.getByTestId('subscription-list')).toContainText('ASMR 自动订阅');
	await expect(page.getByText('daily_limit')).toBeVisible();
	await page.getByTestId('save-subscription').click();
	await expect.poll(() => savedSubscriptionRequests).toBe(1);
	await page.getByTestId('preview-subscription').click();
	await expect(page.getByTestId('subscription-preview-results')).toContainText('D:/PT/Demo Site');

	await page.locator('.n-tabs-tab').filter({ hasText: '周期与 qB 资源' }).click();
	await expect(page.getByTestId('qb-category-tree')).toContainText('PT/ASMR');
	await page.getByPlaceholder('PT/ASMR').fill('PT/Music');
	await page.getByTestId('create-qb-category').click();
	await expect(page.getByTestId('qb-category-tree')).toContainText('PT/Music');

	await page.locator('.n-tabs-tab').filter({ hasText: '批量下载' }).click();
	await page.getByTestId('preview-batch').click();
	await expect(page.getByTestId('batch-preview-results')).toContainText('ASMR Free Torrent');
	await page.getByTestId('execute-batch').click();
	await page.getByRole('button', { name: 'Confirm' }).click();
	await expect(page.getByTestId('batch-result')).toContainText('发送 1');
});
