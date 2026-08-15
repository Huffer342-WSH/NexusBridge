# 视频字幕

本文统一记录 NexusBridge 的 MKV 内嵌字幕与普通视频外挂字幕方案。字幕只做解析、格式转换、缓存和浏览器渲染，不转码音视频，不修改源媒体或字幕文件。

## 支持范围

| 来源 | 支持格式 | 发现方式 | 持久内容 |
| --- | --- | --- | --- |
| MKV 内嵌字幕 | SRT/UTF-8、WebVTT、ASS、SSA | 读取当前 MKV 容器头部 | 按轨道生成的 VTT，以及 ASS/SSA 富样式产物 |
| 普通视频外挂字幕 | `.ass`、`.ssa`、`.srt`、`.vtt` | 用户在播放页手动选择 | 视频绝对路径与字幕绝对路径的 SQLite 关联，以及转换后的字幕产物 |

PGS、VobSub 等图片字幕当前不做 OCR 或渲染。一个视频目前只保存一个手动外挂字幕关联；重新选择会替换旧关联。

## 统一播放模型

当前媒体的全部可用字幕都写入 `PlaybackMedia.subtitles`：

- `stream_url` 始终提供浏览器兼容的 WebVTT。
- ASS/SSA 额外通过 `rich_url` 提供原样式字幕。
- 手动外挂字幕使用 `external: true` 和保留的轨道编号 `0`；MKV 内嵌轨道使用容器轨道 ID。
- `PlaybackContext.external_subtitle` 返回已保存的外挂字幕路径及当前可用状态。字幕文件暂时离线时保留关联，但不把不可读取的轨道交给播放器。

Artplayer 默认选择 forced 轨、default 轨或第一条字幕轨；设置面板允许切换轨道、关闭字幕和调整时间偏移。原生视频播放器使用标准 `<track>`。

## MKV 内嵌字幕流程

1. 播放清单只读取当前 MKV 的容器元数据，列出支持的文本轨。
2. 浏览器请求字幕时，后端按播放来源重新解析受控源文件和轨道 ID。
3. SRT/WebVTT 生成标准 VTT；ASS/SSA 同时生成 ASS 富样式产物和扁平化 VTT。
4. 服务启动约 3 秒后扫描全部已配置媒体库并检查所有 MKV，之后按“设置 → 媒体库”的全局周期检查；默认 360 分钟（6 小时），设置为 0 时关闭周期扫描。周期检查只把新增、大小变化或修改时间变化的 MKV 放入预热队列。人工重扫会重新检查该媒体库的全部 MKV，以修复缺失缓存。全局队列按规范视频路径防重，最多并行预热两个文件。缓存缺失时播放请求仍即时提取，并等待相同的在途任务。

数据库种子、纯 qB 任务和本机文件分别通过对应的受控媒体路径重新定位同一个 MKV，客户端不能直接提交任意容器内部轨道内容。

## 手动外挂字幕流程

1. 播放页使用文件选择器选择 ASS、SSA、SRT 或 VTT 文件。
2. 后端确认视频和字幕都是当前可读的本机普通文件，并规范化为绝对路径。
3. 保存前立即生成 WebVTT 产物；ASS/SSA 同时生成富样式产物。生成失败时不保存关联。
4. SQLite 表 `playback_external_subtitles` 保存一对一的路径关联。
5. 之后从本机文件、剧集或同机 qB 入口打开相同视频路径，都会恢复该关联。
6. 更换字幕覆盖路径关联；移除只删除数据库关联，不删除字幕源文件和视频。

外挂 ASS/SSA 原样提供给 JASSUB，并生成 VTT 回退；SRT 转换为 VTT，VTT 直接进入统一缓存。

## 缓存与失效

字幕产物保存在元数据库同级 `subtitles/`。每个产物最大 32 MiB，并采用原子写入：

- MKV 内嵌字幕的指纹包含规范源路径、文件大小、修改时间和轨道 ID。
- 外挂字幕的指纹包含规范视频路径、规范字幕路径、字幕大小和字幕修改时间。
- 同一进程内相同产物只生成一次；其他请求等待首次生成完成。
- 源文件或字幕修改后指纹改变，新请求自然生成新版本。
- 缓存写入失败不阻断当前播放，仍可返回本次内存产物。
- 缓存目录记录对应源视频路径；只有媒体库成功扫描确认视频消失，并再次确认文件不存在时，才删除该视频的缓存。

字幕响应包含 ETag 与 `X-NexusBridge-Subtitle-Cache: persistent|memory`。缓存目录可以整体删除并按需重建；SQLite 中的外挂字幕路径关联不受影响。正常的字幕更新或重新生成不会主动清理旧指纹缓存，直到对应视频丢失。

## ASS/SSA 浏览器渲染与回退

WebUI 使用 `artplayer-plugin-jassub` 和 JASSUB/libass 渲染 ASS/SSA。Worker、标准 WASM、SIMD WASM 和默认字体随 WebUI 同源发布。

浏览器具备 WASM、Worker 和 OffscreenCanvas 时优先加载 `rich_url`；JASSUB 初始化或运行失败时，播放器自动切回同轨 `stream_url` 的 WebVTT。当前不申请本机字体权限，也尚未提取 MKV 字体附件，因此依赖特殊内嵌字体的 ASS 可能使用默认字体替代。

## HTTP API

| API | 用途 |
| --- | --- |
| `GET /api/torrents/{site_id}/{torrent_id}/media/{file_index}/subtitles/{track_id}` | 数据库种子 MKV 内嵌字幕 |
| `GET /api/playback/qb/{hash}/media/{file_index}/subtitles/{track_id}` | qB 任务 MKV 内嵌字幕 |
| `GET /api/playback/file/subtitles/{track_id}?path=...` | 本机 MKV 内嵌字幕 |
| `PUT /api/playback/external-subtitle` | 保存或替换视频与外挂字幕路径关联 |
| `DELETE /api/playback/external-subtitle?path=...` | 删除关联但保留源文件 |
| `GET /api/playback/external-subtitle?path=...` | 获取外挂字幕的 VTT 产物 |
| `GET /api/settings/media-libraries` | 获取所有媒体库共用的字幕扫描周期 |
| `PUT /api/settings/media-libraries` | 保存扫描周期并立即重排后台计时器 |

内嵌或外挂 ASS/SSA 的读取 API 都可追加 `format=ass` 获取富样式产物。完整请求和响应结构见 [HTTP API](../api.md) 与 [OpenAPI](../api/openapi.yaml)。

## 后续范围

- 自动发现目录中的子媒体库或剧集后续实现；当前只处理已经配置的媒体库。
- 一个视频保存多个外挂字幕文件。
- 自动匹配同目录、同名字幕。
- MKV 字体附件提取与加载。
- PGS、VobSub 等图片字幕明确不开发。

## 代码导航

| 职责 | 代码 |
| --- | --- |
| 播放字幕模型 | `internal/core/models_playback.go` |
| MKV 轨道发现和字幕接口 | `internal/core/playback_subtitles.go` |
| 外挂字幕关联 | `internal/core/playback_external_subtitles.go`、`internal/storage/playback_subtitles.go` |
| 字幕产物、缓存与格式转换 | `internal/core/subtitleartifact/` |
| HTTP 路由与响应 | `internal/server/server.go`、`internal/server/handlers_torrents.go` |
| 播放页字幕管理 | `webui/src/components/PlaybackView.vue` |
| Artplayer/JASSUB 渲染 | `webui/src/components/player/ArtplayerVideo.vue` |
