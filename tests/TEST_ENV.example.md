# 后端真实测试环境变量示例

复制为本地 `tests/TEST_ENV.md` 后填写。该本地文件已被忽略，`go test` 不会自动读取其中内容。

```env
NEXUSBRIDGE_TEST_BASE_URL="https://example.invalid/"
NEXUSBRIDGE_TEST_CONFIG="data/config.example.json"
NEXUSBRIDGE_TEST_SITE_ID="example"
NEXUSBRIDGE_TEST_CURL_FILE="path/to/local/fetch.sh"
NEXUSBRIDGE_TEST_QB_URL="http://127.0.0.1:8080/"
NEXUSBRIDGE_TEST_QB_API_KEY=""
NEXUSBRIDGE_TEST_QB_TORRENT_URL=""
NEXUSBRIDGE_TEST_LLM_BASE_URL=""
NEXUSBRIDGE_TEST_LLM_API_KEY=""
NEXUSBRIDGE_TEST_LLM_MODEL=""
```

PowerShell 中需要逐项设置，例如：

```powershell
$env:NEXUSBRIDGE_TEST_CONFIG = "data/config.example.json"
go test ./...
```
