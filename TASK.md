# TASK.md

## 目标

NexusBridge 的目标是做一个轻量的 NexusPHP 站点资源桥接工具：以站点声明式配置为核心，完成 HTML 抓取、结构化解析、本地缓存、筛选订阅、RSS 输出和 qBittorrent 自动下载。

参考 `reference/nexus-media` 后，本项目不复制其完整媒体库、插件、RBAC、多下载器和外部索引器体系，而吸收以下设计：

- 用声明式站点 JSON 描述 URL、请求参数、HTML selector、字段提取规则和下载链接规则，避免在代码中硬编码站点差异。
- 用 SiteEngine 统一加载站点定义、匹配站点、构造搜索/浏览 URL、解析 HTML、解析下载链接。
- 将规则判断做成纯领域逻辑，输入结构体和规则，输出匹配结果与拒绝原因。
- 下载流程拆为独立阶段：获取种子内容、解析下载设置、提交下载器、记录状态。
- 调度器要能管理任务状态、失败重试、单站点并发保护和执行统计。

## 开发原则

- 核心逻辑写在 `internal/`，测试代码统一放在 `tests/`。
- 业务层按 `domain/entity -> repository -> service/engine -> api/cli` 分层；底层不能依赖 CLI、HTTP、WebUI。
- 每个小功能都要有测试；解析和规则优先用 fixture/fake server，真实访问只做端到端验证。
- `fetch.sh` 只作为测试输入，不是正式应用配置。
- cookie、账号密码、passkey、token 等敏感信息最终必须加密或受保护地存储，不进入示例配置和日志。
- 站点名、域名、站点特例只能出现在测试输入、运行时配置或用户导入的站点定义中，不能写进主程序逻辑。

## 1. 站点定义与 SiteEngine

状态：已完成基础 HTML parser 和测试用站点 JSON，下一步应收敛为正式 SiteEngine。

目标：

- 新增 `internal/site` 或 `internal/engine`，负责加载和管理站点定义。
- 站点定义 JSON 覆盖：
  - `id`、`name`、`domain`、`domain_aliases`、`encoding`。
  - `browse`：浏览页路径、默认参数、分页参数。
  - `search`：搜索路径、method、query template、关键词和筛选参数组合规则。
  - `selectors`：搜索框、种子行、标题区域、分页区域。
  - `fields`：种子字段提取规则，包括 selector、attribute、regex、default、case 映射。
  - `download`：下载链接模板或从详情页解析下载地址。
  - `torrent_attr`：促销、HR、做种数等详情页属性提取规则。
- 支持从 `data/sites/*.json` 加载用户站点定义，测试使用 `tests/fixtures/site_parse_config.json`。
- 提供接口：
  - `LoadDefinitions(dir)`.
  - `GetByID(id)`.
  - `GetByURL(url)`.
  - `BuildBrowseURL(siteID, page, params)`.
  - `BuildSearchURL(siteID, SearchQuery)`.
  - `ParseSearchOptions(siteID, html)`.
  - `ParseTorrents(siteID, html)`.
  - `ResolveDownloadURL(siteID, torrent)`.

测试：

- 站点定义 JSON schema/校验测试。
- URL 构造测试：浏览、关键词搜索、分页、checkbox 参数、select 参数。
- 站点匹配测试：domain 和 alias。
- fixture HTML 解析测试。
- 缺失 selector、字段为空、非法正则时返回可诊断错误，不 panic。

## 2. HTML 字段提取 DSL

目标：

- 将当前固定 parser 逐步改造成配置驱动字段提取器。
- 支持：
  - CSS selector。
  - attribute 提取。
  - text 提取。
  - regex 提取。
  - querystring 提取。
  - replace/trim/number/size/date 等 filters。
  - case 映射，用于促销图标、标签、HR 等状态。
  - default/default_format。
- 保留当前 NexusPHP 默认 parser 作为 fallback，但新增站点优先走配置驱动。

测试：

- 每个 filter 单测。
- 字段提取器单测。
- 真实结构 fixture 验证：ID、标题、详情页、下载页、分类、标签、促销、大小、发布时间、统计字段。
- 多字段组合测试：标题 fallback、促销过期时间、绝对 URL 转换。

## 3. 数据模型与仓库层

目标：

