# API 说明

HTTP 服务默认绑定 `127.0.0.1:8090`。

开发版默认从项目 `data/config.json` 读取监听配置；正式版从统一运行数据目录读取。Linux 发布程序已嵌入 WebUI，运行 `nexusbridge serve` 不需要额外的 `webui/dist` 目录。

浏览器版和桌面版使用完全相同的 `/api`、`/rss` 契约。浏览器版由 `internal/server` 监听 HTTP 地址；Wails v3 桌面版不开放端口，而是在 AssetServer Middleware 中把这两个路径前缀交给同一个 `Server.Handler()`。前端 `src/api.ts` 无需区分运行环境。

## 接口契约维护

- `docs/api/openapi.yaml` 是已实现 HTTP API 的机器可读契约，前端类型、接口评审和自动化检查应优先以它为准。
- 本文件是面向开发者阅读的接口说明，描述接口用途、敏感字段规则和业务注意事项。
- `docs/api/requests.md` 是前端提出后端新接口或接口调整需求的入口，尚未实现的接口不要直接写入本文件作为已实现能力。
- 后端新增、删除或修改接口时，必须同步更新 `internal/server` 路由、`docs/api/openapi.yaml`、本文件、前端调用类型和相关测试。

前端可用以下命令按需从 OpenAPI 生成 TypeScript 类型：

```sh
cd webui
pnpm dlx openapi-typescript ../docs/api/openapi.yaml -o src/generated/api-types.ts
```

## 健康检查

`GET /api/health`

返回 API 状态和监听地址。

## 会话

`GET /api/session`

返回 WebUI 是否需要展示登录界面。

`POST /api/session/login`

接收 `{ "username": "...", "password": "..." }`，返回本地占位 token。

## 站点

`GET /api/sites`

返回已加载站点，不包含 cookie 等敏感字段。站点只从 `sites_dir` 目录加载。

`GET /api/sites/{site_id}/credential`

返回站点凭据状态，包括 `user_agent` 和 `has_cookie`，不会返回 cookie 明文。

`POST /api/sites/{site_id}/credential`

保存指定站点的 `cookie` 和 `user_agent`。`cookie` 使用浏览器请求头格式，例如 `name=value; name2=value2`；如果 cookie 留空，只更新 user-agent。

`POST /api/sites/{site_id}/fetch`

读取数据库中的站点 cookie，抓取 `torrents.php`，按页面顺序写入本地缓存和 SQLite，并补齐缺失的 torrent BLOB、v1/v2 hash 与原始 `info.name`。响应中的 `inserted` 和 `changed` 分别表示新入库和既有记录变化；该接口不会匹配订阅或写入 qB。

`POST /api/sites/{site_id}/run-once`

立即抓取一次站点，并执行该站点的已启用订阅。该流程和后台计划使用相同的首个订阅独占、配额与幂等规则。

`GET /api/sites/{site_id}/schedule`

返回站点独立的后台周期配置。未保存时返回 `enabled=false`、`interval_seconds=900` 的默认值。

`POST /api/sites/{site_id}/schedule`

保存站点周期配置。`interval_seconds` 允许 `60` 至 `86400`，并使用整分钟步长；站点计划默认关闭。后台调度只由常驻 WebUI/CLI 服务和桌面进程启动，CLI 的单次命令不会在命令结束后保留后台任务。

`GET /api/sites/{site_id}/filter-options`

返回规则编辑器的单站点选项。站点分类、站点标签和副标题标签来自该站点已抓取种子的本地集合；促销来自站点 JSON 中标记为 `role=promotion` 的检索下拉框。接口只读取本地数据，不触发站点抓取。

## 种子

`GET /api/torrents`

返回 SQLite 中的种子元数据、最近抓取列表序号 `source_order`、列表页副标题、从副标题拆出的 `tags`、官方站点标签 `tag_ids`、按需详情页字段（商品链接、详情页 hash、简介）、torrent 文件保存状态、v1/v2 hash 和最近一次持久化的 `qb_status`；不会返回 torrent BLOB，普通请求不会访问 qB。

预留查询参数：

- `site_id`
- `q`

`GET /api/torrents/{site_id}/{torrent_id}/qb-status`

按本地 v1 hash 实时查询单个 qB 任务并回写状态快照；只有显式传入 `weak_match=true` 时才允许标题兼容匹配。

