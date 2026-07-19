# 配置说明

NexusBridge 使用 JSON 配置。CLI、浏览器服务和桌面端统一通过 `internal/runtimeconfig` 解析配置与数据目录。

开发构建和测试默认使用仓库的 `data/`，配置文件为 `data/config.json`；文件不存在时会自动生成。正式构建默认使用系统用户配置目录下的 `NexusBridge`：Windows 对应 `%AppData%\NexusBridge`，Linux 通常对应 `$XDG_CONFIG_HOME/NexusBridge` 或 `~/.config/NexusBridge`。

所有运行形态首次启动都会安装程序内置的 KamePT 到 `sites/html/kamept.json`，并安装通用站点索引规则到 `sites/html/common/nexusphp_search.json`。主配置、同名站点文件和同名索引规则都只在目标不存在时排他创建，已有文件保持原内容，不会被启动或升级覆盖。`NEXUSBRIDGE_DATA_DIR` 可覆盖数据根目录；旧的 `NEXUSBRIDGE_DESKTOP_DATA_DIR` 仅为兼容保留。显式传入 `--config` 时，其所在目录成为数据根目录。

Docker 一体化镜像固定设置 `NEXUSBRIDGE_DATA_DIR=/config/nexusbridge`，首次启动安装适用于容器的 `config.json`，其中 `server.host` 为 `0.0.0.0`、qB 地址为 `http://127.0.0.1:8080`。qBittorrent 配置位于 `/config/qBittorrent`，mihomo 配置位于 `/config/mihomo`；三者均通过同一个 `/config` 卷持久化，已有配置不会被镜像默认值覆盖。部署细节见 [Docker 说明](../docker/README.md)。

Mihomo Provider 设置默认读取 `~/.config/mihomo/config.yaml`；当该目录不存在且设置了 `XDG_CONFIG_HOME` 时，使用 `$XDG_CONFIG_HOME/mihomo`。`NEXUSBRIDGE_MIHOMO_CONFIG_DIR` 可显式覆盖默认目录，Docker 镜像将其设为 `/config/mihomo`。WebUI 中选定的目录保存在 NexusBridge 数据库，新增 Provider 只追加 `proxy-providers` 节点，不覆盖同名 Provider。

正式版还会在可执行文件旁读取 `nexusbridge.bootstrap.json`：

```json
{
  "data_dir": "data"
}
```

绝对路径直接使用；相对路径以可执行文件目录为基准。旁置文件不存在时才回退用户配置目录。仓库的 `nexusbridge.bootstrap.example.json` 可作为模板。

默认值：

- `server.host`：`0.0.0.0`
- `server.port`：`8090`
- `storage.path`：`nexusbridge.db`
- `sites_dir`：`sites/html`
- `logging.level`：`info`
- `auth.enabled`：`false`
- `network.mode`：`system`

示例：

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8090
  },
  "storage": {
    "path": "nexusbridge.db"
  },
  "sites_dir": "sites/html",
  "logging": {
    "level": "debug",
    "file": "logs/nexusbridge.log",
    "log_sensitive_fetch": false
  },
  "auth": {
    "enabled": false,
    "username": "",
    "password": ""
  },
  "qbittorrent": {
    "auth_mode": "uid",
    "url": "http://127.0.0.1:8080",
    "api_key": "",
    "username": "",
    "user_id": "",
    "password": "",
    "category": "nexusbridge",
    "tags": ["nexusbridge"]
  },
  "llm": {
    "base_url": "",
    "api_key": "",
    "model": ""
  },
  "network": {
    "mode": "system",
    "proxy_url": "",
    "no_proxy": "localhost\n127.*\n192.168.*"
  },
  "media_library": {
    "root": "",
    "dry_run": true
  },
  "rules": []
}
```

完整示例位于 `data/config.example.json`。站点不通过主配置的 `sites[]` 配置；运行时只配置 `sites_dir`，服务会加载目录中的多个站点 JSON。

`auth.enabled=true` 时必须配置 `auth.username` 和 `auth.password`。当前骨架只提供登录接口和 WebUI 登录流程，完整 API 鉴权可在后续认证里程碑补齐。

## 日志

`logging.level` 支持 `debug`、`info`、`warn`、`error`。日志会输出到终端；配置 `logging.file` 后会同时追加写入文件。

`logging.log_sensitive_fetch=true` 时可输出额外请求头用于调试，但 `Cookie` 始终脱敏且只记录名称，不会输出值。正式发布或共享日志前仍建议改为 `false`。

默认示例把日志文件写入数据目录下的 `logs/nexusbridge.log`。

## 网络代理

`network` 只通过标准环境变量控制应用的 HTTP 访问，不会替换或修改任何 HTTP 客户端、Transport 或调用点。qBittorrent WebUI 也使用默认客户端，其地址会自动追加到 `NO_PROXY`。

- `network.mode`：`system`、`manual` 或 `direct`，默认 `system`。
- `network.proxy_url`：仅在 `manual` 时必填；支持 `http://`、`https://` 和 `socks5://`，省略协议时按 HTTP 代理处理。
- `network.no_proxy`：一行一个直连规则；可使用 `127.*`、`192.168.*` 等 IPv4 通配符以及 `*.example.com` 域名通配符。