- 扩展 SQLite schema，并把数据库操作收敛到 repository。
- 建议表：
  - `sites`：用户站点、启用状态、刷新间隔、站点定义版本、最近错误。
  - `site_credentials`：cookie、UA、headers、token，敏感字段加密或受保护存储。
  - `site_definitions`：导入的站点定义 JSON 和校验状态。
  - `search_options`：解析出的搜索项快照。
  - `torrents`：结构化种子主表。
  - `torrent_tags`：种子标签。
  - `torrent_promotions`：促销状态和有效时间。
  - `torrent_snapshots`：每次抓取的动态字段快照，可选。
  - `fetch_runs`：抓取任务执行记录、耗时、状态、错误。
- upsert 规则：
  - `site_id + torrent_id` 唯一。
  - 首次发现时间不覆盖。
  - seeders/leechers/snatches/promotion/updated_at 每次更新。
  - 促销未在新页面出现时需要记录为可能过期，避免长期误标。

测试：

- migration 测试。
- repository CRUD 测试。
- torrent upsert 幂等测试。
- search options 保存/读取测试。
- credential 不出现在普通查询响应和日志中的测试。

## 4. 抓取服务整合

目标：

- 新增核心服务，把站点定义、凭据、fetcher、parser、repository 串起来。
- 输入 `site_id` 和可选查询参数，执行：
  - 读取站点定义。
  - 读取站点凭据。
  - 构造 URL 和 headers。
  - 执行请求。
  - 解析搜索项和种子列表。
  - 写入数据库。
  - 记录 `fetch_runs`。
- 当前阶段仍只支持 cookie，不实现账号密码登录。
- 支持手动刷新和按站点刷新。

测试：

- fake HTTP server 验证完整流程。
- 数据库 cookie -> 抓取 -> 解析 -> 入库端到端测试。
- 403、302 登录页、网络超时、cookie 缺失、站点定义缺失时返回明确错误。
- 不打印敏感 header/cookie。

## 5. 查询与筛选引擎

目标：

- 在 `internal/core` 或 `internal/rules` 中实现纯规则引擎，不依赖数据库和 HTTP。
- 支持：
  - include/exclude 关键词。
  - 分类包含/排除。
  - tag/label 包含/排除。
  - 促销状态：FREE、2X、普通、折扣。
  - HR 排除。
  - 大小范围。
  - 发布时间范围。
  - 做种/下载/完成数范围。
  - 已收藏、置顶、是否已下载、是否已发送。
- 每条拒绝要能返回原因，用于 WebUI 调试规则。
- 规则格式先用 JSON，不沿用复杂字符串 DSL；后续如需要再提供导入兼容。

测试：

- 每个规则字段至少一个通过和拒绝用例。
- 多条件 AND 组合。
- 拒绝原因稳定。
- 排序和分页稳定。

## 6. 订阅配置与 RSS 输出

目标：

- 新增用户订阅配置，基于数据库缓存生成 RSS，不在 RSS 请求时实时抓站。
- 订阅配置包含：
  - 名称。
  - 站点范围。
  - 筛选规则。
  - 输出数量。
  - 排序方式。
  - 是否只输出未读/未发送。
- RSS item：
  - GUID 使用 `site_id + torrent_id`。
  - link 指向详情页。
  - enclosure 指向本地下载代理或真实下载 URL。
  - title 可拼接分类/促销/标题，但避免过长。
- 提供：
  - 全站最近种子 feed。
  - 每个订阅规则独立 feed。
  - RSS token，避免公开暴露。

测试：

- RSS XML 可解析。
- GUID 稳定。
- 规则变更后输出结果可预测。
- 过期促销不会继续误标。
- 未授权 token 返回明确错误。

## 7. qBittorrent 客户端与下载流水线

目标：

- 新增 `internal/downloader/qbittorrent`，直接封装 WebUI API。
- 初始能力：
  - login/test connection。
  - add torrent by URL。
  - add torrent by file bytes。
  - category/tags。
  - paused/start 状态。
  - 获取 torrents 列表。
  - 检测重复任务。
- 下载流水线拆分为：
  - resolve：根据站点/订阅配置决定下载器、category、tags、是否暂停。
  - fetch：下载 `.torrent` 或使用 URL/magnet。
  - add：提交到 qBittorrent。
  - record：写入发送历史、失败原因、下载器 hash。
