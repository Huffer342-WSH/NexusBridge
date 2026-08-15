/** 定义 WebUI 与后端共享的接口数据结构。 */

export type Session = {
  mode: 'local';
  requires_login: boolean;
};

export type Health = {
  status: string;
  addr: string;
};

export type LogEntry = {
  timestamp: string;
  level: string;
  module: string;
  message: string;
  fields: string[];
  text: string;
};

export type LogSnapshot = {
  items: LogEntry[];
  total: number;
  capacity: number;
};

export type Site = {
  id: string;
  name: string;
  base_url: string;
  user_agent?: string;
  has_cookie: boolean;
};

export type MediaFilterOption = {
  value: string;
  label: string;
};

export type MediaFilterGroup = {
  name: string;
  label: string;
  options: MediaFilterOption[];
};

export type MediaFilterOptions = {
  site_id: string;
  category_label: string;
  categories: MediaFilterOption[];
  checkboxes: MediaFilterGroup[];
  promotion_label: string;
  promotions: MediaFilterOption[];
};

export type SiteCredential = {
  site_id: string;
  base_url: string;
  user_agent: string;
  cookie?: string;
  has_cookie: boolean;
};

export type SiteAttendance = {
  site_id: string;
  configured: boolean;
  enabled: boolean;
  time_of_day: string;
  timezone: string;
  last_run_at?: string;
  next_run_at?: string;
  last_error?: string;
  updated_at?: string;
};

export type SiteAttendanceInput = Pick<SiteAttendance, 'enabled' | 'time_of_day' | 'timezone'>;

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
  promotion_class?: string;
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
  source_order: number;
  sticky_level: number;
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
  task_id?: string;
  task_status?: string;
  rule_name?: string;
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

export type NetworkConfig = {
  mode: 'system' | 'manual' | 'direct';
  proxy_url: string;
  no_proxy: string;
};

export type FetchSettings = {
  max_pages: number;
};

export type SiteFetchRequest = {
  mode: 'incremental' | 'pages';
  pages?: number;
};

export type SiteFetchJob = {
  id: string;
  site_id: string;
  trigger: string;
  mode: 'incremental' | 'pages' | string;
  requested_pages: number;
  status: 'queued' | 'running' | 'completed' | 'completed_with_errors' | 'failed' | string;
  current_page: number;
  pages_fetched: number;
  fetched: number;
  inserted: number;
  changed: number;
  matched: number;
  download_sent: number;
  torrent_files_saved: number;
  torrent_files_failed: number;
  stop_reason?: string;
  error?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
};

export type TorrentPage = {
  items: Torrent[];
  offset: number;
  limit: number;
  total: number;
};

export type TorrentPageQuery = {
  offset: number;
  limit: number;
  site_id?: string;
  q?: string;
  qb_task?: 'present' | 'absent';
  qb_progress?: 'complete' | 'incomplete';
  categories?: string[];
  site_checkboxes?: string[];
  promotions?: string[];
  include_pinned: boolean;
  sort_by?: 'published_at';
  sort_direction?: 'desc';
};

export type PlaybackMediaType = 'video' | 'audio' | 'image';

export type PlaybackMedia = {
  index: number;
  name: string;
  media_type: PlaybackMediaType;
  mime_type: string;
  size: number;
  progress: number;
  selected: boolean;
  complete: boolean;
  available: boolean;
  stream_url: string;
  thumbnail_url?: string;
  subtitles?: PlaybackSubtitle[];
};

export type PlaybackSubtitle = {
  track_id: number;
  label: string;
  language?: string;
  codec: string;
  default: boolean;
  forced: boolean;
  stream_url: string;
};

export type PlaybackDirectoryFile = {
  name: string;
  path: string;
  size: number;
  modified_at: string;
  media_type?: PlaybackMediaType;
  mime_type?: string;
  thumbnail_url?: string;
  playable: boolean;
  current: boolean;
};

