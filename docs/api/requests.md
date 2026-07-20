# 前端接口需求

本文档用于记录前端需要后端新增或调整的接口。这里可以写尚未实现的需求；已实现接口必须同步进入 `docs/api/openapi.yaml` 和 `docs/api.md`。

## 状态

- `proposed`：前端提出需求，后端尚未确认。
- `accepted`：后端接受需求，接口形状已确认，等待实现。
- `implemented`：后端已实现并更新 OpenAPI、文档和测试。
- `rejected`：暂不实现，必须写明原因。

## 编写要求

- 每个需求使用一个二级标题，标题包含状态。
- 必须说明前端场景、期望接口、请求、响应和验收标准。
- 不要在本文档写真实 cookie、账号、密码、API Key 或站点私密链接。
- 需求被接受后，先补 `docs/api/openapi.yaml` 草案，再实现后端。

## [implemented] 从保留文件夹恢复 qB 种子

### 前端场景

qB 任务被意外删除，但完整数据目录仍保留在 NexusBridge 可访问的本地文件系统中。后端 API 与 WebUI 文件管理器均已对接。

### 期望接口

`POST /api/qb/recovery/preview`

请求：

```json
{
  "path": "D:/Downloads/example",
  "site_ids": ["kamept"],
  "search_mode": "database_then_site"
}
```

`site_ids` 可选；未指定时，数据库模式查询全部站点索引，站点模式遍历全部已配置站点。`search_mode` 可选，支持仅本地数据库 `database`、仅站点网页 `site`、先数据库再站点 `database_then_site`，默认使用 `database_then_site`。数据库使用文件大小倒排索引缩小 BLOB 候选；站点网页合并最多 5 个文件名或标题变体的第一页结果，先按站点展示总大小的 5%（最少 1 MiB）容差缩小 torrent 文件下载候选，再以 torrent 内精确大小集合确认。响应包含标准化路径、实际模式、`save_path`、可恢复候选、已评估候选数和站点搜索诊断。预览不调用 qB 写接口。

`POST /api/qb/recovery`

请求：

```json
{
  "path": "D:/Downloads/example",
  "site_id": "kamept",
  "search_mode": "site"
}
```

数据库和站点执行接口使用与预览相同的三种模式；`site_id` 可选，未指定时站点检索遍历全部已配置站点；`torrent_id` 仅作为发生多候选时的可选消歧字段，正常恢复无需预先知道。后端只在完整大小集合候选唯一时自动执行，否则返回无匹配或多候选错误。执行前再次验证大小并生成全部文件映射。所有来源都以目标父目录作为 qB `savepath` 并传入 `root_folder=false`、`autoTMM=false`、`paused=true` 和 `skip_checking=true`；添加后只调用 qB `renameFile` 改任务内部路径，不改动磁盘结构，随后显式强制校验，确认完整后才调用 `start`。

### 验收标准

- 完整文件大小多重集合必须一致；缺失、额外文件、大小或同尺寸出现次数不同都不得恢复。
- `database_then_site` 在数据库命中时不请求站点，三种模式都有真实 qB 恢复子测试。
- qB 已有同 hash 任务时拒绝将其当作本次恢复。
- 当前切片搜索每个站点最多 5 个名称变体的第一页并合并结果；多页检索和独立搜索记录持久化属于后续完整功能。

## [implemented] 恢复文件管理器与丢失任务扫描

### 前端场景

用户在 WebUI 中浏览 NexusBridge 所在机器的目录和文件，查看哪些条目已属于 qB 任务，并对未关联条目打开恢复弹窗。弹窗可选择站点检索，也可直接粘贴 torrent 下载 URL。高级入口先只读扫描当前目录下可能丢失的任务，再由用户一次确认后自动恢复全部唯一候选；是否允许数据库未命中时继续搜索站点网页由用户显式选择，默认关闭。

### 期望接口

`POST /api/files/browse`

请求 `{ "path": "D:/Downloads" }`；空路径返回可用文件系统根目录。响应包含标准化当前路径、父目录、qB 连接诊断和按目录优先排序的条目。每个条目包含名称、完整路径、类型、大小、修改时间，以及完整路径相等的 qB 任务摘要。目录浏览本身不读取文件内容。

`POST /api/qb/recovery/preview`

在已有字段外增加可选 `torrent_url` 和 `category`。提供 URL 时不再查询数据库或站点搜索页：后端先按指定站点或 URL 主机匹配已配置站点；命中站点或其 `request_rules` 后通过统一站点请求入口携带允许的 Cookie 下载，否则使用无 Cookie 的普通 HTTP 请求。下载结果必须通过 torrent bencode 校验；目标与 torrent 的完整文件大小多重集合必须相等，相对路径和文件名用于优先映射同尺寸文件，无线索时使用稳定映射并交由 qB 强制校验。响应返回自动推断或调用方覆盖的 qB 分类。

`POST /api/qb/recovery`