系统代理会恢复进程启动时继承的 `HTTP_PROXY`、`HTTPS_PROXY` 环境变量；手动代理会写入这两个变量，直连模式会清除它们。所有模式都会将 `network.no_proxy` 转换为 Go 标准库支持的 `NO_PROXY` 格式，并追加 qBittorrent WebUI 的主机地址。保存时界面可用“恢复默认 NO_PROXY”载入常用局域网、Steam、Bilibili 和本地域名规则。

WebUI 的 `设置 / 网络代理` 页面保存到 SQLite，优先级高于主配置。应用启动时会先写入环境变量，再由 Go 标准 HTTP 客户端读取。Go 标准库会缓存环境代理决策，因此运行中保存设置后必须重启 NexusBridge 才能可靠生效。

## 抓取凭据

当前抓取阶段只支持通过外部 cookie 访问页面，不实现账号密码登录。

正式运行时，WebUI 的 `设置 / 站点` 页面可以为每个站点分别保存 `Cookie` 和 `User-Agent`。服务端会把 cookie 写入 SQLite 的 cookie 表，并按站点基础 URL 隔离；API 响应只返回 `has_cookie`，不回显 cookie 明文。

所有生产站点请求统一在发起前选择 Cookie：同主机发送当前站点全部 Cookie，跨主机默认不发送。需要跨主机复用指定 Cookie 时，在所属站点 JSON 顶层配置 `request_rules`：

```json
{
  "request_rules": [
    {
      "domain_suffix": "p.kamept.com",
      "cookie_names": ["cf_clearance"]
    }
  ]
}
```

`domain_suffix` 只填写主机名，不包含协议、端口、路径或通配符；匹配主机本身及其点分隔子域。`cookie_names` 是区分大小写的名称白名单，禁止留空。多个规则命中时采用最长后缀。指定 Cookie 缺失时仍尝试无 Cookie 请求并在日志中记录缺失名称。重定向到新主机时会重新匹配规则，不继承上一跳 Cookie。

KamePT 图片域通常还要求与 `cf_clearance` 配套的浏览器 User-Agent；两者均通过 WebUI 的站点凭据设置更新。实际 Cookie 值不得写入站点定义。仓库参考定义已包含规则，已复制到 `data/sites/html` 的旧文件不会自动迁移。

测试通过以下方式提供 cookie：

- `NEXUSBRIDGE_TEST_CURL_FILE`：包含 `curl -b 'name=value; ...'` 的脚本文件。

真实页面抓取测试会将 cookie 写入 SQLite，默认位置为 `data/tests/cookies.db`。该数据库是本地测试产物，可作为后续真实抓取测试读取 cookie 的来源，不应提交到仓库。

最终版本中，cookie 和账号密码应加密存储到 SQLite 或用户指定的安全文件中，不能以明文写入示例配置或提交到仓库。

## qBittorrent

`qbittorrent.url` 填写 WebUI 根地址，例如 `http://127.0.0.1:8080`。

`qbittorrent.auth_mode` 支持：

- `uid`：使用 qBittorrent 原生登录接口，提交 `username` 或 `user_id` 与 `password`，服务端获取 `SID` cookie 后调用后续接口。
- `api_key`：不调用登录接口，后续请求携带 `Authorization: Bearer ...`、`X-API-Key`，并在填写用户标识时携带 `X-User-ID`。该模式用于支持代理或扩展认证环境。

