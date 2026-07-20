# 测试说明

Go 测试代码统一放在 `tests/` 目录。HTML 相关测试先跑离线 fixture，再跑在线抓取；`fetch.sh` 只允许作为测试输入，不是正式应用配置。

## 本地 GitHub Actions 打包

Windows 上的 Docker Desktop 使用 Linux 容器模式时，可通过 `act` 运行 `build-release.yml` 的 Linux job。先运行 dry-run 检查 workflow 选择和 runner 映射：

```powershell
.\scripts\ci\run-act.ps1 -Job linux-arm64 -DryRun
```

完整执行 ARM64 打包：

```powershell
.\scripts\ci\run-act.ps1 -Job linux-arm64
```

默认产物为 `dist/nexusbridge-webui-linux-aarch64.tar.gz` 和 `dist/nexusbridge-desktop-linux-aarch64.tar.gz`。本地 `act` 跳过 GitHub artifact 上传，压缩包直接保留在 `dist/`。首次执行需要构建 ARM64 runner 镜像并下载 Actions，QEMU 仿真下的 Wails/Go 编译可能比原生 ARM64 runner 慢很多。

正式运行 ARM64 job 前，脚本会执行 `docker run --rm --platform linux/arm64 ubuntu:24.04 uname -m`，预期输出为 `aarch64`。如果基础镜像也出现 `exec format error`，说明 Docker Desktop 当前的 ARM64 模拟环境不可用，并非 Go 编译错误；重启 Docker Desktop、确认使用 Linux 容器模式后再次执行该检查。如果基础镜像正常而本地 runner 异常，使用 `-RebuildRunner` 重建 runner 镜像。

如果需要使用主机 `7890` HTTP 代理，显式传入容器可访问的地址：

```powershell
.\scripts\ci\run-act.ps1 -Job linux-arm64 -ProxyUrl http://host.docker.internal:7890
```

默认指令不会绕过 PowerShell 执行策略。如果 `.ps1` 被策略拦截，先确认脚本可信，再使用当前用户的 `RemoteSigned` 策略，或仅对单次命令显式使用 `-ExecutionPolicy Bypass`。

`build-release.yml` 是构建入口，`publish-release.yml` 是构建成功后调用的独立发布 workflow。`act` 固定传入 `publish_release=false`，因此本地打包不会创建 tag 或 GitHub Release。正式推送 `v*` tag 会全量构建并发布；手动发布必须选择 `all` 和 `publish_release=true`，空 `release_tag` 会生成 `manual-YYYY.MM.DD.<run_number>` prerelease tag。

修改 CI 后至少执行以下静态检查；这些检查不创建远程 tag 或 Release：

```sh
bash -n scripts/ci/build-release-linux.sh
git diff --check
```

```powershell
$errors = $null
[System.Management.Automation.Language.Parser]::ParseFile(
  (Resolve-Path .\scripts\ci\build-release-windows.ps1),
  [ref]$null,
  [ref]$errors
) | Out-Null
if ($errors.Count -gt 0) { $errors | ForEach-Object { Write-Error $_ } }
```

如果本机安装了 `actionlint`，同时运行 `actionlint .github/workflows/build-release.yml .github/workflows/publish-release.yml`。完整发布验证必须在 GitHub Actions 上进行，因为 tag 推送、跨 job artifact 下载、Release Notes 的贡献者查询和 GitHub Release 创建都依赖仓库权限与 `GITHUB_TOKEN`。

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

- `TestParseTorrentsFromFixtureHTML` 使用本地 HTML 和站点 JSON 验证种子列表解析，不访问网络，并在日志中打印结构体 JSON。
- `TestParseTorrentsFromSavedKamePTHTML` 使用 `data/tests/torrents_page.html` 和内置 KamePT JSON 验证完整保存页；文件不存在时跳过。
- `TestParseTorrentsFromFetchedHTML` 是在线版本，从真实页面解析当前页种子并在日志中打印结构体 JSON。
- 种子字段规则来自站点 JSON 的 `html.torrents.fields`；未配置字段会回退到 NexusPHP 默认结构。
- 列表页 `<br>` 后的文本归入 `subtitle`，并按空白拆分为 `tags`；彩色 `span` 归入 `tag_ids`；`description` 留给详情页简介。subtitle 拆出来的 tag 会按站点 JSON 的长度上限过滤，避免整段文本误入 tags。

