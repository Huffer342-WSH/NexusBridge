package tests

import (
	"strings"
	"testing"

	"nexusbridge/internal/llm"
)

func TestLLMRealChat(t *testing.T) {
	client := realLLMClient(t)
	got, err := client.Chat(t.Context(), []llm.ChatMessage{
		{Role: "system", Content: "你是一个用于集成测试的简洁助手。"},
		{Role: "user", Content: "回复 pong，只输出这个单词。"},
	}, 0)
	if err != nil {
		t.Fatalf("real llm chat: %v", err)
	}
	if strings.TrimSpace(got) == "" {
		t.Fatalf("expected non-empty llm response")
	}
}

func TestLLMRealExtractCosplayVideo(t *testing.T) {
	client := realLLMClient(t)
	info, raw, err := client.ExtractCosplayVideo(t.Context(), llm.CosplayVideoInput{
		Title:       "[社团A] 测试标题 1080p",
		Subtitle:    "CoserC 角色D cosplay",
		Description: "角色D 来自 作品E。请根据输入提取结构化字段。",
	})
	if err != nil {
		t.Fatalf("real llm extract cosplay video: %v; raw=%s", err, raw)
	}
	if strings.TrimSpace(raw) == "" {
		t.Fatalf("expected raw llm response")
	}
	if strings.TrimSpace(info.FormattedTitle) == "" {
		t.Fatalf("expected formatted title, got %#v", info)
	}
}

func realLLMClient(t *testing.T) *llm.Client {
	t.Helper()
	return llm.New(llm.Config{
		BaseURL: requiredEnv(t, "NEXUSBRIDGE_TEST_LLM_BASE_URL"),
		APIKey:  requiredEnv(t, "NEXUSBRIDGE_TEST_LLM_API_KEY"),
		Model:   requiredEnv(t, "NEXUSBRIDGE_TEST_LLM_MODEL"),
	})
}
