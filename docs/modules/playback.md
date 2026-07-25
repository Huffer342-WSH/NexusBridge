# 媒体播放

NexusBridge 提供独立播放页 `/play/:site_id/:torrent_id`，直接读取同机 qBittorrent 下载目录并向浏览器传输源文件。服务端不转码、不解码、不调整文件优先级；容器和编码兼容性由浏览器决定。

首版要求 NexusBridge 能按 qB 返回的 `save_path` 直接访问文件，不处理远程 qB、容器路径映射、字幕匹配、缩略图生成或转码回退。

## 工作流程

```mermaid
flowchart LR
    Media[媒体卡片] -->|新标签页| Route[/play/:site_id/:torrent_id]
    Route --> View[PlaybackView]
    View --> Manifest[播放清单 API]
    Manifest --> Catalog[(本地种子与 hash)]
    Manifest --> QBFiles[qB 实时文件清单]
    QBFiles --> Files[自然排序的媒体选集]
    Files --> Selected[selectedIndex]
    Selected --> Canvas[MediaCanvas]
    Canvas --> Engine[视频 / 音频 / 图片播放器]
    Engine -->|GET / HEAD / Range| Stream[源文件 API]
    Stream --> QBVerify[按 hash 和文件索引重新查询 qB]
    QBVerify --> Local[(同机源文件)]
    Local -->|原始字节| Browser[浏览器解码]
```

媒体卡片只在 `qb_status.added=true` 时显示播放入口。入口由 Vue Router 生成地址，并使用 `window.open(..., '_blank', 'noopener,noreferrer')` 打开，不影响媒体页已有的下载、暂停和恢复操作。

播放路由使用独立页面外壳，只保留品牌、当前页面名称和返回媒体库入口。直接打开播放页时，`App.vue` 不初始化普通 Dashboard 数据，也不会触发媒体首页的自动抓取。

## 播放清单

前端进入页面或切换其他种子后，请求：

```http
GET /api/torrents/{site_id}/{torrent_id}/playback
```

后端按以下顺序生成清单：

1. 从本地种子记录解析 qB hash。
2. 按 hash 查询实时 qB 任务和文件清单。
3. 使用固定扩展名白名单识别视频、音频和图片。
4. 根据 qB `save_path` 检查同机文件是否存在、是否仍位于保存目录内且为普通文件。
5. 按文件名执行不区分大小写的自然排序。
6. 生成默认文件索引和同源 `stream_url`。

每个媒体文件包含：

| 字段 | 含义 |
| --- | --- |
| `index` | qB 文件索引，也是源文件接口使用的稳定选择值 |
| `name` | qB 返回的种子内相对文件名 |
| `media_type` | `video`、`audio` 或 `image` |
| `mime_type` | 按扩展名白名单确定的 MIME |
| `size` | qB 文件大小 |
| `progress` | qB 实时下载进度，范围为 `0` 到 `1` |
| `selected` | qB 是否选择下载该文件，不表示播放页当前选集 |
| `complete` | 文件进度是否达到 `1` |
| `available` | 进度大于零且同机文件通过路径和普通文件检查 |
| `stream_url` | 使用 qB 文件索引的同源源文件地址 |

`stream_url` 的 `filename` 查询参数只向播放器提供扩展名提示，例如帮助 Vidstack 识别无扩展名 API 地址中的 WAV；后端不会使用该参数解析本机路径。

播放清单中的 `qb_status` 沿用现有 qB 状态模型，因此可能包含 `save_path` 和 `content_path` 本机绝对路径。

### 默认文件

后端在自然排序后的文件中依次选择：

1. 第一个完整且可用的视频。
2. 第一个完整且可用的音频。
3. 第一个完整且可用的图片。
4. 没有完整文件时，选择下载进度最高的可用文件；同进度按视频、音频、图片排序。

没有可用文件时不返回 `default_file_index`，播放画布显示空状态。

## 前端选集

`PlaybackView.vue` 使用两个不同状态：

- `file.selected` 表示 qB 是否选择下载该文件。
- `selectedIndex` 表示播放页当前展示的 qB 文件索引。

首次加载时，`selectedIndex` 使用后端的 `default_file_index`。用户点击右侧选集后，前端只在 `file.available=true` 时更新 `selectedIndex`；进度为零或同机不可读取的文件保持禁用。

`currentFile` 根据 `selectedIndex` 从当前清单中查找，并传给 `MediaCanvas`。选集切换不会修改 qB 文件优先级。

只要清单中仍有 qB 已选择但未完成的媒体文件，页面每 5 秒刷新一次清单。刷新时优先保留当前仍可用的文件索引；当前文件失效时回退到新的默认文件。轮询失败不会清空已经加载的清单。

