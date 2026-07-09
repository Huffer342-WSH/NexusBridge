package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

type Client struct {
	cfg    Config
	client *http.Client
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OrganizeSuggestion struct {
	MediaType   string  `json:"media_type"`
	RelativeDir string  `json:"relative_dir"`
	Filename    string  `json:"filename"`
	Confidence  float64 `json:"confidence"`
}

type CosplayVideoInput struct {
	Title       string
	Subtitle    string
	Description string
}

type CosplayVideoInfo struct {
	Circle         string  `json:"circle"`
	Author         string  `json:"author"`
	Coser          string  `json:"coser"`
	Character      string  `json:"character"`
	SourceWork     string  `json:"source_work"`
	FormattedTitle string  `json:"formatted_title"`
	Confidence     float64 `json:"confidence"`
	Notes          string  `json:"notes"`
}

func New(cfg Config) *Client {
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Chat 发送基础聊天请求并返回首个回答。
func (c *Client) Chat(ctx context.Context, messages []ChatMessage, temperature float64) (string, error) {
	mapped := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		mapped = append(mapped, map[string]string{
			"role":    message.Role,
			"content": message.Content,
		})
	}
	return c.chat(ctx, mapped, temperature)
}

// FormatTorrentTitle 使用 LLM 从种子标题中提取可读标题。
func (c *Client) FormatTorrentTitle(ctx context.Context, title string) (string, string, error) {
	if strings.TrimSpace(title) == "" {
		return "", "", fmt.Errorf("title is required")
	}
	prompt := `你是 PT 种子标题整理助手。根据用户给出的原始标题输出一个适合展示和下载任务识别的中文标题，不要输出额外解释。
格式建议: [作者][演员（可选）][角色（可选）]中文翻译标题
要求:
1. 自然翻译标题中的关键信息，保留番号、版本、分辨率、字幕、合集等重要标识。
2. 不要换行，不要 Markdown，不要 JSON。
3. 如果无法判断，返回清理后的原始标题。`
	raw, err := c.chat(ctx, []map[string]string{
		{"role": "system", "content": prompt},
		{"role": "user", "content": title},
	}, 0.2)
	if err != nil {
		return "", "", err
	}
	formatted := strings.TrimSpace(raw)
	formatted = strings.ReplaceAll(formatted, "\r", " ")
	formatted = strings.ReplaceAll(formatted, "\n", " ")
	formatted = strings.Join(strings.Fields(formatted), " ")
	if formatted == "" {
		return "", raw, fmt.Errorf("llm returned empty title")
	}
	return formatted, raw, nil
}

// ExtractCosplayVideo 提取 cosplay 视频作品结构化信息。
func (c *Client) ExtractCosplayVideo(ctx context.Context, input CosplayVideoInput) (CosplayVideoInfo, string, error) {
	if strings.TrimSpace(input.Title) == "" {
		return CosplayVideoInfo{}, "", fmt.Errorf("title is required")
	}
	prompt := `你是 cosplay 视频作品信息提取 agent。用户会提供种子标题、副标题和详情描述。
请只输出 JSON，不要输出额外说明。JSON 字段固定为：
{"circle":"","author":"","coser":"","character":"","source_work":"","formatted_title":"","confidence":0.0,"notes":""}
要求：
1. circle 表示社团、店铺、发布组织；author 表示作者或发布者；无法区分时优先放入 circle。
2. coser 表示演员、模特、COSER。
3. character 表示 cosplay 角色；source_work 表示角色所属作品。
4. formatted_title 使用适合文件名和下载任务展示的中文标题，格式优先为 [社团][coser][角色]中文标题。
5. 不要编造没有依据的信息；不确定的字段留空，并在 notes 简短说明。`
	user := fmt.Sprintf("标题: %s\n副标题: %s\n描述: %s", input.Title, input.Subtitle, input.Description)
	raw, err := c.Chat(ctx, []ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: user},
	}, 0.1)
	if err != nil {
		return CosplayVideoInfo{}, "", err
	}
	content := stripJSONFence(raw)
	var info CosplayVideoInfo
	if err := json.Unmarshal([]byte(content), &info); err != nil {
		return CosplayVideoInfo{}, raw, fmt.Errorf("parse cosplay video info: %w", err)
	}
	if strings.TrimSpace(info.FormattedTitle) == "" {
		return CosplayVideoInfo{}, raw, fmt.Errorf("llm returned empty formatted_title")
	}
	return info, raw, nil
}

// SuggestOrganization 生成媒体库整理建议。
func (c *Client) SuggestOrganization(ctx context.Context, title, sourcePath string) (OrganizeSuggestion, string, error) {
	prompt := `你是媒体库文件整理助手。根据标题和源路径输出 JSON，不要输出额外文字。
JSON 格式: {"media_type":"movie|tv|anime|other","relative_dir":"相对目录","filename":"文件名含扩展名","confidence":0.0到1.0}
要求 relative_dir 必须是相对路径，不要使用 ..。`
	content, err := c.chat(ctx, []map[string]string{
		{"role": "system", "content": prompt},
		{"role": "user", "content": fmt.Sprintf("标题: %s\n源路径: %s", title, sourcePath)},
	}, 0.1)
	if err != nil {
		return OrganizeSuggestion{}, "", err
	}
	content = stripJSONFence(content)
	var suggestion OrganizeSuggestion
	if err := json.Unmarshal([]byte(content), &suggestion); err != nil {
		return OrganizeSuggestion{}, content, fmt.Errorf("parse llm suggestion: %w", err)
	}
	return suggestion, content, nil
}

// chat 发送 OpenAI-compatible Chat Completions 请求。
func (c *Client) chat(ctx context.Context, messages []map[string]string, temperature float64) (string, error) {
	if strings.TrimSpace(c.cfg.BaseURL) == "" || strings.TrimSpace(c.cfg.Model) == "" {
		return "", fmt.Errorf("llm base_url and model are required")
	}
	request := map[string]any{
		"model":       c.cfg.Model,
		"messages":    messages,
		"stream":      false,
		"temperature": temperature,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/")
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("llm status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}

// stripJSONFence 去掉 LLM 返回的 JSON 代码块包裹。
func stripJSONFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```JSON")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSpace(content)
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}
