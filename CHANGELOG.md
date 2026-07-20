# Changelog

本文记录 NexusBridge 面向用户的功能变化和重要修复。

## [Unreleased]

### Added

- 新增站点增量分页扫描和持久化任务状态，支持首页、手动及周期入口共享同一站点任务，默认最多扫描 3 页，并在连续遇到既有普通种子时提前停止。
- 媒体列表新增服务端分页、站点及关键词全库筛选、置顶显示开关和稳定排序，支持 20、50、100 条页大小。
- 新增种子封面持久缓存，首次下载后保存到数据目录，并通过内容摘要 ETag 和浏览器缓存复用本地文件。
- 新增网络代理设置，支持系统代理、手动代理和直连模式。
  - 通过标准 `HTTP_PROXY`、`HTTPS_PROXY` 与 `NO_PROXY` 环境变量统一配置，不改动现有 HTTP 客户端。
  - 可逐行维护 NO_PROXY 规则、恢复内置局域网与常用域名默认项，并自动将 qBittorrent WebUI 地址加入直连列表。
- 新增同时运行 qBittorrent-nox、NexusBridge 和 Mihomo 的一体化 Docker 镜像，支持 `linux/amd64` 与 `linux/arm64`。
  - 三个应用的配置统一持久化到 `/config`，并保留 qBittorrent 首次启动临时密码日志。
  - 镜像预置 MetaCubeXD、GeoIP、GeoSite、Country MMDB 和 ASN MMDB，并提供 `nano` 修改容器内配置。
  - 新增 GitHub Actions 多架构镜像构建与 GHCR 发布流程。
- 设置页新增 Mihomo Proxy Provider 快捷管理，可选择并持久化配置目录，使用名称和 URL 追加 HTTP Provider。

### Changed

- 所有站点列表抓取统一为逐页入库、逐页消费订阅队列并执行已启用订阅；移除重复的 WebUI 单次运行入口，无启用订阅的站点计划不再访问站点。
- 站点数字分页统一由 JSON `html.search.fields.page` 和 `html.browse.start` 声明；KamePT 列表按 `torrents.php?page=0/1/2` 形式请求。
- NexusBridge 默认监听地址改为 `0.0.0.0:8090`。
- Mihomo 默认配置改为从 `proxy-providers` 动态筛选地区、AI、自动选择和负载均衡组，并集成自定义分流规则。
- Mihomo External Controller 默认发布 `9090` 端口，可通过内置 WebUI 管理。

### Fixed

- 修复分页链接基于域名根路径解析、旧置顶状态与内存缓存不同步、重复抓取入口可能创建多个活动任务，以及媒体分页计数与列表筛选规则重复维护的问题。
- 修复第三方图床把封面请求误判为网页导航而返回 HTML 的问题；图片请求会按同源、同站或跨站关系生成浏览器请求头，并仅按站点域名和 `request_rules` 白名单携带 Cookie。
- 运行时仅在主配置、内置站点和同名站点索引规则不存在时排他创建，避免启动或升级覆盖用户配置。
- Docker 默认模板改为在 `/config` 卷挂载后由 s6 安装，避免被 bind mount 或 volume 遮蔽。

## [0.0.0-beta.1] - 2026-07-19

### Fixed

- 修复从保留文件恢复 qB 任务时，无法为 `savePath` 为空的分类自动分配分类的问题。
  - 分类自身配置了保存路径时，继续优先使用该路径。
  - 分类路径为空时，从最近的、保存路径非空的父分类继承，并追加剩余分类层级。
  - 整条父分类链都没有保存路径时，读取 qBittorrent `app/defaultSavePath`，再追加完整分类层级。
  - 多个分类的有效路径同时匹配恢复目录时，选择层级最深、最具体的分类。
