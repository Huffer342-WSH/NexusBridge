# 架构说明

NexusBridge 以共享 Go 服务为核心，CLI、HTTP、WebUI 和桌面端都是薄适配层。

## 后端

- `cmd/nexusbridge` 是 CLI 入口。
- `desktop` 是 Wails v3 桌面入口，只组装窗口、托盘与嵌入资源。
- `internal/cli` 负责 Cobra 命令和进程级行为。
- `internal/config` 负责加载和校验 JSON 配置。
- `internal/runtimeconfig` 统一解析开发/测试数据目录、正式版用户目录、可执行文件旁引导配置和配置内相对路径。
- `internal/builtin` 嵌入首装站点定义；`webui/assets_production.go` 仅在正式构建中嵌入 WebUI，`assets_development.go` 让开发和测试不依赖 `dist`；`desktop/assets.go` 嵌入桌面图标。
- `internal/core` 放领域模型、服务接口和 MVP 应用服务。`FilterRule` 只负责数据库种子筛选与排序，`Subscription` 引用规则并保存站点、优先级、qB 下载计划和配额；torrent 文件补齐、下载计划、订阅调度、qB 分类标签、hash 同步、文件浏览和保留目录恢复分别放在独立文件中，避免把下载器编排继续堆叠在 `app.go`。
- `internal/requestpolicy` 负责严格域名后缀匹配、Cookie 白名单选择和缺失项描述；`internal/core/site_requests.go` 统一加载站点凭据并编排搜索页、详情、torrent 文件和封面请求。
- `internal/fetcher` 只负责构造和执行 HTTP 请求；动态 Cookie 策略会在初始请求及每次重定向前重新解析，且日志不输出 Cookie 值。
- `internal/parser` 负责加载新格式站点定义、按 NexusPHP 通用搜索框规则补完 `html.search.fields`，并按 `html.torrents.fields` 字段规则解析 NexusPHP `torrents.php` 和详情页 HTML；该包不依赖 HTTP、SQLite、CLI 或 WebUI。
- 站点定义正式来源是 `sites_dir` 目录，格式统一为 `id/name/domain/html` 顶层结构，不再支持旧 `site_config/selectors/search_config` 站点 JSON。
- `internal/qbittorrent` 分层封装 qBittorrent：`client.go` 负责连接、认证和通用 HTTP 请求，`torrents.go` 对应原生 torrent/category/tag WebAPI，`sync.go` 封装 `sync/maindata` 增量协议，`transfer.go` 负责 metainfo 原始名称、文件结构、v1/v2 hash 与添加协调。core 的增量同步只合并列表变化，全量同步继续负责 properties、完成检测和整理任务。
- `internal/llm` 封装 OpenAI-compatible Chat Completions 调用，用于下载前标题整理、cosplay 视频信息提取和下载完成后的媒体库路径建议。
- `internal/organizer` 校验 LLM 输出的相对路径，并执行 dry-run 或硬链接。
- `internal/server` 暴露本地 HTTP API、RSS 端点，并支持嵌入 WebUI 或开发期静态目录。
- `internal/desktop` 负责共享 core/server 生命周期与 Wails HTTP 适配；数据目录策略由 `internal/runtimeconfig` 提供，不重复实现业务接口。
- `internal/storage` 负责 SQLite 连接、cookie、种子列表、torrent BLOB/hash/原始名称、qB 分类标签和任务状态快照、筛选规则、订阅、站点计划、独占候选、运行记录、下载任务、整理任务和应用设置。SQLite 固定使用 WAL，并通过 modernc DSN 为连接池中的每条连接设置锁等待超时；连接池有界，正常短写事务只会让并发写入等待，调度读取不会被阻塞。规则以名称作为大小写不敏感的自然键；本次模型调整采用全库重建，不承载旧规则表兼容。`torrent_files` 与各类 qB 快照独立于列表元数据，普通列表查询不会读取 BLOB。

核心服务不能依赖 CLI、WebUI 或桌面端代码。

## 订阅闭环

常驻 WebUI/CLI 服务版和桌面版显式启动最小站点调度器；`fetch`、`run-once` 等 CLI 单次命令不会启动常驻后台任务。站点计划相互独立且默认关闭，同站点使用防重入控制。每次到期执行按以下边界推进：