`POST /api/torrents/{site_id}/{torrent_id}/qb-control`

接收 `{ "action": "start" }` 或 `{ "action": "stop" }`，按本地 v1 hash 恢复或暂停对应 qB 任务，操作成功后立即查询并返回最新 `qb_status` 快照。媒体状态条在接口成功后先切换到目标状态，并在短暂保护期后用增量同步结果校准，避免 qB 瞬时旧状态造成界面回跳。

`GET /api/torrents/{site_id}/{torrent_id}/cover`

使用数据库中的封面地址、站点 user-agent、Referer 和统一 `request_rules` Cookie 策略代理图片，供 WebUI 以同源地址加载 WebP 等受鉴权、防盗链或混合内容限制的封面。跨主机只发送站点定义允许的 Cookie 名称，响应仅允许图片类型，单张最大 10 MiB。

`POST /api/torrents/{site_id}/{torrent_id}/download/preview`

读取本地种子记录，调用 LLM 对标题进行提取和整理，返回 `original_title`、`formatted_title`、`download_url`。需要先在 WebUI 或数据库中保存 LLM 配置。

`POST /api/torrents/{site_id}/{torrent_id}/download`

接收 `{ "formatted_title": "..." }`，服务端先使用站点 cookie 下载 `.torrent` 文件并计算 v1 info hash，再通过 qBittorrent `torrents/add` 上传文件。若响应包含 `added_torrent_ids` 则直接核对并写入 `download_tasks.qb_hash`；旧版响应或 `409 Conflict` 会按本地 hash 查询协调。同一 `site_id + torrent_id + manual` 不重复发送；纯 v2 torrent 当前会返回失败任务。

## 筛选规则

`GET /api/rules`

返回数据库中的可复用筛选规则。`/api/rules` 路径保持兼容，领域含义为 `FilterRule`；规则本身不包含站点周期和 qB 下载目标。

`POST /api/rules`

保存筛选规则。`name` 是大小写不敏感的唯一标识，不再存在独立规则 ID 或“自动订阅可用”开关，所有已保存规则都能被订阅引用。筛选字段包括 `site_ids`、`site_categories`、`site_tags`、`subtitle_tags`、`title_expression`、`promotions`、体积上下限、做种/下载/完成数上下限和 `published_within_minutes`；`action` 仍只接受 `download`。

`site_ids` 和 `site_categories` 各字段内部按 OR 匹配，`site_tags` 与 `subtitle_tags` 各自要求种子具备全部指定值，`promotions` 内部按 OR 匹配，不同字段之间按 AND 匹配。任一站点来源条件非空时，`site_ids` 必须且只能包含一个站点。所有 `0` 数值边界表示不限。

`title_expression` 只对主标题执行大小写不敏感的子串判断，支持 `!`、`&`、`|` 和括号，优先级依次为 `!`、`&`、`|`。双引号可包裹包含运算符的关键词，例如 `"A&B"`；保存和预览都会拒绝语法错误。

`sort_by` 接受 `source_order`、`published_at`、`size_bytes`、`seeders`、`leechers` 或 `snatches`，`sort_direction` 接受 `asc` 或 `desc`。默认 `source_order/asc`，即保持站点列表从上到下；相同排序值使用站点 ID 和种子 ID 稳定排序。

`PUT /api/rules/{rule_name}`

更新由路径中原名称定位的规则。允许修改名称，并在同一事务中级联更新订阅、候选和本地下载任务引用。

`DELETE /api/rules/{rule_name}`

删除筛选规则。仍有订阅引用该规则时拒绝删除。

`POST /api/rules/preview`

接收 `{ "rule": {...}, "site_id": "...", "limit": 100, "offset": 0 }`。规则可以尚未保存；接口只分页读取 SQLite 并返回 `matched`、全部稳定 `reasons[].code` 和基于本地快照/任务的 `qb_state`。命中项稳定排在排除项之前；接口不会抓站、写候选/下载任务或访问 qB。

## 自动订阅

`GET /api/subscriptions`

返回全部订阅，顺序为 `priority DESC, id ASC`。

`POST /api/subscriptions`

创建或更新订阅。订阅必须引用已保存规则并明确提供 `site_ids`；站点不得超出规则的 `site_ids` 范围。自动订阅默认关闭，启用订阅时 `download.qb_category` 不能为空。