同步支持 `torrent_url` 和 `category`。所有来源都暂停并跳过初始校验添加，以目标父目录作为最终保存目录，再以完整 `oldPath` 调用 `renameFile` 设置全部文件路径；磁盘文件保持不变。分类和重命名完成后显式强制校验，未观察到 checking 状态时自动等待重试；只有暂停且 100% 完成才启动做种。校验失败保留唯一暂停任务，并返回“仍然启动”或“删除任务但保留文件”的可用操作，不再次添加。`category` 为空时，仅当最终 qB `savepath` 与某个分类的保存目录一致才自动设置分类。

`GET /api/qb/recovery/index` 与 `POST /api/qb/recovery/index/rebuild`

文件管理器展示大小索引覆盖状态，并提供一次手动重建入口。没有后台补齐；旧数据完成重建后，torrent BLOB 的保存、覆盖和删除会事务内自动维护索引。

`POST /api/qb/recovery/scan`

请求：

```json
{
  "path": "D:/Downloads",
  "site_ids": ["kamept"],
  "search_mode": "database",
  "max_depth": 1,
  "limit": 200
}
```

递归枚举根目录下的文件和子目录，对不属于现有 qB 任务的条目执行恢复预览。默认只查本地数据库、深度 1，最大深度 5、最多评估 500 个条目。响应区分唯一候选、多候选、未匹配和跳过原因；接口不调用 qB 写操作。

`POST /api/qb/recovery/batch`

请求携带扫描结果中已经唯一确定的候选以及扫描时是否启用了网页搜索：

```json
{
  "web_search": false,
  "items": [
    { "path": "D:/Downloads/example", "site_id": "kamept", "torrent_id": "123", "category": "PT" }
  ]
}
```

后端逐项串行调用单任务恢复流程，不并发冲击 qB、SQLite 或站点；`web_search=false` 时重新验证只查询数据库，启用后使用数据库优先、站点网页兜底。请求必须提供扫描阶段选定的 `site_id + torrent_id`，相同路径或相同 torrent 在一个批次中只执行一次。单项错误不终止后续项，响应分别统计 `recovered`、`needs_attention`、`failed` 和 `skipped`。校验失败或启动失败的任务保持暂停并返回原有显式启动/删除操作，不会作为额外下载任务自动启动。

### 验收标准

- 文件与目录均可作为恢复目标；文件目标只比较该文件，不受同目录其他文件影响。
- qB 归属只使用规范化后的完整 `content_path` 精确判断，不用名称猜测。
- URL 私站识别不得把 Cookie 发送到未匹配站点或未列入 `request_rules` 的域名。
- 扫描接口有深度、条目数限制且只读；前端默认仅查本地数据库，用户可显式启用网页搜索。
- WebUI 提供目录导航、路径输入、qB 状态标记、右键菜单、恢复预览弹窗、扫描候选列表和一次确认的自动批量恢复。

## 模板

````md
## [proposed] 下载任务批量重试

### 前端场景

下载任务列表需要支持批量重试失败任务。

### 期望接口

`POST /api/download-tasks/retry`

### 请求

```json
{
  "ids": ["task-1", "task-2"]
}
```

### 响应

```json
{
  "retried": 2,
  "failed": 0
}
```

### 验收标准

- 不存在的 ID 返回明确错误。
- 只有 `failed` 状态任务允许 retry。
- 前端能根据响应刷新任务列表。
````

## 待办需求

暂无。

## [implemented] 可复用筛选规则与草稿预览

### 前端场景

订阅页需要独立编辑可复用筛选条件，在保存前用 SQLite 中的既有种子验证全部条件和拒绝原因。

### 期望接口

- `GET/POST /api/rules`
- `PUT/DELETE /api/rules/{rule_name}`
- `POST /api/rules/preview`
- `GET /api/sites/{site_id}/filter-options`

### 请求与响应

草稿预览提交 `rule`、可选 `site_id`、`limit` 和 `offset`，返回每个种子的 `matched`、稳定 `reasons[]` 和本地 `qb_state`；CRUD 返回完整 `Rule` 或删除结果，PUT 支持事务级联改名。

### 验收标准

- 草稿可以未保存，所有已保存规则均可被订阅引用。
- 预览不抓站、不访问 qB、不写候选或下载任务。
- 被订阅引用的规则不能删除。
- 筛选和排序字段与 OpenAPI 的 `Rule` 一致。

## [implemented] 自动订阅、站点计划与运行审计

### 前端场景

订阅页需要把筛选规则与站点、优先级、qB 下载配置和配额组合，并查看未读候选与最近执行结果；站点设置需要独立开启周期抓取。

### 期望接口

- `GET/POST /api/subscriptions`
- `DELETE /api/subscriptions/{subscription_id}`
- `POST /api/subscriptions/{subscription_id}/preview`
- `GET /api/subscriptions/{subscription_id}/candidates`
- `GET /api/subscription-runs`
- `GET/POST /api/sites/{site_id}/schedule`
- `POST /api/sites/{site_id}/fetch`
- `GET /api/site-fetch-jobs`

### 请求与响应