`username` 和 `user_id` 当前都可填写；服务会在保存时互相补齐，便于兼容不同命名习惯。

WebUI 的 `设置 / qBittorrent` 页面保存 qBittorrent 设置时，如果密码或 API Key 输入框留空，服务端会保留数据库中已有值。该页面同时控制增量刷新：`auto_sync` 默认启用；连接正常且媒体页位于前台时使用 `sync_interval_seconds=3`，页面隐藏或位于其他页面时使用 `inactive_sync_interval_seconds=30`，连接失败后使用 `disconnected_sync_interval_seconds=60` 重试。URL 为空或关闭自动同步时不轮询。

应用在站点检索后自动下载缺失的 `.torrent` 文件，以 SQLite BLOB 保存并解析 v1/v2 hash。该行为无需新增配置项，固定最多 3 个并发；失败记录会在后续检索中重试。

`qbittorrent.category` 和 `qbittorrent.tags` 是兼容的全局下载默认值；订阅另外保存自己的分类、路径、标签、名称与暂停配置。全局标签会排在订阅标签和种子标签之前合并。qB 分类按完整字符串处理，例如 `PT/ASMR`，应用不会把它拆成 qB 不存在的父子字段；WebUI 的树仅是展示。

分类与标签会同步到 SQLite 快照。qB 离线时 API 可返回最近一次快照并标记 `stale/error`；创建分类时只创建用户提交的完整名称，不隐式创建父节点。自动订阅发送前要求目标分类已经存在，但会自动创建计划中缺少的标签。

应用以文件方式添加种子：发送前按本地 v1 hash 查询，已存在则记录 `exists` 且不修改已有 qB 任务；新版 qB 返回 torrent ID 时直接核对，旧版响应或重复添加冲突时使用本地 hash 查询并回写下载任务。普通列表请求仍只读取数据库；媒体页启用自动同步后通过 qB `sync/maindata` 增量更新匹配快照，显式全量同步仍负责 properties、完成检测和整理任务创建。首版精确验证支持 v1 和带 v1 的 hybrid torrent，纯 v2 torrent 会保存文件但明确拒绝精确发送，不会伪装为成功。

## 筛选规则与自动订阅

主配置中的 `rules[]` 只作为筛选规则的首次初始化来源：启动时仅在 SQLite 不存在同名规则时插入，不会覆盖 WebUI 已经修改的数据库规则。名称是大小写不敏感的唯一标识，规则没有启用开关，所有规则都可被订阅引用。订阅和站点计划不放入主配置，统一通过 WebUI/API 保存到 SQLite；两者都默认关闭。

`rules[]` 支持：

- `site_ids`：限定站点，空数组表示不限。
- `site_categories`：匹配已抓取种子的站点分类；数组内任一值命中即可。
- `site_tags` / `subtitle_tags`：分别匹配站点官方标签和副标题拆分标签；每个数组中指定的值必须全部具备。
- `title_expression`：仅匹配主标题，支持 `!`、`&`、`|`、括号和双引号关键词。
- `promotions`：匹配站点检索定义中的促销选项；数组内任一值命中即可。
- `min_size` / `max_size`：按字节限制大小。
- `min_seeders` / `max_seeders`、`min_leechers` / `max_leechers`、`min_snatches` / `max_snatches`：限制做种、下载和完成数。
- `published_within_minutes`：限制发布时间窗口；种子缺少发布时间时不命中。
- `sort_by`：`source_order`、`published_at`、`size_bytes`、`seeders`、`leechers` 或 `snatches`。
- `sort_direction`：`asc` 或 `desc`。默认 `source_order/asc`，保持最近站点列表从上到下。
- `action`：当前只支持 `download`。

所有数值筛选的 `0` 表示不限。不同字段之间按 AND 匹配，拒绝时 API 返回全部稳定原因代码。使用任一分类、标签或促销条件时必须只配置一个 `site_id`；没有这些条件时仍可覆盖多个站点。草稿预览可以使用尚未保存的规则。

