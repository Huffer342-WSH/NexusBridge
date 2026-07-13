/** 定义 WebUI 与后端共享的接口数据结构。 */

export type Session = {
  mode: 'local';
  requires_login: boolean;
};

export type Health = {
  status: string;
  addr: string;
};

export type Site = {
  id: string;
  name: string;
  base_url: string;
  user_agent?: string;
  has_cookie: boolean;
};

export type SiteCredential = {
  site_id: string;
  base_url: string;
  user_agent: string;
  cookie?: string;
  has_cookie: boolean;
};

export type Torrent = {
  id: string;
  site_id: string;
  category?: string;
  category_query?: string;
  title: string;
  detail_url: string;
  download_url?: string;
  cover_url?: string;
  tags?: string[];
  tag_ids?: string[];
  description?: string;
  detail_title?: string;
  subtitle?: string;
  product_url?: string;
  detail_info_hash?: string;
  detail_description?: string;
  detail_raw_text?: string;
  detail_fetched_at?: string;
  promotion?: string;
  promotion_ends_at?: string;
  size_bytes?: number;
  seeders?: number;
  leechers?: number;
  snatches?: number;
  comments?: number;
  published_at?: string;
  published_text?: string;
  first_seen_at?: string;
  last_seen_at?: string;
	 torrent_file_saved: boolean;
	 info_hash_v1?: string;
	 info_hash_v2?: string;
	 torrent_file_error?: string;
	 qb_status?: QBTorrentStatus;
};

export type QBTorrentStatus = {
	available: boolean;
	added: boolean;
	source: string;
	error?: string;
	fetched_at: string;
	hash?: string;
	name?: string;
	state?: string;
	progress?: number;
	category?: string;
	tags?: string;
	save_path?: string;
	content_path?: string;
	download_speed?: number;
	upload_speed?: number;
	eta?: number;
	ratio?: number;
	size?: number;
	completed?: number;
	amount_left?: number;
};

export type FetchResult = {
  site_id: string;
  status: string;
  fetched: number;
  changed: number;
  matched?: number;
  download_sent?: number;
	torrent_files_saved?: number;
	torrent_files_failed?: number;
};

export type QBSyncResult = {
	completed: number;
	organize_created: number;
	checked?: number;
	linked?: number;
	missing?: number;
	torrent_matched?: number;
	torrent_updated?: number;
	torrent_removed?: number;
	detail_failed?: number;
};

export type QBTorrentUpdate = {
	site_id: string;
	torrent_id: string;
	qb_status: QBTorrentStatus;
};

export type QBPollResult = {
	rid: number;
	full_update: boolean;
	connected: boolean;
	updated: number;
	removed: number;
	updates: QBTorrentUpdate[];
};

export type QBittorrentConfig = {
  auth_mode: 'uid' | 'api_key';
  url: string;
  api_key: string;
  username: string;
  user_id: string;
  password: string;
  category: string;
  tags: string[];
	auto_sync: boolean;
	sync_interval_seconds: number;
	inactive_sync_interval_seconds: number;
	disconnected_sync_interval_seconds: number;
};

export type LLMConfig = {
  base_url: string;
  api_key: string;
  model: string;
};

export type DownloadPreview = {
  site_id: string;
  torrent_id: string;
  original_title: string;
  formatted_title: string;
  download_url: string;
  llm_response?: string;
};

export type DownloadTask = {
  id: string;
  site_id: string;
  torrent_id: string;
  status: string;
  torrent_title: string;
  download_url: string;
  rule_id: string;
  qb_hash?: string;
  content_path?: string;
  error?: string;
  created_at?: string;
  updated_at?: string;
};

export type OrganizeTask = {
  id: string;
  download_task_id: string;
  status: string;
  title: string;
  source_path: string;
  relative_dir?: string;
  filename?: string;
  target_path?: string;
  llm_response?: string;
  confidence?: number;
  error?: string;
  created_at?: string;
  updated_at?: string;
};
