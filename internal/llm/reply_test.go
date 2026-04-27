package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/config"
)

func TestReplyGeneratorStream(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好，\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"我是测试回复。\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	generator := NewReplyGenerator(NewClient(), config.ModelConfig{
		Model:   "reply-model",
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, "你是一个克制的中文语音助手。")

	var chunks []string
	err := generator.Stream(context.Background(), nil, "介绍一下自己", func(delta string) error {
		chunks = append(chunks, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("len(chunks) = %d, want 2", len(chunks))
	}
	if chunks[0] != "你好，" || chunks[1] != "我是测试回复。" {
		t.Fatalf("chunks = %#v", chunks)
	}
}