export type SeriesDirectory = {
  path: string;
  order: number;
  available: boolean;
  last_error?: string;
  last_scanned_at?: string;
};

export type SeriesVideo = {
  path: string;
  directory_path: string;
  relative_path: string;
  name: string;
  size: number;
  modified_at: string;
  available: boolean;
  episode_number?: number;
  episode_version?: number;
  episode_label?: string;
  thumbnail_url?: string;
};

export type SeriesSummary = {
  id: string;
  name: string;
  episode_number_detection: boolean;
  directories: SeriesDirectory[];
  video_count: number;
  available_video_count: number;
  last_selected_path?: string;
  last_scanned_at?: string;
  scan_errors: string[];
  created_at: string;
  updated_at: string;
};

export type SeriesDetail = SeriesSummary & {
  videos: SeriesVideo[];
};

export type SeriesSaveRequest = {
  name: string;
  directories: string[];
  episode_number_detection: boolean;
};

export type SeriesSelectionRequest = {
  path: string;
};

export type MediaLibraryKind = 'collection' | 'series';

export type MediaLibrarySettingsOverride = {
  episode_number_detection?: boolean;
  auto_detect_series?: boolean;
};

export type MediaLibrarySettings = {
  episode_number_detection: boolean;
  auto_detect_series: boolean;
};

export type MediaLibrarySummary = {
  id: string;
  name: string;
  kind: MediaLibraryKind;
  parent_id?: string;
  directories: SeriesDirectory[];
  settings: MediaLibrarySettingsOverride;
  effective_settings: MediaLibrarySettings;
  video_count: number;
  available_video_count: number;
  last_scanned_at?: string;
  scan_errors: string[];
  created_at: string;
  updated_at: string;
};

export type MediaLibraryDetail = MediaLibrarySummary & {
  videos: SeriesVideo[];
};

export type MediaLibrarySaveRequest = {
  name: string;
  kind: MediaLibraryKind;
  parent_id: string;
  directories: string[];
  settings: MediaLibrarySettingsOverride;
};

export type PlaybackContext = {
  source: 'torrent' | 'qb' | 'file';
  title: string;
  current_path: string;
  current_directory: string;
  torrent?: Torrent;
  qb_status?: QBTorrentStatus;
  qb_hash?: string;
  files: PlaybackMedia[];
  directory_files: PlaybackDirectoryFile[];
  default_file_index?: number;
  current_file_index?: number;
  series?: SeriesSummary;
  series_files?: SeriesVideo[];
};

export type PlaybackTorrent = {
  id: string;
  site_id: string;
  title: string;
  cover_url?: string;
  category?: string;
  published_at?: string;
};

export type MihomoProvider = {
  name: string;
  has_url: boolean;
};

export type MihomoSettings = {
  config_dir: string;
  default_config_dir: string;
  config_path: string;
  providers: MihomoProvider[];
};

