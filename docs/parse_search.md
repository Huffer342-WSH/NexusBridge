# HTML 解析配置

NexusBridge 的 HTML 解析分两层：

- 搜索项解析：使用 NexusPHP 通用搜索框规则识别页面上的参数，再补写到站点 JSON。
- 种子列表解析：使用站点 JSON 中的 `html.torrents.fields` 解析每一行种子信息。

测试顺序固定为先离线 HTML，再在线抓取。离线测试输入是不完整站点 JSON 和本地 HTML，输出是补完后的站点 JSON。

## 通用搜索框规则

通用规则随内置站点一起安装到 `sites/html/common/nexusphp_search.json`。该文件不是站点定义，不会被当作站点加载；它描述 NexusPHP 搜索框常见结构：

```json
{
  "id": "nexusphp-search",
  "form_selector": "form[name=\"searchbox\"]",
  "fields": {
    "checkbox": {
      "selector": "input[type=\"checkbox\"][name]",
      "type": "bool",
      "query": "{{name}}={{value}}"
    },
    "select": {
      "selector": "select[name]",
      "type": "select",
      "query": "{{name}}={{value}}"
    },
    "range": {
      "begin_suffix": "_begin",
      "end_suffix": "_end",
      "number_type": "number_range",
      "date_type": "date_range"
    },
    "keyword": {
      "selector": "input[name=\"search\"]",
      "type": "string",
      "query": "search={{value}}"
    },
    "tag": {
      "selector": "a[href*=\"tag_id=\"]",
      "type": "tag",
      "exclusive": true,
      "query": "tag_id={{value}}"
    }
  }
}
```

当前 Go 解析器按这份规则的语义实现识别逻辑：checkbox 从相邻链接、label 或图片 `alt/title` 取标题；select 从上方标题行取标题；range 识别 `_begin/_end` 输入对；`tag_id` 作为独占字段。

## 站点搜索字段输出

解析搜索框后，`UpdateSiteDefinitionSearchOptions` 会把字段写入站点 JSON 的 `html.search.fields`：

```json
{
  "html": {
    "search": {
      "paths": [{ "path": "torrents.php", "method": "get" }],
      "params": {
        "search": "{keyword}",
        "notnewword": 1
      },
      "fields": {
        "checkboxes": [
          {
            "name": "cat",
            "type": "bool",
            "label": "分类",
            "options": [
              { "name": "cat410", "value": "1", "label": "同人AV", "query": "cat410=1" },
              { "name": "cat420", "value": "1", "label": "外语音声", "query": "cat420=1" }
            ]
          },
          {
            "name": "source",
            "type": "bool",
            "label": "马赛克",
            "options": [
              { "name": "source1", "value": "1", "label": "全身无码", "query": "source1=1" }
            ]
          }
        ],
        "selects": [
          {
            "name": "spstate",
            "type": "select",
            "label": "显示促销种子",
            "query": "spstate={{value}}",
            "options": [
              { "value": "0", "label": "全部", "query": "spstate=0" },
              { "value": "2", "label": "免费", "query": "spstate=2" }
            ]
          }
        ],
        "ranges": [
          {
            "name": "size",
            "type": "number_range",
            "label": "体积范围(GB)",
            "begin": "size_begin",
            "end": "size_end",
            "query": "size_begin={{begin}}&size_end={{end}}"
          },
          {
            "name": "added",
            "type": "date_range",
            "label": "发布时间范围",
            "begin": "added_begin",
            "end": "added_end",
            "query": "added_begin={{begin}}&added_end={{end}}"
          }
        ],
        "keyword": {
          "name": "search",
          "type": "string",
          "label": "搜索关键字",
          "query": "search={{value}}"
        },
        "tags": {
          "name": "tag_id",
          "type": "tag",
          "label": "标签",
          "exclusive": true,
          "query": "tag_id={{value}}",
          "options": [
            { "value": "8", "label": "中文字幕", "query": "tag_id=8" }
          ]
        }
      }
    }
  }
}
```

字段含义：

- `checkboxes[]`：checkbox 分组；分类在 `name: "cat"` 的分组内，不再在每个元素里重复写 `group`。
- `name`：参数标识符；checkbox 分组使用前缀，例如 `cat`，range 使用公共前缀，例如 `size`。
- `type`：参数类型，支持 `bool`、`select`、`number_range`、`date_range`、`string`、`tag`。
- `label`：展示标题，从 HTML 文本、链接或图片属性推导。
- `query`：附加到 URL 上的参数模板或已确定参数。
- `begin` / `end`：range 参数的起止输入名。
- `options`：checkbox、select 和 tag 的可选项；checkbox option 额外保存自己的 `name`。
- `exclusive`：独占搜索参数；`tag_id` 必须为 `true`，后续组 URL 时不能和其他字段组合。

## 种子列表字段

种子列表字段由站点 JSON 的 `html.torrents.fields` 提供。解析器读取以下规则能力：

- `selector`：在每个种子行内匹配节点。
- `attribute` / `attributes`：从属性取值，多个属性按顺序取第一个非空值。
- `filters`：支持 `re_search`、`replace`、`querystring`、`dateparse`。
- `remove`：取文本前移除不参与解析的子节点。
- `after`：只读取某个节点之后的文本。
- `split`：把字段文本拆为数组，当前用于 tag。

KamePT 的列表页 tag 位于标题区域第一个 `<br>` 之后，因此配置为：

```json
{
  "tags": {
    "selector": "table.torrentname td.embedded:nth-child(2)",
    "after": "br",
    "remove": ["a", "img", "font", "span", "div"],
    "split": "whitespace"
  }
}
```

## URL 参数组合规则

站点 JSON 补完后，后续搜索 URL 由 `html.search.paths[0].path` 和 `html.search.fields` 中各分组字段的 `query` 组成：

```text
torrents.php?cat410=1&spstate=2&size_begin=1&size_end=2&search=keyword
```

`tag_id` 是独占参数，单独生成：

```text
torrents.php?tag_id=8
```

## 测试输入输出

离线搜索解析测试：

- 输入：`tests/fixtures/site_parse_config.json`，缺少 `html.search.fields`。
- 输入：`tests/fixtures/torrents_page.html`。
- 输出：测试临时目录中的 `site.completed.json`。
- 验证：重新加载输出 JSON，确认 `checkboxes.cat.options.cat410`、`incldead`、`size`、`added`、`search`、`tag_id` 已补齐。

在线搜索解析测试：

- 输入：环境变量指定的站点配置、cookie DB 和真实抓取 HTML。
- 输出：`data/tests/<site>.updated.json`。
- 验证：同样检查 `html.search.fields`，并保留搜索项快照日志。
