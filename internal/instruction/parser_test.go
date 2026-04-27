package instruction_test

import (
	"encoding/json"
	"testing"

	"github.com/luoliwoshang/open-xiaoai-agent/internal/instruction"
)

func TestFinalASRText_ReturnsFinalRecognizeResult(t *testing.T) {
	t.Parallel()

	data := mustRawJSON(t, map[string]any{
		"NewLine": `{"header":{"namespace":"SpeechRecognizer","name":"RecognizeResult"},"payload":{"is_final":true,"results":[{"text":"  今天天气怎么样  "}]}}`,
	})

	text, err := instruction.FinalASRText(data)
	if err != nil {
		t.Fatalf("FinalASRText() error = %v", err)
	}
	if text != "今天天气怎么样" {
		t.Fatalf("FinalASRText() = %q, want %q", text, "今天天气怎么样")
	}
}

func TestFinalASRText_IgnoresNonASRPayloads(t *testing.T) {
	t.Parallel()

	tests := map[string]json.RawMessage{
		"new file marker": mustRawJSON(t, "NewFile"),
		"non final result": mustRawJSON(t, map[string]any{
			"NewLine": `{"header":{"namespace":"SpeechRecognizer","name":"RecognizeResult"},"payload":{"is_final":false,"results":[{"text":"你好"}]}}`,
		}),
		"other namespace": mustRawJSON(t, map[string]any{
			"NewLine": `{"header":{"namespace":"SpeechSynthesizer","name":"Speak"},"payload":{"is_final":true,"results":[{"text":"你好"}]}}`,
		}),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			text, err := instruction.FinalASRText(data)
			if err != nil {
				t.Fatalf("FinalASRText() error = %v", err)
			}
			if text != "" {
				t.Fatalf("FinalASRText() = %q, want empty", text)
			}
		})
	}
}

func TestFinalASRText_ReturnsErrorOnInvalidPayload(t *testing.T) {
	t.Parallel()

	data := mustRawJSON(t, map[string]any{
		"NewLine": "{invalid-json",
	})

	_, err := instruction.FinalASRText(data)
	if err == nil {
		t.Fatal("FinalASRText() error = nil, want non-nil")
	}
}

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
