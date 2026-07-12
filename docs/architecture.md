# 架构说明

NexusBridge 以共享 Go 服务为核心，CLI、HTTP、WebUI 和桌面端都是薄适配层。

## 后端

- `cmd/nexusbridge` 是 CLI 入口。
- `desktop` 是 Wails v3 桌面入口，只组装窗口、托盘与嵌入资源。
- `internal/cli` 负责 Cobra 命令和进程级行为。
- `internal/config` 负责加载和校验 JSON 配置。
- `internal/runtimeconfig` 统一解析开发/测试数据目录、正式版用户目录、可执行文件旁引导配置和配置内相对路径。
- `internal/builtin` 嵌入首装站点定义；`webui/assets_production.go` 仅在正式构建中嵌入 WebUI，`assets_development.go` 让开发和测试不依赖 `dist`；`desktop/assets.go` 嵌入桌面图标。
- `internal/core` 放领域模型、服务接口和 MVP 应用服务；torrent 文件补齐、qB 配置与发送、qB hash 同步分别放在独立文件中，避免把下载器编排继续堆叠在 `app.go`。
- `internal/requestpolicy` 负责严格域名后缀匹配、Cookie 白名单选择和缺失项描述；`internal/core/site_requests.go` 统一加载站点凭据并编排搜索页、详情、torrent 文件和封面请求。
- `internal/fetcher` 只负责构造和执行 HTTP 请求；动态 Cookie 策略会在初始请求及每次重定向前重新解析，且日志不输出 Cookie 值。
- `internal/parser` 负责加载新格式站点定义、按 NexusPHP 通用搜索框规则补完 `html.search.fields`，并按 `html.torrents.fields` 字段规则解析 NexusPHP `torrents.php` 和详情页 HTML；该包不依赖 HTTP、SQLite、CLI 或 WebUI。
- 站点定义正式来源是 `sites_dir` 目录，格式统一为 `id/name/domain/html` 顶层结构，不再支持旧 `site_config/selectors/search_config` 站点 JSON。
- `internal/qbittorrent` 分层封装 qBittorrent：`client.go` 负责连接、认证和通用 HTTP 请求，`torrents.go` 对应原生 torrent WebAPI，`sync.go` 封装 `sync/maindata` 增量协议，`transfer.go` 负责 info hash 与添加协调。core 的增量同步只合并列表变化，全量同步继续负责 properties、完成检测和整理任务。
- `internal/llm` 封装 OpenAI-compatible Chat Completions 调用，用于下载前标题整理、cosplay 视频信息提取和下载完成后的媒体库路径建议。
- `internal/organizer` 校验 LLM 输出的相对路径，并执行 dry-run 或硬链接。
- `internal/server` 暴露本地 HTTP API、RSS 端点，并支持嵌入 WebUI 或开发期静态目录。
- `internal/desktop` 负责共享 core/server 生命周期与 Wails HTTP 适配；数据目录策略由 `internal/runtimeconfig` 提供，不重复实现业务接口。
- `internal/storage` 负责 SQLite 连接、迁移、cookie、种子列表、torrent BLOB/hash、qB 状态快照、规则、下载任务、整理任务和应用设置。`torrent_files` 与 `torrent_qb_snapshots` 独立于列表元数据，普通列表查询不会读取 BLOB。

核心服务不能依赖 CLI、WebUI 或桌面端代码。

## 前端

`webui/` 是 Vue/Vite/TypeScript 前端，使用 Naive UI 作为组件库。浏览器 WebUI 和 Wails 桌面端共用这一套前端代码。

组件通过 `src/api.ts` 和主服务通信，避免在多个组件中散落 HTTP 调用。浏览器模式由 Go HTTP Server 提供这些路由，桌面模式由 Wails AssetServer Middleware 将相同路径直接交给同一个 Handler，因此组件不需要桌面专用传输。`src/utils/runtime.ts` 只隔离打开外部链接等桌面能力，并在 Wails 环境中按需加载 runtime，避免影响普通浏览器。WebUI 可以根据 `/api/session` 返回值展示登录界面；默认本地模式不要求登录。

当前 WebUI 是应用壳结构：桌面端使用左侧导航，移动端使用底部导航。主页面分为媒体、任务、设置；设置下再分站点、LLM、qBittorrent 三个子页面。媒体页默认汇总所有站点缓存的种子，也可以在前端按 `site_id` 切换到单站点视图。展示偏好拆分到独立 composable 与浮动快捷设置组件；设置默认值、范围和步长由 composable 或 `src/config` 的只读元数据统一提供，组件不重复硬编码。通用显示格式放在 `src/utils`。qB 轮询 composable 根据连接、页面和可见性选择频率。卡片状态控件挂载在封面容器内部，以图片作为定位和裁剪边界，默认折叠为图标并在 hover/focus 时展开；列表模式使用右侧的独立控件，避免复用绝对定位。触屏设备始终展开。全局操作结果由悬浮消息展示。

站点设置页提供每站点 cookie/user-agent 保存、抓取和 `run-once` 操作；检索完成后自动补齐缺失的 torrent BLOB/hash。站点定义的 `request_rules` 可按目标域后缀从当前站点 scope 中选择跨主机 Cookie，未命中规则时禁止跨域转发。媒体页通过显式同步按 hash 更新 qB 快照，展示状态、进度、分类、标签和路径，并提供快捷展示设置与 WebUI 入口。远程封面经统一请求入口获取，由 server 同源返回并限制为图片响应。手动下载仍先生成标题预览，再复用数据库 torrent 文件发送 qB。

## 桌面端

桌面端使用固定版本 Wails v3 `v3.0.0-alpha2.117`。入口位于 `desktop/main.go`，只负责创建窗口、托盘和嵌入 WebUI；`internal/desktop` 创建唯一的 `core.App`。Wails Middleware 仅拦截 `/api`、`/rss`，其余请求继续交给生产静态资源或开发期 Vite Handler，不开放本地端口。

桌面应用使用 Wails 单实例锁。关闭窗口时取消关闭事件并隐藏到托盘；托盘“显示主窗口”负责恢复和聚焦，“完全退出”触发应用关闭并依次取消后台 context、关闭 SQLite 和日志。桌面二进制固定命名为 `nexusbridge-desktop`，WebUI/CLI 服务二进制命名为 `nexusbridge-webui`。

## 发布结构

开发构建通过缺省 build tag 使用仓库 `data/`。发布构建使用 `production` tag，并优先读取可执行文件旁的 `nexusbridge.bootstrap.json`，否则使用系统用户配置目录。GitHub Actions 手动触发后调用 `scripts/ci/` 下的 Bash/PowerShell 脚本，为桌面版和 WebUI/CLI 服务版分别构建 Windows amd64、Linux amd64、Linux aarch64，共六个发布包；所有版本都嵌入 WebUI 和内置站点定义。调试时可通过 workflow 的 `target_job` 只运行单个平台构建。

桌面入口、Wails Taskfile、平台资源和打包配置都位于 `desktop/`。仓库不使用根 `build/` 保存源码或配置，避免和临时构建产物混淆；实际产物统一写入被忽略的 `bin/` 和工作流临时 `dist/`。

WebUI/CLI 服务版通过根 Taskfile 的 `build:webui` 任务统一构建。该任务先调用共享前端任务，再按 `GOOS`、`ARCH` 生成 `nexusbridge-webui`，GitHub Release 与本地构建使用同一入口。
