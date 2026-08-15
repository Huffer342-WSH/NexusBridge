# 媒体库目录

媒体库目录用统一节点组织本机媒体路径，并为后续目录监控、自动发现剧集、字幕预处理和其他计划任务提供稳定作用域。节点不依赖站点数据库或 qB 任务归属，也不修改源文件。

## 节点与层级

- `collection` 是可继续包含下级的普通媒体库，提供目录边界、设置继承和媒体扫描。
- `series` 是叶子剧集，不能作为父节点；点击后直接进入旧剧集的最后选集和选集播放页。
- 普通媒体库可以继续包含普通媒体库或剧集，因此剧集之外的树没有固定层级。
- 子节点目录必须严格位于父节点至少一个目录内；父节点修改目录时也不能把已有子节点移出范围。
- 节点类型创建后不能修改；有子节点的媒体库不能删除。
- 当前沿用旧剧集表的全局唯一名称约束；后续迁移为同一父节点内唯一。

旧 `/series` 页面重定向到 `/libraries`，旧 `/api/series` 和 `/play/series/:series_id` 继续兼容。已有剧集自动作为根级 `series` 媒体库出现，不需要重建。

## 设置继承

当前可继承设置包括：

- `episode_number_detection`：剧集文件名的集数识别。
- `auto_detect_series`：媒体库自动发现子剧集的预留开关。

每个设置在当前节点可以选择继承、启用或关闭。有效值从根节点向下合并，离当前节点最近的显式覆盖生效；整条父链都未设置时使用关闭状态。`auto_detect_series` 当前只持久化和展示，不启动后台任务。

## 扫描边界

所有媒体库保存后都递归扫描，手动重扫也走同一流程。扫描当前节点时会跳过直接子媒体库负责的目录树，避免父子节点重复缓存和展示媒体。`/libraries/:library_id` 使用单一内容列表混合展示直接子媒体库与扫描媒体，以文件夹、剧集和媒体缩略图区分；普通媒体通过本机文件播放入口打开，剧集直接进入 `/play/series/:series_id`。目录 watcher、稳定文件判断、自动创建子剧集和字幕提取尚未启用，开发顺序见[媒体库与字幕开发计划](../development/media-library.md)。

## 代码导航

| 职责 | 代码 |
| --- | --- |
| 领域模型、层级和继承 | `internal/core/models_media_library.go`、`internal/core/media_libraries.go` |
| SQLite 兼容存储 | `internal/storage/media_libraries.go`、`internal/storage/series.go` |
| HTTP API | `internal/server/handlers_media_libraries.go` |
| WebUI | `webui/src/components/MediaLibrariesView.vue` |
| 旧剧集兼容 | `internal/core/series.go`、`internal/server/handlers_series.go` |
