# 站点抓取与解析

## 边界

- 站点定义来自 `sites_dir`；内置定义只在目标文件不存在时安装。
- `parser` 只处理 JSON 和 HTML，不发起请求、不访问 SQLite。
- 所有生产站点请求由 core 统一读取凭据并经过 Cookie 策略，不能从业务代码绕过。

## 关系

```text
站点 JSON -> parser definitions
SQLite 凭据 -> requestpolicy -> fetcher -> parser -> core -> SQLite
```

| 模块 | 作用 |
| --- | --- |
| `internal/parser` | 加载站点定义，解析搜索表单、种子列表和详情页。 |
| `internal/requestpolicy` | 按目标域名决定允许发送的 Cookie 名称。 |
| `internal/fetcher` | 构造请求、处理重定向并返回响应正文。 |
| `internal/core/site_requests.go` | 把站点、凭据、策略和 fetcher 组合为统一请求入口。 |
| `internal/core/site_attendance.go` | 校验并保存每日签到配置，使用统一请求入口执行签到。 |
| `internal/core/site_fetch.go` | 持久化扫描任务、逐页抓取、增量边界和订阅触发。 |
| `internal/core/torrent_files.go` | 下载 torrent，解析元数据并交给 storage。 |
| `internal/storage` | 保存凭据、种子元数据、扫描任务和 torrent 文件索引；原文件写入数据库同目录的 `torrents/`。 |

## 列表扫描

首页、手动、CLI 和周期计划复用同一流程。每页解析后批量写入 SQLite，未变化记录不执行 UPSERT；随后消费该站点的持久化订阅队列并执行已启用订阅，再请求下一页。订阅或 qB 错误只记录到任务，不回滚已完成页面。只有首次入库的种子主动补抓 `.torrent`；相同载荷按 SHA-256 复用同一文件，实际读取发现文件缺失时按需重新下载。

`incremental` 和 `pages` 都受全局 `max_pages` 限制，默认 3、范围 1–100。增量扫描在连续遇到 5 条既有普通种子时停止；`sticky_level > 0` 的置顶种子正常入库但不参与边界判断。第一页成功解析和写入后会替换该站点的内存置顶快照，置顶等级不写 SQLite。跨页种子按联合键去重，`source_order` 在整个任务内连续。

扫描任务持久化到 `site_fetch_jobs`。单个 NexusBridge 进程内同一站点只运行一个任务，API、CLI 和周期入口的重复触发都会复用活动任务；进程启动时遗留的活动任务会标为中断，每站点保留最近 100 条结束记录。

## 自动签到

站点 JSON 的 `attendance_page_url` 声明签到页面，缺失时使用 `attendance.php`。该地址必须相对站点或与站点同源。每日计划保存到 `site_attendance_schedules`，以 `HH:mm` 和 IANA 时区计算下次执行时间；到期后使用站点现有 Cookie 发起 GET，并与列表扫描共用站点锁。HTTP 2xx 视为完成，失败状态保留到下一次执行，响应正文不持久化。

## 站点定义

站点 JSON 顶层包含 `id`、`name`、`domain`、可选 `attendance_page_url`、`request_rules` 和 `html`。`html.search.fields.page` 使用 `type: "number"` 声明数字分页，`name` 是参数名，`query` 是包含 `{{value}}` 的查询模板；`html.browse.start` 可声明起始页，缺失时为 0。网页分页链接用于判断是否还有下一页，实际数字分页 URL 从列表入口按该声明构造。`html.search` 的其他字段描述搜索路径与筛选项，`html.torrents` 描述列表和字段提取。

`request_rules` 只允许把指定 Cookie 发送给匹配的域名后缀；重定向后必须重新决策。日志可记录 Cookie 名称，不得记录值。

## 不变量

- 跨主机默认不发送 Cookie，规则匹配使用完整点分隔域名后缀。
- 站点响应必须先解析为领域模型，再由 core 持久化。
- torrent 下载结果必须通过 bencode 和元数据解析后才能保存或发送给 qB。
- 详情补充、封面和恢复关键词搜索不属于列表扫描，不触发自动订阅。