## 播放器选择与生命周期

构建配置位于 `webui/src/config/mediaPlayer.ts`：

```ts
export const mediaPlayerConfig = {
  video: 'artplayer',
  audio: 'vidstack',
  image: 'native',
};
```

支持的配置为：

| 媒体类型 | 默认实现 | 可选实现 |
| --- | --- | --- |
| 视频 | Artplayer | `artplayer`、`native` |
| 音频 | Vidstack | `vidstack`、`native` |
| 图片 | 原生查看器 | `native` |

播放路由和各播放器组件都按需加载。`MediaCanvas` 根据 `currentFile.media_type` 和构建配置选择异步组件，并使用媒体类型、qB 文件索引和流地址组成组件 key。切换文件时 Vue 会卸载旧组件并创建新组件：

- Artplayer 在卸载时显式执行 `destroy()`。
- 原生 `<video>`、`<audio>` 和 Vidstack 随组件卸载释放。
- 图片查看器随组件卸载清理 `ResizeObserver` 和动画帧。

### 视频

Artplayer 使用 HTML5 视频源，启用中文、主题色、快捷键、倍速、画中画、网页全屏和系统全屏，不加载转流插件。构建配置切换为 `native` 后使用原生 `<video>`。

### 音频

Vidstack 使用浏览器原生 audio provider，并接收包含 URL 和 MIME 的 source 对象。流地址同时携带原文件扩展名提示，以支持 WAV 等无法仅从 API 路径判断格式的音频。构建配置切换为 `native` 后使用原生 `<audio>`。

原生音视频只在第一次 `canplay` 时尝试自动播放，避免后续缓冲恢复覆盖用户的暂停操作。浏览器阻止自动播放时，页面显示点击播放提示。

### 图片

图片打开后按原始宽高比例缩小到媒体画布范围，小图不主动放大。查看器支持：

- 滚轮和按钮缩放。
- 放大后的鼠标或触摸拖动。
- 受画布边界约束的平移。
- 重置或双击恢复适应画布。
- 全屏查看。
- 画布尺寸变化后重新计算适应尺寸。

控制栏是独立覆盖层，不参与图片尺寸布局。

## 源文件传输

播放器通过以下接口读取源文件：

```http
GET|HEAD /api/torrents/{site_id}/{torrent_id}/media/{file_index}
```

服务端不接受客户端路径。每次请求都会重新解析本地种子、qB hash 和实时 qB 文件清单，再按 `file_index` 取得文件名。

路径检查分为两层：

1. 清理 qB 相对文件名，拒绝绝对路径、`.`、`..` 和越出 `save_path` 的字面路径。
2. 解析 `save_path` 和候选文件中的符号链接，再次确认最终目标位于实际保存目录内。

符号链接可以直接使用，但最终目标不能越出 `save_path`，并且必须是普通文件。

文件由 `http.ServeContent` 返回，支持 `Range`、seek、`Content-Length`、`Last-Modified` 和 HEAD。响应设置正确 MIME、`inline`、私有缓存策略和 `nosniff`。浏览器请求不支持的容器或编码时，只显示加载或解码错误，不进行格式转换。

主要失败状态：

| 状态 | 场景 |
| --- | --- |
| `404` | 种子、文件索引、文件或符号链接目标不存在 |
| `409` | 未关联 qB，或文件下载进度为零 |
| `503` | qB 不可用、路径越界、权限错误或源文件不可访问 |

## 其他种子

页面另外请求：

```http
GET /api/playback/torrents?exclude_site_id=...&exclude_torrent_id=...&limit=20
```

列表只包含已关联 qB 且至少有一个完整、可读取音频或视频文件的种子。只有完整图片的种子不会进入列表。结果按 `published_at` 倒序，缺失发布时间的记录置后；点击后在当前播放标签页切换播放路由。

## 代码导航

| 职责 | 代码 |
| --- | --- |
| 播放领域模型 | `internal/core/models_playback.go` |
| 清单、默认选集、路径和其他种子 | `internal/core/playback.go` |
| HTTP 路由与源文件响应 | `internal/server/server.go`、`internal/server/handlers_torrents.go` |
| 前端路由与独立外壳 | `webui/src/router.ts`、`webui/src/App.vue` |
| 播放页状态和选集 | `webui/src/components/PlaybackView.vue` |
| 播放器分派 | `webui/src/components/player/MediaCanvas.vue` |
| 各媒体播放器 | `webui/src/components/player/` |
| 构建配置 | `webui/src/config/mediaPlayer.ts` |
| API 定义 | `docs/api.md`、`docs/api/openapi.yaml` |

修改后至少运行：

```powershell
go test ./...
go vet ./...
Set-Location webui
pnpm run format:check
pnpm run build
```
