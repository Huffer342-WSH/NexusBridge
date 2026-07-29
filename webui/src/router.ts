/** 定义 WebUI 页面路由、标题和按页面加载边界。 */

import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';
import MediaView from './components/MediaView.vue';

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: { name: 'media' } },
  {
    path: '/media',
    name: 'media',
    component: MediaView,
    meta: { page: 'media', title: '媒体库' },
  },
  {
    path: '/series',
    name: 'series',
    component: () => import('./components/SeriesView.vue'),
    meta: { page: 'series', title: '剧集' },
  },
  {
    path: '/play/series/:series_id',
    name: 'playback-series',
    component: () => import('./components/PlaybackView.vue'),
    meta: { page: 'playback', layout: 'playback', title: '剧集播放' },
  },
  {
    path: '/play/file',
    name: 'playback-file',
    component: () => import('./components/PlaybackView.vue'),
    meta: { page: 'playback', layout: 'playback', title: '媒体播放' },
  },
  {
    path: '/play/qb/:hash',
    name: 'playback-qb',
    component: () => import('./components/PlaybackView.vue'),
    meta: { page: 'playback', layout: 'playback', title: '媒体播放' },
  },
  {
    path: '/play/:site_id/:torrent_id',
    name: 'playback',
    component: () => import('./components/PlaybackView.vue'),
    meta: { page: 'playback', layout: 'playback', title: '媒体播放' },
  },
  {
    path: '/files',
    name: 'files',
    component: () => import('./components/FileManagerView.vue'),
    meta: { page: 'files', title: '文件与恢复' },
  },
  {
    path: '/subscriptions',
    name: 'subscriptions',
    component: () => import('./components/SubscriptionsView.vue'),
    meta: { page: 'subscriptions', title: '订阅与筛选' },
  },
  {
    path: '/tasks',
    name: 'tasks',
    component: () => import('./components/TasksView.vue'),
    meta: { page: 'tasks', title: '任务' },
  },
  { path: '/settings', redirect: { name: 'settings-sites' } },
  {
    path: '/settings/sites',
    name: 'settings-sites',
    component: () => import('./components/SettingsSites.vue'),
    meta: { page: 'settings', settingsPage: 'sites', title: '设置 / 站点' },
  },
  {
    path: '/settings/network',
    name: 'settings-network',
    component: () => import('./components/SettingsNetwork.vue'),
    meta: { page: 'settings', settingsPage: 'network', title: '设置 / 网络代理' },
  },
  {
    path: '/settings/llm',
    name: 'settings-llm',
    component: () => import('./components/SettingsLLM.vue'),
    meta: { page: 'settings', settingsPage: 'llm', title: '设置 / LLM' },
  },
  {
    path: '/settings/qbittorrent',
    name: 'settings-qbittorrent',
    component: () => import('./components/SettingsQB.vue'),
    meta: { page: 'settings', settingsPage: 'qbittorrent', title: '设置 / qBittorrent' },
  },
  {
    path: '/settings/mihomo',
    name: 'settings-mihomo',
    component: () => import('./components/SettingsMihomo.vue'),
    meta: { page: 'settings', settingsPage: 'mihomo', title: '设置 / Mihomo' },
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('./components/NotFoundView.vue'),
    meta: { page: 'not-found', title: '页面不存在' },
  },
];

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  scrollBehavior: () => ({ top: 0 }),
});

router.afterEach((to) => {
  const title = typeof to.meta.title === 'string' ? to.meta.title : '';
  document.title = title ? `${title} - NexusBridge` : 'NexusBridge';
});
