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