- 暂不实现多下载器抽象；接口设计预留 `DownloaderClient`。

测试：

- httptest 验证 qBittorrent WebUI 请求路径、cookie/session、表单字段。
- 登录失败、添加失败、重复种子、超时。
- 下载流水线阶段单测。
- 自动添加任务幂等：同一 `site_id + torrent_id` 不重复发送。

## 8. 自动下载与刷流规则

目标：

- 在订阅配置基础上增加自动下载规则。
- 保存发送状态：
  - pending、sent、exists、failed、skipped。
  - downloader id/hash。
  - 失败原因、重试次数、最后尝试时间。
- 限制：
  - 默认关闭自动下载。
  - 每个规则可配置最大同时下载数、每日数量、只下载促销、排除 HR。
  - 刷流和普通订阅分开，刷流默认需要明确确认。
- 后续再加删种/停种规则，不在第一版自动下载中实现。

测试：

- 匹配规则后调用下载流水线。
- 重复刷新不重复发送。
- 失败后可重试且状态可查询。
- 自动下载关闭时只生成 RSS，不发送下载器。

## 9. 调度器与任务状态

目标：

- 新增轻量 scheduler，不引入大型外部依赖。
- 支持：
  - interval 刷新站点。
  - 手动触发一次。
  - 单站点 max_instances=1。
  - context cancellation。
  - 失败重试和退避。
  - 最近执行统计。
- 任务类型：
  - 站点刷新。
  - 订阅规则计算。
  - 自动下载发送。
  - qBittorrent 状态同步。

测试：

- 短 interval/fake clock 测试。
- 并发保护。
- 失败不退出调度器。
- 取消后不继续执行。

## 10. Web API 与 WebUI

目标：

- API 优先服务实际工作流：
  - 站点列表、站点定义导入、站点连接测试。
  - 凭据导入/更新。
  - 搜索配置读取。
  - 手动刷新站点。
  - 种子列表查询。
  - 订阅配置 CRUD。
  - RSS feed 列表和 token。
  - 下载器连接测试。
  - 手动发送到 qBittorrent。
  - 任务状态和最近错误。
- WebUI 第一阶段：
  - 登录页。
  - 站点管理。
  - 抓取状态。
  - 种子列表与筛选。
  - 订阅规则编辑。
  - qBittorrent 设置。
- GUI 和 WebUI 继续共用 `webui/`。

测试：

- HTTP handler 使用 httptest。
- 前端保持 `pnpm run build` 通过。
- API 响应不泄露敏感字段。

## 11. 安全与配置收敛

目标：

- 从测试输入迁移到正式凭据管理。
- 明确敏感信息存储方案：
  - Windows 可优先使用 DPAPI 或用户级密钥文件加密。
  - 跨平台 fallback 使用本地密钥文件，权限限制并提示风险。
- 日志脱敏：
  - cookie、token、password、Authorization、passkey。
- 导入能力：
  - 从 curl/fetch.sh 导入 cookie 到数据库。
  - 从 JSON cookie 导入。
  - 手动编辑 headers/UA。
- 账号密码登录作为后续扩展，不阻塞当前 cookie 流程。

测试：

- cookie 导入后可读取使用。
- API 和日志不返回敏感值。
- 密钥缺失、密文损坏、配置非法时错误明确。

## 12. 可选扩展，暂不进入 MVP

- Jackett/Prowlarr 外部索引器。
- 多下载器统一抽象。
- 插件系统。
- 媒体识别、TMDB/豆瓣/Bangumi。
- 文件整理、媒体库刮削、字幕下载。
- 自动签到和站点用户统计。
- Redis/分布式锁。

这些能力参考项目已经覆盖较广，但会显著扩大本项目范围。NexusBridge 当前应先完成“站点资源桥接”闭环。

## 推荐执行顺序

1. 站点定义 JSON schema 与 SiteEngine。
2. 配置驱动 HTML 字段提取 DSL。
3. SQLite 数据模型与 repository。
4. 抓取服务整合：站点定义 + cookie + fetch + parse + upsert。
5. 查询与筛选规则引擎。
6. RSS 输出。
7. qBittorrent WebUI client。
8. 下载流水线与自动下载状态。
9. 调度器与任务状态。
10. Web API 与 WebUI。
11. 凭据安全存储和日志脱敏。
