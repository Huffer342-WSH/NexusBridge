# 测试说明

Go 测试代码统一放在 `tests/` 目录。当前真实站点相关测试拆成三类：

1. `TestFetchTorrentsPageHTML`
   - 从 `fetch.sh` 读取 cookie 和 headers。
   - 将 cookie 写入 SQLite。
   - 从 SQLite 读取 cookie。
   - 使用数据库中的 cookie 抓取 `torrents.php`。
   - 保存 HTML 到本地文件，供人工检查。
2. `TestParseSearchConfigFromFetchedHTML`
   - 从新格式站点 JSON 读取 URL 和 HTML selector。
   - 从 SQLite 读取 cookie，重新抓取 HTML 到内存。
   - 解析搜索框中的可选搜索项。
   - 将搜索项快照保存到文件。
3. `TestParseTorrentsFromFetchedHTML`
   - 从新格式站点 JSON 读取 URL 和 HTML selector。
   - 从 SQLite 读取 cookie，重新抓取 HTML 到内存。
   - 解析当前页种子列表到结构体。
   - 在测试日志中打印结构体 JSON。
4. `TestTorrentDetail`
   - 从 `fetch.sh` 读取 cookie 和 headers。
   - 抓取真实站点种子列表，选择当前页第一条种子。
   - 将该种子写入 SQLite。
   - 使用数据库中的 cookie 抓取该种子的 `detail_url`。
   - 解析并持久化详情页字段。

MVP 单元测试不依赖真实站点：

- `TestRuleMatchAndTorrentUpsert`：验证筛选规则命中和 SQLite 种子 upsert 去重/变化判断。
- `TestParseTorrentDetailAllowsMissingOptional`：验证详情页缺少可选字段时仍能解析基础标题。
- `TestFakeTorrentDetailFetchAndPersist`：使用 fake 站点验证详情页抓取、cookie/user-agent、解析和 SQLite 持久化。
- `TestLLMBasicChat`：使用 fake LLM 服务验证基础 Chat Completions 调用。
- `TestLLMExtractCosplayVideo`：验证 cosplay 视频信息结构化提取。
- `TestOrganizerUsesLLMAndCreatesHardlink`：使用 fake LLM 服务验证整理输出和硬链接创建。
- `TestOrganizerRejectsUnsafeLLMPath`：验证 LLM 返回不安全相对路径时会被拒绝。
- `TestDesktopPrepareData`：验证桌面用户目录、默认配置、内置站点安装、绝对路径解析及已有站点文件不覆盖。
- `TestDesktopRuntimeServesSharedAPI`：验证 Wails 中间件复用现有 `/api` Handler，同时将非应用请求交回静态资源 Handler。
- `TestRuntimeConfigDevelopmentDataDir`：验证开发与测试默认使用仓库 `data/`。
- `TestRuntimeConfigDataDirOverride`：验证统一数据目录覆盖、配置生成和相对路径解析。
- `TestRuntimeConfigProductionBootstrap`：使用 `production` 标签验证可执行文件旁的引导配置。

桌面生产构建使用 `wails3 build`，产物为 `bin/nexusbridge-desktop`（Windows 带 `.exe`）。冒烟检查应使用独立的 `NEXUSBRIDGE_DATA_DIR`，确认配置、SQLite 和 KamePT 自动创建，重复启动不会产生第二个后台实例，关闭窗口后进程继续驻留，托盘“完全退出”后进程结束。Linux amd64/aarch64 桌面发布分别在对应架构 runner 上执行原生 CGO 构建。

qBittorrent 测试使用真实 WebAPI，不使用 fake 服务：

- `TestQBittorrentClientReadRealAPI`：读取 qB WebAPI 版本、分类、标签、任务列表；若已有任务，则读取 properties 和 contents。
- `TestQBittorrentClientClosedLoopRealAPI`：执行“读取 -> 新增 -> 读取 -> 删除 -> 读取”闭环。测试会使用 `paused=true` 添加任务，结束后调用 `torrents/delete` 删除测试任务，`deleteFiles=false`，不会删除下载目录文件。
- `TestParseTorrentHashes`：使用本地 metainfo 验证 v1、v2 和 hybrid info hash 解析，不访问网络。
- `TestQBittorrentTorrentFileHashRealAPI`：下载 `.torrent` 文件、本地计算并打印 v1/v2 hash，以文件方式添加到 qB，再重复添加一次；验证首次响应 ID 与本地 hash 一致，并验证第二次的 `409 Conflict` 可按 hash 协调为已有任务。若 qB 中已存在该 hash，则跳过以免删除已有任务。
- `TestTorrentFileAndQBSnapshotPersistence`：验证 torrent BLOB/hash 保存、元数据查询不加载 BLOB、qB 快照替换以及已移除任务标记。
- `TestRequestPolicyCookieSelection`：验证同主机全量 Cookie、严格域名后缀、最长规则优先和 KamePT 图片域白名单。
- `TestRequestPolicyMissingCookieStillRequests`：验证规则 Cookie 缺失时保留诊断信息并继续无 Cookie 请求。
- `TestFetcherReevaluatesCookiesOnRedirect`：验证 HTTP 重定向重新解析策略且不会泄露上一跳 Cookie。
- `TestQBPollUpdatesTorrentSnapshot`：验证 qB `sync/maindata` 完整响应和部分响应按 hash 合并到本地快照。

