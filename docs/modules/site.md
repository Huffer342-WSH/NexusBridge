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
| `internal/core/torrent_files.go` | 下载 torrent，解析元数据并交给 storage。 |
| `internal/storage` | 保存凭据、种子元数据和 torrent BLOB。 |

## 站点定义

站点 JSON 顶层包含 `id`、`name`、`domain`、可选 `request_rules` 和 `html`。`html.search` 描述请求路径与搜索字段，`html.torrents` 描述列表和字段提取，详情解析使用同一字段规则体系。当前格式以 `internal/parser/types.go` 和 `internal/builtin/sites/` 为准。

`request_rules` 只允许把指定 Cookie 发送给匹配的域名后缀；重定向后必须重新决策。日志可记录 Cookie 名称，不得记录值。

## 不变量

- 跨主机默认不发送 Cookie，规则匹配使用完整点分隔域名后缀。
- 站点响应必须先解析为领域模型，再由 core 持久化。
- torrent 下载结果必须通过 bencode 和元数据解析后才能保存或发送给 qB。