1. 抓取一次站点，按页面顺序 upsert；`inserted` 记录与 `subscription_ingest_queue` 在同一事务写入，只有完成首次订阅匹配后才确认删除队列记录，进程中断不会丢失新种子。
2. 下载并解析缺失的 torrent BLOB、hash 和原始 `info.name`。
3. 按 `priority DESC, id ASC` 评估该站点已启用订阅；首个命中的订阅写入唯一候选，后续订阅不再评估所有权。
4. 合并该订阅此前因配额或 qB 前置条件不足而保留的 `unread` 候选，按规则排序处理。
5. 原子领取下载任务，保存最终计划和运行统计；context 取消时停止后续处理。

候选所有权、下载任务幂等键和进程内 v1 hash 锁分别阻止低优先级回退、同规则重放及不同入口同时添加相同 hash。订阅改换规则/站点或被删除时，未完成候选会在 SQLite 事务内重新写入 ingest 队列，再按当前优先级重新匹配；启动时会恢复中断的候选与任务领取。所有已保存规则都可被订阅引用；规则改名在事务内级联订阅、候选和下载任务引用。草稿规则预览完全在 SQLite 上计算，不进入闭环。站点 `/fetch` 始终只抓取，站点和订阅 `/run-once` 才执行订阅。保留目录恢复同样由 core 编排，支持数据库、站点网页、数据库优先和直接 URL 四种候选来源；URL 私站识别复用 `request_rules` 与统一 Cookie 请求入口。四种来源最终都比较完整文件大小多重集合；SQLite `torrent_file_size_index` 以 `(file_size, site_id, torrent_id)` 倒排到候选并用出现次数、文件总数和总大小确认集合相等，避免逐次解析全部 BLOB。站点搜索先用展示总大小的容差减少 torrent 文件下载，最终判断不使用展示值。旧 BLOB 只在用户手动重建时补齐，之后 torrent BLOB 的保存、覆盖和删除在同一事务中维护索引；手动重建基于原 BLOB 乐观更新，避免覆盖并发保存的新索引。恢复状态机按“最终目录暂停且跳过初始校验添加、只在 qB 中用 `renameFile` 映射现有磁盘结构、设置分类、重试触发强制校验、确认暂停且完整、开始做种”执行；校验失败保留唯一暂停任务，后续操作只控制或删除该 hash，不重新添加。当前不做 piece SHA1 预校验。文件管理器以 `save_path` 到 `content_path` 的第一段作为唯一内容根，并标记其全部后代；递归扫描复用同一边界且保持只读。

下载计划是纯确定性渲染：只使用缓存的站点/种子/规则/订阅字段，不调用 LLM。显式 save path 会让 qB add 使用 `autoTMM=false`；未配置路径时只提交 category。qB 分类以 `PT/ASMR` 这类完整字符串保存与发送，`/` 拆分仅用于 WebUI 展示。分类必须事先存在，计划标签可在发送前创建。纯 v2 torrent 仍因缺少当前精确协调路径而明确失败。

## 前端

`webui/` 是 Vue/Vite/TypeScript 前端，使用 Naive UI 作为组件库。浏览器 WebUI 和 Wails 桌面端共用这一套前端代码。

组件通过 `src/api.ts` 和主服务通信，避免在多个组件中散落 HTTP 调用。浏览器模式由 Go HTTP Server 提供这些路由，桌面模式由 Wails AssetServer Middleware 将相同路径直接交给同一个 Handler，因此组件不需要桌面专用传输。`src/utils/runtime.ts` 只隔离打开外部链接等桌面能力，并在 Wails 环境中按需加载 runtime，避免影响普通浏览器。WebUI 可以根据 `/api/session` 返回值展示登录界面；默认本地模式不要求登录。

