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

读取数据库中的站点 cookie，抓取 `torrents.php`，解析后写入本地缓存和 SQLite，并补齐缺失的 torrent BLOB/hash。响应增加 `torrent_files_saved` 和 `torrent_files_failed`。

`POST /api/sites/{site_id}/run-once`

执行一次闭环：抓取、解析、持久化、匹配本地规则，并把命中的新种子发送到 qBittorrent。

## 种子

`GET /api/torrents`

返回 SQLite 中的种子元数据、torrent 文件保存状态、v1/v2 hash 和最近一次持久化的 `qb_status`；不会返回 torrent BLOB，普通请求不会访问 qB。

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

返回本地筛选规则。

`POST /api/rules`

保存筛选规则。首版动作只支持 `download`。

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

`GET /api/download-tasks`

返回自动发送到 qBittorrent 的下载任务。

## 整理

`GET /api/organize-tasks`

返回待整理或已整理的任务。

`POST /api/organize/pending`

调用 OpenAI-compatible LLM API 生成媒体库相对路径，并按配置执行 dry-run 或硬链接。

## RSS

`GET /rss/{feed_id}`

预留 RSS 生成接口。当前骨架返回 `501 Not Implemented`。

