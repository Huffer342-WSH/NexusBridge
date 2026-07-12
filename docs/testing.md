# 测试说明

Go 测试代码统一放在 `tests/` 目录。HTML 相关测试先跑离线 fixture，再跑在线抓取；`fetch.sh` 只允许作为测试输入，不是正式应用配置。

## 解析

### 解析搜索

说明：

- `TestCompleteSiteDefinitionSearchConfigFromFixtureHTML` 使用不完整站点 JSON 和本地 HTML，解析搜索框并导出补完后的站点 JSON。
- 输入为 `tests/fixtures/site_parse_config.json` 和 `tests/fixtures/torrents_page.html`。
- 输出写入测试临时目录，重新加载后验证 `html.search.fields` 中的 checkbox、select、数字范围、日期范围、关键字和独占 `tag_id`。
- `TestParseSearchConfigFromFetchedHTML` 是在线版本，从 cookie DB 读取凭据后抓取真实 HTML，再导出 `data/tests/<site>.updated.json`。

指令：

```sh
go test ./tests -run TestCompleteSiteDefinitionSearchConfigFromFixtureHTML -v
go test ./tests -run TestParseSearchConfigFromFetchedHTML -v
```

### 解析种子

说明：

- `TestParseTorrentsFromFixtureHTML` 使用本地 HTML 和站点 JSON 验证种子列表解析，不访问网络。
- `TestParseTorrentsFromFetchedHTML` 是在线版本，从真实页面解析当前页种子并在日志中打印结构体 JSON。
- 种子字段规则来自站点 JSON 的 `html.torrents.fields`；未配置字段会回退到 NexusPHP 默认结构。

指令：

```sh
go test ./tests -run TestParseTorrentsFromFixtureHTML -v
go test ./tests -run TestParseTorrentsFromFetchedHTML -v
```

### 解析详情

说明：

- `TestParseTorrentDetailAllowsMissingOptional` 验证详情页缺少可选字段时仍能解析基础标题。
- `TestFakeTorrentDetailFetchAndPersist` 使用 fake 站点验证详情页抓取、cookie/user-agent、解析和 SQLite 持久化。
- `TestTorrentDetail` 是真实站点详情页测试，先抓取列表页第一条种子，再抓取并持久化详情字段。

指令：

```sh
go test ./tests -run "TestParseTorrentDetail|TestFakeTorrentDetail" -v
go test ./tests -run TestTorrentDetail -v
```

### 解析 torrent 文件

说明：

- `TestParseTorrentHashes` 使用本地 metainfo 验证 v1、v2 和 hybrid info hash 解析，不访问网络。

指令：

```sh
go test ./tests -run TestParseTorrentHashes -v
```

## 通信

### 站点通信

说明：

- `TestFetchTorrentsPageHTML` 从 `fetch.sh` 读取 cookie 和 headers，写入 `data/tests/cookies.db`，再用数据库 cookie 抓取 `torrents.php`。
- 抓取到的 HTML 默认保存到 `data/tests/torrents_page.html`，供后续在线解析测试和人工检查。
- 在线解析测试依赖该 cookie DB，通常先运行抓取测试。

指令：

```sh
$env:NEXUSBRIDGE_TEST_BASE_URL = "https://example.invalid"
$env:NEXUSBRIDGE_TEST_CONFIG = "data/config.json"
$env:NEXUSBRIDGE_TEST_SITE_ID = "kamept"
$env:NEXUSBRIDGE_TEST_CURL_FILE = "fetch.sh"
go test ./tests -run TestFetchTorrentsPageHTML -v
```

### qBittorrent 通信

说明：

- qBittorrent 测试使用真实 WebAPI，不使用 fake 服务。
- `TestQBittorrentClientReadRealAPI` 读取版本、分类、标签、任务列表以及已有任务详情。
- `TestQBittorrentClientClosedLoopRealAPI` 执行读取、新增、读取、删除、读取闭环，测试任务使用 `paused=true` 添加，结束后 `deleteFiles=false` 删除任务。
- `TestQBittorrentTorrentFileHashRealAPI` 下载 `.torrent` 文件并验证本地 hash、qB 添加响应和重复添加协调。
- `TestTorrentFileAndQBSnapshotPersistence`、`TestQBPollUpdatesTorrentSnapshot` 覆盖本地 BLOB/hash、qB 快照和增量同步。

指令：

```sh
go test ./tests -run TestQBittorrent -v
go test ./tests -run TestQBittorrentTorrentFileHashRealAPI -v
go test ./tests -run "TestTorrentFileAndQBSnapshotPersistence|TestQBPoll" -v
```

### LLM 与整理通信

说明：