订阅保存完整 `Subscription`；预览返回命中、可执行性、全部原因和最终 `DownloadPlan`。站点抓取返回持久化 `SiteFetchJob`，运行记录按页汇总订阅结果；站点计划使用 `enabled` 和 `interval_seconds`，响应包含最近/下次时间与错误。

### 验收标准

- 订阅和站点计划默认关闭，周期允许 60 至 86400 秒。
- 同站点订阅按 `priority DESC, id ASC`，新种子由首个命中者独占。
- 配额或 qB 前置条件不满足时，候选保持 `unread` 且不回退到低优先级订阅。
- 所有站点列表抓取逐页入库并执行该站点已启用订阅；站点和订阅不再提供重复的 run-once 入口。

## [implemented] qB 分类与标签快照

### 前端场景

订阅编辑器需要在 qB 离线时仍展示最近分类/标签，并把 `PT/ASMR` 这类完整分类字符串渲染为选择树；用户还需要创建完整分类和标签。

### 期望接口

- `GET/POST /api/qb/categories`
- `GET/POST /api/qb/tags`

### 请求与响应

GET 接受可选 `refresh=true` 并返回 `items/stale/connected/error/synced_at`。创建分类提交 `name/save_path`；创建标签提交 `tags[]`。

### 验收标准

- `path_segments` 只用于展示，不改变发送给 qB 的完整分类原值。
- 创建分类时不隐式创建父节点。
- qB 离线且有缓存时返回带 `stale/error` 的快照；无缓存时返回明确错误。

## [implemented] 手动批量下载计划与任务重试

### 前端场景

规则预览结果需要多选种子，快速套用一个订阅或使用临时 qB 参数；确认前必须看到最终路径、分类、标签、名称和阻塞原因，失败任务需要按历史快照重试。

### 期望接口

- `POST /api/downloads/batch/preview`
- `POST /api/downloads/batch`
- `POST /api/download-tasks/{task_id}/retry`

### 请求与响应

批量请求提交 `torrents[]` 联合键，并在 `subscription_id` 与 `options` 中严格二选一。preview 返回逐项计划；execute 返回计数和下载任务；retry 返回更新后的单个任务。

### 验收标准

- preview 零写入，execute 重新校验并持久化最终计划快照。
- 快速套用订阅时忽略配额、周期和订阅优先级。
- retry 复用保存的计划快照，不受订阅后续修改影响。
- 纯 v2、分类缺失、qB 离线和真实新增失败均返回可解释状态，不伪装为成功。

## [implemented] 鉴权站点封面代理

### 前端场景

媒体页需要显示 WebP 及需要站点 cookie、Referer 的封面，浏览器直接访问可能受到鉴权、防盗链或混合内容限制。

### 期望接口

`GET /api/torrents/{site_id}/{torrent_id}/cover`

### 请求

路径参数使用媒体卡片已有的站点 ID 和种子 ID，无请求体。

### 响应

成功时返回可缓存一小时的同源图片二进制；不存在或远端响应不是图片时返回错误。

### 验收标准

- 使用对应站点已有 User-Agent、Referer 和 `request_rules` 白名单 Cookie 请求原始封面。
- WebP 能被浏览器实际解码显示。
- 不向前端或日志暴露 Cookie 值；跨主机重定向重新匹配规则，非图片响应和超过 10 MiB 的响应被拒绝。

## [implemented] 媒体卡片 qB 状态控制

### 前端场景

媒体卡片把下载入口、下载进度、做种和暂停状态合并为圆形控件，需要在不打开 qB WebUI 的情况下暂停或恢复单个任务。

### 期望接口

`POST /api/torrents/{site_id}/{torrent_id}/qb-control`

### 请求

```json
{ "action": "stop" }
```

`action` 仅允许 `start` 或 `stop`。

### 响应

返回操作并实时刷新后的 `QBTorrentStatus`。

### 验收标准

- 只按数据库 v1 hash 或已有下载任务 hash 控制精确匹配的 qB 任务。
- 操作成功后立即回写快照，卡片无需等待全量同步。
- 未找到 hash、qB 请求失败或 action 非法时返回明确错误。

## [implemented] qB 媒体状态增量刷新

### 前端场景

媒体状态条需要低成本更新下载字节、总量、进度和做种状态，并在页面隐藏或 qB 断连时降低请求频率。

### 期望接口

`GET /api/qb/poll?rid={rid}`

### 请求

首次请求使用 `rid=0`，后续请求携带上次响应的 `rid`。

### 响应

返回 qB 连接状态、下一 `rid`、是否完整更新，以及按本地 `site_id + torrent_id` 映射后的 `qb_status` 更新列表。

### 验收标准

- 后端使用 qB `sync/maindata`，正确合并部分字段并持久化快照。
- 页面前台且连接正常时按高频间隔轮询，后台和断连状态使用独立可配置间隔。
- qB URL 为空或用户关闭自动同步时停止轮询。
- 暂停、恢复和新增下载后立即触发一次增量同步。