指令：

```sh
go test ./tests -run TestParseTorrentsFromFixtureHTML -v
go test ./tests -run TestParseTorrentsFromSavedKamePTHTML -v
go test ./tests -run TestParseTorrentsFromFetchedHTML -v
```

### 解析详情

说明：

- `TestParseTorrentDetailFromFetchedHTML` 是真实站点详情页测试，先打印列表页第一条种子的解析结果，再抓取、持久化并打印详情补充后的结果。

指令：

```sh
go test ./tests -run TestParseTorrentDetailFromFetchedHTML -v
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
- `TestQBittorrentTorrentDetailsRealAPI` 通过 `NEXUSBRIDGE_TEST_QB_HASH` 指定已有任务，完整打印 properties 和文件明细，不修改任务。
- `TestQBittorrentRenameRealAPI` 通过 `go test -args` 指定已有任务 hash、完整旧路径和新路径，可选择 `file` 或 `folder` 验证 `renameFile`/`renameFolder`。测试默认在验证后反向重命名恢复原路径；这是会短暂修改真实任务的实验测试，运行前应暂停任务并核对参数。
- `TestQBittorrentClientClosedLoopRealAPI` 执行读取、新增、读取、删除、读取闭环，测试任务使用独立分类、标签并以 `paused=true` 添加，结束后始终以 `deleteFiles=false` 删除本次新增任务。
- `TestQBittorrentTorrentFileHashRealAPI` 下载 `.torrent` 文件并验证本地 hash、qB 添加响应和重复添加协调。
- `TestTorrentFileAndQBSnapshotPersistence`、`TestQBPollUpdatesTorrentSnapshot` 覆盖本地 BLOB/hash、qB 快照和增量同步。
- `TestTorrentFileSizeIndexLifecycle` 覆盖文件大小倒排行、重复大小计数、站点过滤、完整多重集合查询，以及 torrent BLOB 保存、覆盖和删除时的事务内维护；它还验证旧 BLOB 的重建结果不能覆盖并发保存的新索引。`TestManualTorrentSizeIndexRebuild` 覆盖旧 BLOB 的显式重建和单项解析失败隔离。
- `TestRecoverRealAPI` 先显式重建一次旧数据大小索引，再对 `NEXUSBRIDGE_TEST_RECOVERY_HASH` 指定的 v1 hash 依次运行 `site`、`database`、`database_then_site`、`torrent_url`，以及网页搜索关闭/开启的两个自动批量恢复子测试。删除前验证文件管理器标记任务；首次删除后验证只读扫描能发现候选；每次都调用 `deleteFiles=false`。所有入口都必须按完整大小集合命中，并用 qB `renameFile` 映射回原磁盘路径；恢复必须暂停且跳过初始校验添加，配置完成后强制校验，并断言至少触发一次校验、结果完整且已开始做种。当原任务分类的保存目录与恢复 `save_path` 一致时，还会断言恢复结果和 qB 任务都自动设置该分类。失败路径会用已确认的数据库候选尝试恢复，但运行前仍应确认目标文件或目录已完整保留。
- 订阅真实闭环需要使用本次测试唯一的分类、标签和名称，按本地 v1 hash 验证；如果 hash 在测试前已经存在则跳过写入和清理，绝不修改预先存在的任务。纯 v2 输入应明确失败。

指令：

```sh
go test ./tests -run TestQBittorrent -v
$env:NEXUSBRIDGE_TEST_QB_HASH = "<existing-qb-torrent-hash>"
go test ./tests -run TestQBittorrentTorrentDetailsRealAPI -v
go test ./tests -run TestQBittorrentRenameRealAPI -count=1 -v -args -hash="<hash>" -oldpath="<old-relative-path>" -newpath="<new-relative-path>" -kind=file -restore=true
go test ./tests -run TestQBittorrentTorrentFileHashRealAPI -v
go test ./tests -run "TestTorrentFileAndQBSnapshotPersistence|TestQBPoll" -v
$env:NEXUSBRIDGE_TEST_RECOVERY_HASH = "<40-character-v1-info-hash>"
$env:NEXUSBRIDGE_TEST_RECOVERY_REAL = "1"
go test ./tests -run TestRecoverRealAPI -v
```

fake qB 测试不需要环境变量，使用 `httptest` 精确断言 `torrents/add` multipart 中的 `savepath`、`category`、`tags`、`paused`、`rename` 和显式路径对应的 `autoTMM=false`，并覆盖 `200`、`202`、重复 hash、响应 body、失败重试与并发重放。它与上面的真实 WebAPI 闭环分开运行。

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

- `TestRuleMatchAndTorrentUpsert` 验证主标题逻辑表达式、站点分类、两类标签、促销、多组范围、稳定拒绝原因、SQLite 种子 inserted/changed 区分和默认页面顺序。
- 筛选/订阅持久化测试覆盖全新自然键表结构、规则级联改名、规则与订阅 roundtrip、qB category/tag 离线快照、首个订阅独占、配额候选保持 `unread` 和下载任务幂等。
- 调度测试使用 fake clock，覆盖动态周期、同站防重入、context 取消、多个订阅优先级和本地自然日跨日配额恢复。
- `TestSQLiteLockContentionRecovers` 使用第二条真实 SQLite 连接持有独占写事务，验证 WAL 下调度读取不中断，以及连接池中的等待写入会在锁释放后恢复。
- API 测试覆盖草稿 preview 零写入、命中优先排序、本地 qB 三态、站点筛选选项、规则级联改名、规则/订阅 CRUD、分类创建、批量快速应用/临时 options 和任务 retry。
- `TestDesktopPrepareData` 验证桌面用户目录、默认配置、内置站点安装、绝对路径解析及已有站点文件不覆盖。
- `TestDesktopRuntimeServesSharedAPI` 验证 Wails 中间件复用现有 `/api` Handler。
- `TestRuntimeConfigDevelopmentDataDir`、`TestRuntimeConfigDataDirOverride`、`TestRuntimeConfigProductionBootstrap` 验证运行目录策略。

指令：

```sh
go test ./tests -run "TestRuleMatchAndTorrentUpsert|TestSubscription|TestSiteSchedule|TestSQLiteLockContentionRecovers|TestDownload|TestDesktop|TestRuntimeConfig" -v
go test -tags production ./tests -run TestRuntimeConfigProductionBootstrap -v
```

## 前端自动化测试

说明：

- WebUI 使用 Playwright 做浏览器自动化测试，测试代码放在 `webui/tests/e2e/`。
- Playwright 会自动启动 Vite 开发服务器 `127.0.0.1:5173`。
- 冒烟测试通过 mock API 验证媒体页、设置页、封面代理、状态控件和轮询行为，不依赖真实站点、cookie 或后端服务。
- `webui/tests/e2e/subscriptions.spec.ts` 覆盖创建/预览筛选规则、创建/预览订阅、同步与新建完整 qB 分类、站点周期、手动批量确认执行，以及未读配额原因和最近运行展示。
- `dashboard.spec.ts` 的文件管理器用例覆盖目录返回/前进按钮、鼠标第 4/5 键、默认事件拦截和页面 URL 不变，并验证扫描默认只查数据库、网页搜索开关以及一次提交全部唯一候选的自动批量恢复。

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
- `NEXUSBRIDGE_TEST_QB_HASH`：只读详情测试要打印的已有 qB 任务 hash。
- `NEXUSBRIDGE_TEST_RECOVERY_HASH`：真实恢复试验的 40 位 v1 info hash。
- `NEXUSBRIDGE_TEST_RECOVERY_REAL=1`：显式启用会删除并重建已有 qB 任务的真实恢复试验。

约束：

- 不要提交 cookie 文件、包含 cookie 的 curl 示例、账号密码或真实 HTML 响应。
- 解析测试从数据库读取 cookie，不应直接把 cookie 写入配置 JSON。
- 不要提交 qBittorrent API Key 或真实 torrent 测试链接；qB 写入闭环测试只能通过环境变量启用。
- 普通真实 qB 闭环只清理本次新增且已按 hash 确认的任务；测试前已经存在的 hash、分类或标签不能作为清理对象。唯一例外是同时显式设置上述两个恢复试验变量的 `TestRecoverRealAPI`。
- 所有真实 qB 测试的删除调用都必须使用 `deleteFiles=false`。