订阅的 `save_path_template`、`qb_tags[]` 和 `filename_template` 只允许：`{{site_id}}`、`{{site_name}}`、`{{torrent_id}}`、`{{category}}`、`{{category_query}}`、`{{rule_name}}`、`{{subscription_name}}`、`{{title}}`、`{{detail_title}}`、`{{subtitle}}`。模板不执行表达式或 LLM，未知占位符会在保存时拒绝。`max_concurrent=0` 和 `daily_limit=0` 表示不限；显式路径会以 `autoTMM=false` 发送，空路径让 qB 使用分类或默认路径策略。

每个站点计划保存 `enabled` 和 `interval_seconds`，默认关闭、默认 900 秒，允许 60 秒至 24 小时并使用整分钟步长。只有常驻服务版和桌面版启动后台调度器，CLI 单次命令不启动后台循环。

## LLM 与媒体库

`llm.base_url` 使用 OpenAI-compatible Chat Completions 地址，可以填写服务根地址，也可以直接填写 `/chat/completions` 完整路径。`llm.api_key` 会以 `Authorization: Bearer ...` 发送。

WebUI 的 `设置 / LLM` 页面保存 LLM 设置时，如果 API Key 输入框留空，服务端会保留数据库中已有值。媒体页的手动下载弹窗会先调用 LLM 提取标题，再由用户确认发送到 qBittorrent。

媒体页的列表/卡片布局、卡片宽度和标题展示策略保存在浏览器 `localStorage`，不新增服务端配置。封面代理复用对应站点已有的 cookie 和 user-agent，不保存第二份凭据。

`media_library.root` 是媒体服务器扫描目录根路径。`media_library.dry_run=true` 时只生成整理结果，不创建硬链接。硬链接要求下载目录和媒体库目录位于同一文件系统；失败时任务会记录错误，不自动复制。

## 站点解析 JSON

正式应用通过 `sites_dir` 加载多个站点 JSON。默认值是相对于数据根目录的 `sites/html`，也可以配置为绝对目录。

规则促销选项复用站点检索定义：对应的 `html.search.fields.selects[]` 设置 `role: "promotion"`，每个 option 使用 `filter_value` 声明和列表页 `promotion_class` 对应的稳定值。聚合选项可使用 `filter_value: "all"`，规则下拉会忽略它；普通种子使用 `normal`。

本地测试可以把站点 JSON 放在 `data/sites/html`。该目录已被 `.gitignore` 忽略，不应随发布提交。

站点 JSON 只使用新格式，顶层字段与 `nexus-media` 风格一致：

- `id`：站点标识。
- `name`：站点名称。
- `domain`：站点基础地址。
- `encoding`：页面编码。
- `html.search.paths[]`：浏览或搜索入口路径。
- `html.search.params`：搜索参数模板。
- `html.search.fields`：搜索框补完字段，由离线或在线 HTML 解析写入。该字段是对象，包含 `checkboxes`、`selects`、`ranges`、`keyword`、`tags`；checkbox 按分组嵌套，例如分类写在 `checkboxes[].name="cat"` 组内，select/tag 带 `options`，range 带 `begin/end`，`tag_id` 带 `exclusive: true`。
- `html.category`：分类配置。
- `html.torrents.list.selector`：种子列表行 selector。
- `html.torrents.fields`：种子列表字段提取规则。解析器会读取 `selector`、`attribute`/`attributes`、`filters`、`remove`、`after` 和 `split` 来提取 ID、标题、分类、链接、副标题、发布时间、体积、做种/下载/完成数与标签；未配置的字段会回退到 NexusPHP 默认结构。

内置通用 NexusPHP 搜索框规则会安装到 `sites/html/common/nexusphp_search.json`。该文件只描述如何从 HTML 识别 checkbox、select、range、keyword 和 `tag_id`，不是站点定义，加载站点时会忽略子目录。

字段过滤器当前支持 `re_search`、`replace`、`querystring` 和 `dateparse`。KamePT 这类列表页中，`tags` 可由副标题文本通过 `after`、`remove`、`split: "whitespace"` 拆出，并用 `max_length` 限制单个 tag 长度；官方彩色 `span` 标签应写入 `tag_ids`，避免和副标题拆词混用。

HTML 解析测试使用同一新格式 fixture：`tests/fixtures/site_parse_config.json`。

解析搜索项后，会把结果补写回站点 JSON，默认在线测试输出为 `data/tests/<site>.updated.json`。