export type MihomoProviderCreateRequest = {
  config_dir: string;
  name: string;
  url: string;
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
  rule_name: string;
  subscription_id?: string;
  trigger?: string;
  qb_hash?: string;
  content_path?: string;
  category?: string;
  save_path?: string;
  tags?: string[];
  rename?: string;
  paused: boolean;
  reason_code?: string;
  attempt_count: number;
  retry_count: number;
  last_attempt_at?: string;
  sent_at?: string;
  error?: string;
  created_at: string;
  updated_at: string;
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

export type RuleSortField = 'source_order' | 'published_at' | 'size_bytes' | 'seeders' | 'leechers' | 'snatches';

/** 可复用于预览、订阅和手动批量下载的数据库筛选规则。 */
export type FilterRule = {
  name: string;
  site_ids: string[];
  site_categories: string[];
  site_tags: string[];
  subtitle_tags: string[];
  title_expression: string;
  promotions: string[];
  min_size: number;
  max_size: number;
  min_seeders: number;
  max_seeders: number;
  min_leechers: number;
  max_leechers: number;
  min_snatches: number;
  max_snatches: number;
  published_within_minutes: number;
  sort_by: RuleSortField | string;
  sort_direction: 'asc' | 'desc' | string;
  action: string;
};

export type RuleReason = {
  code: string;
  field?: string;
  message: string;
};

export type RulePreviewRequest = {
  rule: FilterRule;
  site_id?: string;
  limit?: number;
  offset?: number;
};

export type RulePreviewItem = {
  torrent: Torrent;
  matched: boolean;
  reasons: RuleReason[];
  qb_state: 'added' | 'not_added' | 'unknown' | string;
};

export type RulePreviewResult = {
  evaluated: number;
  matched: number;
  items: RulePreviewItem[];
  truncated: boolean;
};

export type RuleSelectOption = {
  value: string;
  label: string;
};

export type RuleFilterOptions = {
  site_id: string;
  site_categories: RuleSelectOption[];
  site_tags: RuleSelectOption[];
  subtitle_tags: RuleSelectOption[];
  promotions: RuleSelectOption[];
};

export type DownloadOptions = {
  qb_category: string;
  save_path_template: string;
  qb_tags: string[];
  filename_template: string;
  paused: boolean;
  max_concurrent: number;
  daily_limit: number;
};

export type Subscription = {
  id: string;
  name: string;
  enabled: boolean;
  rule_name: string;
  site_ids: string[];
  priority: number;
  download: DownloadOptions;
  created_at?: string;
  updated_at?: string;
};

export type SiteSchedule = {
  site_id: string;
  enabled: boolean;
  interval_seconds: number;
  last_run_at?: string;
  next_run_at?: string;
  last_error?: string;
  updated_at?: string;
};

export type SiteScheduleInput = Pick<SiteSchedule, 'enabled' | 'interval_seconds'>;

export type DeletedResult = {
  deleted: boolean;
};

export type TorrentKey = {
  site_id: string;
  torrent_id: string;
};

export type DownloadPlan = {
  category: string;
  save_path: string;
  tags: string[];
  rename: string;
  original_name?: string;
  paused: boolean;
  auto_tmm?: boolean;
  reasons: RuleReason[];
};

export type SubscriptionPreviewItem = {
  torrent: Torrent;
  matched: boolean;
  eligible: boolean;
  reasons: RuleReason[];
  plan: DownloadPlan;
};

export type SubscriptionPreview = {
  subscription_id: string;
  evaluated: number;
  matched: number;
  eligible: number;
  items: SubscriptionPreviewItem[];
};

export type SubscriptionCandidate = {
  site_id: string;
  torrent_id: string;
  subscription_id: string;
  rule_name: string;
  status: 'unread' | 'processing' | 'processed' | 'failed' | string;
  source_order: number;
  reason_code?: string;
  created_at: string;
  updated_at: string;
};

export type SubscriptionRun = {
  id: string;
  subscription_id?: string;
  site_id?: string;
  trigger: 'manual' | 'homepage' | 'scheduled' | 'manual-batch' | string;
  status: string;
  fetched: number;
  inserted: number;
  matched: number;
  attempted: number;
  sent: number;
  exists: number;
  failed: number;
  skipped: number;
  error?: string;
  started_at: string;
  finished_at?: string;
};

export type QBCategory = {
  name: string;
  save_path: string;
  path_segments: string[];
  synced_at?: string;
};

export type QBCategoriesResult = {
  items: QBCategory[];
  stale: boolean;
  connected: boolean;
  error?: string;
  synced_at?: string;
};

export type QBTagsResult = {
  items: string[];
  stale: boolean;
  connected: boolean;
  error?: string;
  synced_at?: string;
};

export type RecoveryPreviewRequest = {
  path: string;
  site_ids?: string[];
  search_mode?: RecoverySearchMode;
  torrent_url?: string;
  category?: string;
};

export type RecoveryRequest = {
  path: string;
  site_id?: string;
  torrent_id?: string;
  search_mode?: RecoverySearchMode;
  torrent_url?: string;
  category?: string;
};

export type RecoverySearchMode = 'database' | 'site' | 'database_then_site' | 'url';

export type RecoveryMatch = {
  torrent: Torrent;
  source: 'database' | 'site_search' | 'url';
  original_name: string;
  save_path: string;
  root_folder: boolean;
  info_hash_v1?: string;
  info_hash_v2?: string;
  file_count: number;
  total_size: number;
  match_method: 'size';
  mapping_complete: boolean;
};

export type TorrentSizeIndexStatus = {
  version: number;
  total: number;
  indexed: number;
  pending: number;
  failed: number;
  processed?: number;
  last_error?: string;
};

export type RecoverySearchAttempt = {
  site_id: string;
  keyword: string;
  status: 'ok' | 'failed';
  candidates: number;
  files_saved: number;
  files_failed: number;
  error?: string;
};

export type RecoveryPreview = {
  path: string;
  folder_name: string;
  save_path: string;
  source: 'database' | 'site_search' | 'url' | 'none';
  search_mode: RecoverySearchMode;
  category?: string;
  evaluated: number;
  matches: RecoveryMatch[];
  search_attempts?: RecoverySearchAttempt[];
};

export type RecoveryResult = {
  path: string;
  save_path: string;
  category?: string;
  match: RecoveryMatch;
  qb_status: QBTorrentStatus;
  verification_status: 'verified' | 'failed' | 'start_failed';
  verification_error?: string;
  recheck_attempts: number;
  started: boolean;
  can_start: boolean;
  can_delete: boolean;
};

export type RecoveryActionResult = {
  hash: string;
  action: 'start' | 'delete';
  deleted: boolean;
  qb_status?: QBTorrentStatus;
};

export type FileQBTask = {
  hash: string;
  name: string;
  state: string;
  category?: string;
  save_path?: string;
  content_path: string;
};

export type FileEntry = {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  modified_at?: string;
  media_type?: PlaybackMediaType;
  thumbnail_url?: string;
  qb_tasks: FileQBTask[];
};

export type FileBrowseResult = {
  path: string;
  parent?: string;
  is_root: boolean;
  qb_connected: boolean;
  qb_error?: string;
  entries: FileEntry[];
};

export type RecoveryScanRequest = {
  path: string;
  site_ids?: string[];
  search_mode?: Exclude<RecoverySearchMode, 'url'>;
  max_depth?: number;
  limit?: number;
};

export type RecoveryScanItem = {
  path: string;
  name: string;
  is_dir: boolean;
  status: 'matched' | 'ambiguous' | 'none' | 'error';
  reason?: string;
  preview: RecoveryPreview;
};

export type RecoveryScanResult = {
  path: string;
  evaluated: number;
  matched: number;
  truncated: boolean;
  items: RecoveryScanItem[];
};

export type RecoveryBatchItemRequest = {
  path: string;
  site_id: string;
  torrent_id: string;
  category?: string;
};

export type RecoveryBatchRequest = {
  items: RecoveryBatchItemRequest[];
  web_search?: boolean;
};

export type RecoveryBatchItemResult = {
  path: string;
  site_id: string;
  torrent_id: string;
  status: 'recovered' | 'needs_attention' | 'failed' | 'skipped';
  error?: string;
  result?: RecoveryResult;
};

export type RecoveryBatchResult = {
  attempted: number;
  recovered: number;
  needs_attention: number;
  failed: number;
  skipped: number;
  items: RecoveryBatchItemResult[];
};

type BatchDownloadSelection = { torrents: TorrentKey[] };

export type BatchDownloadRequest = BatchDownloadSelection &
  ({ subscription_id: string; options?: never } | { subscription_id?: never; options: DownloadOptions });

export type BatchDownloadPreview = {
  items: SubscriptionPreviewItem[];
};

export type BatchDownloadResult = {
  attempted: number;
  sent: number;
  exists: number;
  failed: number;
  skipped: number;
  tasks: DownloadTask[];
};