下载配置字段：

- `qb_category`：qB 中已经存在的完整分类字符串。
- `save_path_template`：可选目标路径模板。
- `qb_tags`：订阅标签模板列表。
- `filename_template`：映射 qB `rename` 的任务名称模板；为空时省略 `rename` 并保留 torrent 原始 `info.name`。
- `paused`：新增任务是否暂停，默认 `false`。
- `max_concurrent`、`daily_limit`：`0` 表示不限；自动和订阅级 run-once 会遵守配额。

每日配额按服务所在时区的自然日统计 `sent`，`exists/failed/skipped` 不计；并发配额统计本地 pending 加该订阅在 qB 中尚未完成的任务。配额检查与 pending 领取原子化，配额跳过不会增加 `retry_count`。

模板只支持以下占位符，不支持表达式或 LLM；未知或语法不完整的占位符在保存时拒绝：

`{{site_id}}`、`{{site_name}}`、`{{torrent_id}}`、`{{category}}`、`{{category_query}}`、`{{rule_name}}`、`{{subscription_name}}`、`{{title}}`、`{{detail_title}}`、`{{subtitle}}`。

路径模板只清理占位符值，保留模板自身的盘符、根路径和 `/`。标签按全局 qB 标签、订阅标签、种子 `tag_ids`、副标题 `tags` 的顺序合并，忽略空值并大小写不敏感去重；逗号和控制字符替换为 `_`，每项最多 64 个字符。文件名会清理跨平台非法字符和结尾点/空格，最长 240 个字符；同名不同 hash 时预览和执行都会追加稳定的 `site_id-torrent_id` 后缀并返回原因。

`DELETE /api/subscriptions/{subscription_id}`

删除订阅配置；历史运行记录和下载任务继续保留，尚未处理的候选会释放并按当前启用订阅优先级重新分配。

`POST /api/subscriptions/{subscription_id}/preview?limit=100`

只读预览已保存订阅。返回规则是否命中、最终 category/save path/tags/rename/paused、`eligible` 以及全部阻塞原因。预览会实时验证 qB 分类和名称冲突，但不会改变候选或任务状态。

`POST /api/subscriptions/{subscription_id}/run-once`

立即执行一次订阅，使用与计划调度相同的抓取、候选、排序、配额和幂等流程，返回 `SubscriptionRun` 统计。

`GET /api/subscriptions/{subscription_id}/candidates?status=unread`

返回首个命中订阅独占的候选队列。候选状态为 `unread`、`processing`、`processed` 或 `failed`；配额不足、qB 离线或分类缺失时保持 `unread`，下一次刷新继续尝试，不回退给低优先级订阅。

`GET /api/subscription-runs?subscription_id={id}`

返回 `scheduled`、`run-once` 或 `manual-batch` 运行记录及 `fetched/inserted/matched/attempted/sent/exists/failed/skipped` 统计。

## qBittorrent

`GET /api/settings/qbittorrent`

返回 qBittorrent WebUI 配置。响应不会返回已保存的密码或 API Key。

`POST /api/settings/qbittorrent`

保存 qBittorrent WebUI 配置，字段包括 `auth_mode`、`url`、`api_key`、`user_id`、`username`、`password`、`category`、`tags`。`auth_mode=uid` 使用账号密码登录；`auth_mode=api_key` 使用 API Key 请求头。如果 `password` 或 `api_key` 为空，服务端会保留数据库中已有值。

`GET /api/settings/llm`

返回 LLM 配置。响应不会返回已保存的 API Key。

`POST /api/settings/llm`

保存 OpenAI-compatible LLM 配置，字段包括 `base_url`、`api_key`、`model`。如果 `api_key` 为空，服务端会保留数据库中已有值。

`POST /api/qb/sync`

读取全部 qB 任务并按 v1 hash 匹配本地种子，查询匹配任务 properties，更新 qB 快照；同时保留下载完成检测和整理任务创建。响应增加 `torrent_matched`、`torrent_updated`、`torrent_removed` 和 `detail_failed`。

`GET /api/qb/poll?rid={rid}`

代理 qB `sync/maindata` 增量接口，按 hash 将完整或部分 torrent 字段合并到数据库快照，返回新的 `rid` 和前端需要更新的本地种子状态。qB 不可连接时返回 `502`，前端按断连间隔退避；该轻量接口不查询 properties，也不创建整理任务。