## 运行

```sh
$env:NEXUSBRIDGE_TEST_CURL_FILE = "fetch.sh"
go test ./tests -v
```

未设置 `NEXUSBRIDGE_TEST_BASE_URL` 时，真实站点测试默认使用 `https://kamept.com`。发布前需要替换为通用示例或外部参数。

单独运行：

```sh
go test ./tests -run TestFetchTorrentsPageHTML -v
go test ./tests -run TestParseSearchConfigFromFetchedHTML -v
go test ./tests -run TestParseTorrentsFromFetchedHTML -v
go test ./tests -run TestTorrentDetail -v
go test ./tests -run "TestParseTorrentDetail|TestFakeTorrentDetail|TestLLM" -v
go test ./tests -run TestQBittorrent -v
go test ./tests -run TestQBittorrentTorrentFileHashRealAPI -v
go test ./tests -run "TestRuleMatchAndTorrentUpsert|TestOrganizer|TestTorrentFileAndQBSnapshotPersistence|TestRequestPolicy|TestFetcherReevaluatesCookiesOnRedirect|TestQBPoll" -v
go test -tags production ./tests -run TestRuntimeConfigProductionBootstrap -v
```

解析测试依赖 `data/tests/cookies.db` 中已有 cookie。通常先运行 `TestFetchTorrentsPageHTML`。

## 前端自动化测试

WebUI 使用 Playwright 做浏览器自动化测试，测试代码放在 `webui/tests/e2e/`。

首次运行前安装 Chromium 测试浏览器：

```sh
cd webui
pnpm run test:e2e:install
```

运行前端端到端测试：

```sh
cd webui
pnpm run test:e2e
```

Playwright 配置会自动启动 Vite 开发服务器 `127.0.0.1:5173`。当前冒烟测试通过 mock API 验证 WebP 封面代理、媒体列表/卡片快捷设置及持久化、状态控件不越界、默认图标态与 hover 展开态、qB 暂停后的即时 UI 更新、自动轮询和设置子页面，不依赖真实站点、cookie 或后端服务。

## 输入

默认输入：

- `fetch.sh`
- `tests/fixtures/site_parse_config.json`
- `tests/fixtures/torrents_page.html`：匿名 parser fixture，仅用于本地结构兼容测试和维护参考。

可选环境变量：

- `NEXUSBRIDGE_TEST_DB_FILE`：覆盖 cookie 数据库路径。
- `NEXUSBRIDGE_TEST_SAVE_HTML`：覆盖 HTML 保存路径。
- `NEXUSBRIDGE_TEST_SITE_CONFIG`：覆盖预设站点解析 JSON。
- `NEXUSBRIDGE_TEST_FILLED_SITE_CONFIG`：覆盖搜索项快照输出路径。
- `NEXUSBRIDGE_TEST_PRINT_HTML=1`：将 HTML 打印到终端。
- `NEXUSBRIDGE_TEST_QB_URL`：qBittorrent WebUI 根地址。可填 `http://ip:port`，也可填裸 `ip:port`。
- `NEXUSBRIDGE_TEST_QB_API_KEY`：qBittorrent API Key。qB 版本需要支持 API Key 认证。
- `NEXUSBRIDGE_TEST_QB_TORRENT_URL`：qB 闭环新增测试使用的 torrent、magnet 或 http(s) 下载链接；hash 对比测试要求它是可直接下载的 http(s) `.torrent` URL。未设置时跳过真实写入测试。
- `NEXUSBRIDGE_TEST_QB_CATEGORY`：qB 闭环测试分类，默认 `nexusbridge-test`。
- `NEXUSBRIDGE_TEST_QB_TAG`：qB 闭环测试标签，默认 `nexusbridge-test`。

## 输出

默认输出：

- `data/tests/cookies.db`
- `data/tests/torrents_page.html`
- `data/tests/<site>.updated.json`

## 约束

- `fetch.sh` 仅作为测试输入文件，不是正式应用配置。
- 解析测试从数据库读取 cookie，不应直接把 cookie 写入配置 JSON。
- 不要提交 cookie 文件、包含 cookie 的 curl 示例、账号密码或真实 HTML 响应。
- 不要提交 qBittorrent API Key 或真实 torrent 测试链接；qB 写入闭环测试只能通过环境变量启用。