- `TestLLMBasicChat` 使用 fake LLM 服务验证基础 Chat Completions 调用。
- `TestLLMExtractCosplayVideo` 验证 cosplay 视频信息结构化提取。
- `TestOrganizerUsesLLMAndCreatesHardlink` 使用 fake LLM 服务验证整理输出和硬链接创建。
- `TestOrganizerRejectsUnsafeLLMPath` 验证 LLM 返回不安全相对路径时会被拒绝。

指令：

```sh
go test ./tests -run "TestLLM|TestOrganizer" -v
```

### 请求策略通信

说明：

- `TestRequestPolicyCookieSelection` 验证同主机全量 Cookie、严格域名后缀、最长规则优先和 KamePT 图片域白名单。
- `TestRequestPolicyMissingCookieStillRequests` 验证规则 Cookie 缺失时保留诊断信息并继续无 Cookie 请求。
- `TestFetcherReevaluatesCookiesOnRedirect` 验证 HTTP 重定向重新解析策略且不会泄露上一跳 Cookie。

指令：

```sh
go test ./tests -run "TestRequestPolicy|TestFetcherReevaluatesCookiesOnRedirect" -v
```

## 本地服务与桌面

说明：

- `TestRuleMatchAndTorrentUpsert` 验证筛选规则命中和 SQLite 种子 upsert 去重/变化判断。
- `TestDesktopPrepareData` 验证桌面用户目录、默认配置、内置站点安装、绝对路径解析及已有站点文件不覆盖。
- `TestDesktopRuntimeServesSharedAPI` 验证 Wails 中间件复用现有 `/api` Handler。
- `TestRuntimeConfigDevelopmentDataDir`、`TestRuntimeConfigDataDirOverride`、`TestRuntimeConfigProductionBootstrap` 验证运行目录策略。

指令：

```sh
go test ./tests -run "TestRuleMatchAndTorrentUpsert|TestDesktop|TestRuntimeConfig" -v
go test -tags production ./tests -run TestRuntimeConfigProductionBootstrap -v
```

## 前端自动化测试

说明：

- WebUI 使用 Playwright 做浏览器自动化测试，测试代码放在 `webui/tests/e2e/`。
- Playwright 会自动启动 Vite 开发服务器 `127.0.0.1:5173`。
- 当前冒烟测试通过 mock API 验证媒体页、设置页、封面代理、状态控件和轮询行为，不依赖真实站点、cookie 或后端服务。

指令：

```sh
cd webui
pnpm run test:e2e:install
pnpm run test:e2e
```

## 输入输出与约束

默认输入：

- `fetch.sh`
- `tests/fixtures/site_parse_config.json`
- `tests/fixtures/torrents_page.html`

默认输出：

- `data/tests/cookies.db`
- `data/tests/torrents_page.html`
- `data/tests/<site>.updated.json`

可选环境变量：

- `NEXUSBRIDGE_TEST_BASE_URL`：真实站点基础地址。
- `NEXUSBRIDGE_TEST_CONFIG`：测试使用的应用配置。
- `NEXUSBRIDGE_TEST_SITE_ID`：测试站点 ID。
- `NEXUSBRIDGE_TEST_CURL_FILE`：包含 cookie 和 headers 的 curl 输入。
- `NEXUSBRIDGE_TEST_DB_FILE`：覆盖 cookie 数据库路径。
- `NEXUSBRIDGE_TEST_SAVE_HTML`：覆盖 HTML 保存路径。
- `NEXUSBRIDGE_TEST_UPDATED_SITE_CONFIG`：覆盖补完站点 JSON 输出路径。
- `NEXUSBRIDGE_TEST_PRINT_HTML=1`：将 HTML 打印到终端。
- `NEXUSBRIDGE_TEST_QB_URL`：qBittorrent WebUI 根地址。
- `NEXUSBRIDGE_TEST_QB_API_KEY`：qBittorrent API Key。
- `NEXUSBRIDGE_TEST_QB_TORRENT_URL`：qB 闭环新增测试使用的 torrent、magnet 或 http(s) 下载链接。
- `NEXUSBRIDGE_TEST_QB_CATEGORY`：qB 闭环测试分类，默认 `nexusbridge-test`。
- `NEXUSBRIDGE_TEST_QB_TAG`：qB 闭环测试标签，默认 `nexusbridge-test`。

约束：

- 不要提交 cookie 文件、包含 cookie 的 curl 示例、账号密码或真实 HTML 响应。
- 解析测试从数据库读取 cookie，不应直接把 cookie 写入配置 JSON。
- 不要提交 qBittorrent API Key 或真实 torrent 测试链接；qB 写入闭环测试只能通过环境变量启用。