`GET /api/qb/categories?refresh=true`

返回 qB 分类快照。qB 分类只有完整名称，例如 `PT/ASMR`；响应额外提供从 `/` 拆分的 `path_segments` 供 WebUI 树状展示，不新增独立父分类或子分类字段。`refresh=true` 强制同步；qB 离线且已有缓存时返回 `stale=true`、`connected=false` 和 `error`，没有可用缓存时返回 `502`。

分类创建和下载参数遵循 [qBittorrent WebUI API](https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-%28qBittorrent-5.0%29) 的完整分类名契约。

`POST /api/qb/categories`

接收 `{ "name": "PT/ASMR", "save_path": "D:/Media/ASMR" }` 并只创建该完整分类名，不隐式创建 `PT`。创建成功后返回重新同步的分类快照。

`GET /api/qb/tags?refresh=true`

返回 qB 标签快照，离线回退规则与分类接口相同。

`POST /api/qb/tags`

接收 `{ "tags": ["nexusbridge", "audio"] }`，在 qB 中创建标签并返回重新同步的快照。订阅执行发送前也会创建下载计划中尚不存在的标签。

`POST /api/qb/recovery/preview`

接收 `{ "path": "D:/Downloads/example", "site_ids": ["kamept"], "search_mode": "database_then_site" }`。`site_ids` 可选；`search_mode` 支持仅数据库 `database`、仅站点网页 `site` 和数据库未命中再查站点 `database_then_site`，省略时使用最后一种。`path` 可以是 NexusBridge 可直接读取的完整数据目录或单个文件。三种检索来源以及直接 URL 最终都要求目标的完整文件大小多重集合与 torrent 一致，文件数量和重复大小次数也必须相同。数据库通过大小倒排索引只读取可能匹配的 BLOB；站点网页模式合并最多 5 个完整名称、去扩展名、括号标题和分词变体的第一页结果，站点展示总大小按 5%（最少 1 MiB）容差预筛 torrent 下载候选，最终仍以 torrent 内精确文件大小为准。

也可传入 `{ "path": "D:/Downloads/example", "torrent_url": "https://tracker.example/download.php?id=123", "site_ids": ["kamept"] }`。URL 模式跳过数据库和搜索页；指定站点时 URL 必须命中该站主域名或 `request_rules`，未指定时自动从全部站点中选择最长匹配。私站 URL 复用统一站点请求入口和 Cookie 白名单策略；未匹配任何站点的 HTTP(S) URL 才使用无 Cookie 请求。路径和文件名只用于优先建立同尺寸文件映射；无名称线索时使用稳定的一一映射，最终由 qB 强制校验确认内容。响应中的 `match_method` 为 `size`，`mapping_complete=true` 表示全部 torrent 文件均已映射。`category` 来自显式覆盖，或 `save_path` 与 qB 分类保存目录的精确匹配。预览不调用 qB 写接口。

`GET /api/qb/recovery/index`

返回已保存 torrent BLOB 的大小索引版本、总数、已索引数、待处理数和失败数。数据库恢复和数据库扫描要求 `pending=0`；站点模式与直接 URL 不依赖旧数据索引。

`POST /api/qb/recovery/index/rebuild`

由用户手动同步触发一次旧 BLOB 重建，不启动后台任务。接口逐个解析现有 torrent，并以短事务替换单个种子的索引；单项解析失败会记录在该 BLOB 上并继续其他项。后续正常保存、覆盖或通过存储接口删除 torrent BLOB 时会自动维护，不需要再次批量补齐。

`POST /api/qb/recovery`

接收 `{ "path": "D:/Downloads/example", "site_id": "kamept", "search_mode": "site", "category": "PT/ASMR" }`，也支持与预览相同的 `torrent_url`。只有 `path` 必填；`site_id` 用于可选的单站点筛选，`torrent_id` 仅作为多候选时的可选消歧字段。执行前会再次比较完整大小集合并重新生成映射。所有来源都以目标父目录作为 `savepath`，设置 `root_folder=false`、`paused=true`、`skip_checking=true` 和 `autoTMM=false`，再用 qB `renameFile` 把 torrent 文件映射到磁盘已有结构；NexusBridge 不移动、复制或重命名磁盘文件。分类和路径全部设置后最多三次触发 qB `recheck`，每次只有观察到 `checking*` 状态才视为生效。校验结束且任务为 `pausedUP/stoppedUP`、进度 100%、剩余 0 字节时才调用 `start` 做种。校验触发失败、超时或内容不完整时响应保留同一个暂停任务，返回 `verification_error`、重试次数和 `can_start/can_delete`，不会再次添加；配置阶段失败才以 `deleteFiles=false` 清理本次新增任务。分类优先使用请求值，其次使用最终保存目录精确推断值，最后回退 qB 全局默认分类。当前不支持纯 v2 torrent 恢复，也不做 piece SHA1 预校验。