当前 WebUI 是应用壳结构：桌面端使用左侧导航，移动端使用五项底部导航。顶层页面分为媒体、文件、订阅、任务、设置；设置下再分站点、LLM、qBittorrent 三个子页面。文件页负责本机路径导航、qB 归属标记、右键恢复弹窗和只读丢失任务扫描；实际恢复仍必须逐项确认。订阅页集中管理筛选规则、数据库预览、订阅、站点周期、qB 分类/标签快照、未读候选和最近运行。规则区在宽屏使用规则列表、编辑器、自动草稿预览三栏布局；站点分类和两类标签来自本地种子集合，促销来自站点检索定义，预览仅使用本地 qB 快照/任务。qB 分类值按 `/` 生成展示树但提交完整原值。媒体页默认汇总所有站点缓存的种子，也可以在前端按 `site_id` 切换到单站点视图。展示偏好拆分到独立 composable 与浮动快捷设置组件；设置默认值、范围和步长由 composable 或 `src/config` 的只读元数据统一提供，组件不重复硬编码。通用显示格式放在 `src/utils`。qB 轮询 composable 根据连接、页面和可见性选择频率。卡片状态控件挂载在封面容器内部，以图片作为定位和裁剪边界，默认折叠为图标并在 hover/focus 时展开；列表模式使用右侧的独立控件，避免复用绝对定位。触屏设备始终展开。全局操作结果由悬浮消息展示。

站点设置页提供每站点 cookie/user-agent 保存、抓取和 `run-once` 操作；检索完成后自动补齐缺失的 torrent BLOB/hash。站点定义的 `request_rules` 可按目标域后缀从当前站点 scope 中选择跨主机 Cookie，未命中规则时禁止跨域转发。媒体页通过显式同步按 hash 更新 qB 快照，展示状态、进度、分类、标签和路径，并提供快捷展示设置与 WebUI 入口。远程封面经统一请求入口获取，由 server 同源返回并限制为图片响应。手动下载仍先生成标题预览，再复用数据库 torrent 文件发送 qB。

## 桌面端

桌面端使用固定版本 Wails v3 `v3.0.0-alpha2.117`。入口位于 `desktop/main.go`，只负责创建窗口、托盘和嵌入 WebUI；`internal/desktop` 创建唯一的 `core.App`。Wails Middleware 仅拦截 `/api`、`/rss`，其余请求继续交给生产静态资源或开发期 Vite Handler，不开放本地端口。

桌面应用使用 Wails 单实例锁。启动后由共享 core 使用桌面生命周期 context 运行站点调度器；关闭窗口时取消关闭事件并隐藏到托盘，后台计划继续运行。托盘“显示主窗口”负责恢复和聚焦，“完全退出”触发应用关闭并依次取消后台 context、停止调度、关闭 SQLite 和日志。桌面二进制固定命名为 `nexusbridge-desktop`，WebUI/CLI 服务二进制命名为 `nexusbridge-webui`。

## 发布结构

开发构建通过缺省 build tag 使用仓库 `data/`。发布构建使用 `production` tag，并优先读取可执行文件旁的 `nexusbridge.bootstrap.json`，否则使用系统用户配置目录。`.github/workflows/build-release.yml` 只负责编排构建，并调用 `scripts/ci/` 下的 Bash/PowerShell 脚本，为桌面版和 WebUI/CLI 服务版分别构建 Windows amd64、Linux amd64、Linux aarch64，共六个发布包；所有版本都嵌入 WebUI 和内置站点定义。调试时可通过 workflow 的 `target_job` 只运行单个平台构建。

发布编排位于独立的可复用 workflow `.github/workflows/publish-release.yml`。推送 `v*` tag 会在全平台构建成功后发布正式版本；手动全量构建显式启用发布时，空 `release_tag` 会按 `Asia/Shanghai` 日期生成 `manual-YYYY.MM.DD.<run_number>` tag 并标记为 prerelease。发布 workflow 校验 tag 与构建 commit 一致、下载同一 workflow run 的 artifacts、通过 `changelogithub` 生成 `RELEASE_NOTES.md`，最后调用 GitHub CLI 创建 GitHub Release。单平台调试构建不进入发布流程。

桌面入口、Wails Taskfile、平台资源和打包配置都位于 `desktop/`。仓库不使用根 `build/` 保存源码或配置，避免和临时构建产物混淆；实际产物统一写入被忽略的 `bin/` 和工作流临时 `dist/`。

WebUI/CLI 服务版通过根 Taskfile 的 `build:webui` 任务统一构建。该任务先调用共享前端任务，再按 `GOOS`、`ARCH` 生成 `nexusbridge-webui`，GitHub Release 与本地构建使用同一入口。