`POST /api/qb/recovery/{hash}/action`

校验失败后接收 `{ "action": "start" }` 或 `{ "action": "delete" }`。`start` 明确忽略校验失败并启动已存在任务；`delete` 始终使用 `deleteFiles=false`，只删除 qB 任务并保留磁盘文件。该接口不添加新任务。

`POST /api/files/browse`

接收 `{ "path": "D:/Downloads" }`；空路径返回可用文件系统根目录。响应按目录优先排序并返回每个文件或目录的完整路径、类型、大小、修改时间，以及 `content_path` 与该条目完整路径精确相等的 qB 任务。qB 不可用时仍返回文件列表，并通过 `qb_connected=false` 和 `qb_error` 提供诊断。

`POST /api/qb/recovery/scan`

接收 `{ "path": "D:/Downloads", "search_mode": "database", "max_depth": 1, "limit": 200 }`。接口递归评估不属于现有 qB 任务的文件和目录，默认只查询本地数据库；用户启用网页搜索时前端改用 `database_then_site`。深度最大 5、条目上限 500。每项返回 `matched`、`ambiguous`、`none` 或 `error` 以及完整恢复预览。扫描只读，不会自动恢复任何任务。

`POST /api/qb/recovery/batch`

接收 `{ "web_search": false, "items": [{ "path": "D:/Downloads/example", "site_id": "kamept", "torrent_id": "123" }] }`。仅接收扫描阶段已唯一确定的候选，按顺序串行执行现有单任务恢复流程；默认重新验证时只查数据库，`web_search=true` 时使用数据库优先、站点网页兜底。相同路径或相同 torrent 在一个批次中只执行一次，最多 500 项。单项失败不终止批次，响应统计 `recovered`、`needs_attention`、`failed`、`skipped`；`needs_attention` 的任务保持暂停并返回 `can_start/can_delete`，不会自动开始下载。

`GET /api/download-tasks`

返回下载任务及其 `subscription_id`、触发来源、最终 category/save path/tags/rename/paused 快照、稳定原因代码、尝试/重试次数和发送时间。状态包括 `pending`、短暂可见的 `processing`、`sent`、`exists`、`failed` 和 `skipped`。

同一 `site_id + torrent_id + rule_name` 只创建一个下载任务。发送前按本地 v1 hash 查询 qB：已存在时记录为 `exists` 且不修改原任务；新增成功为 `sent`；真实新增错误为 `failed`；配额、qB 离线或分类缺失为 `skipped`。纯 v2 torrent 不能执行当前的精确发送验证，会明确失败而不会伪装成成功。

`POST /api/download-tasks/{task_id}/retry`

使用任务已保存的 category/save path/tags/rename/paused 快照重试真实发送失败，不重新套用已修改的订阅模板。配额跳过不增加 `retry_count`。

`POST /api/downloads/batch/preview`

`POST /api/downloads/batch`

两个接口都接收种子联合键 `torrents: [{ "site_id": "...", "torrent_id": "..." }]`，并且必须二选一：

- `subscription_id`：快速套用订阅的 qB 下载配置，但忽略自动配额、周期和优先级。
- `options`：使用本次临时提交的 `DownloadOptions`。

preview 每次重新计算最终下载计划且不写状态；execute 会再次校验并把计划快照保存到下载任务，响应汇总 `attempted/sent/exists/failed/skipped`。

## 整理

`GET /api/organize-tasks`

返回待整理或已整理的任务。

`POST /api/organize/pending`

调用 OpenAI-compatible LLM API 生成媒体库相对路径，并按配置执行 dry-run 或硬链接。

## RSS

`GET /rss/{feed_id}`

预留 RSS 生成接口。当前骨架返回 `501 Not Implemented`。

