package server

import (
	"encoding/json"
	"testing"
)

func TestNew_ConfiguresServer(t *testing.T) {
	t.Parallel()

	called := false
	handler := func(_ *Session, _ string) { called = true }
	s := New(Config{Addr: ":4399", Debug: true}, handler)

	if s == nil {
		t.Fatal("New() = nil, want non-nil server")
	}
	if s.config.Addr != ":4399" {
		t.Fatalf("server config addr = %q, want %q", s.config.Addr, ":4399")
	}
	if !s.config.Debug {
		t.Fatal("server config debug = false, want true")
	}
	if s.onASR == nil {
		t.Fatal("server onASR = nil, want handler")
	}

	_ = called
}

func TestHandleEvent_InvokesASRHandlerOnFinalResult(t *testing.T) {
	t.Parallel()

	var got string
	s := New(Config{}, func(_ *Session, text string) {
		got = text
	})

	err := s.handleEvent(&Session{}, eventMessage{
		Event: "instruction",
		Data: mustRawJSON(t, map[string]any{
			"NewLine": `{"header":{"namespace":"SpeechRecognizer","name":"RecognizeResult"},"payload":{"is_final":true,"results":[{"text":"打开客厅空调"}]}}`,
		}),
	})
	if err != nil {
		t.Fatalf("handleEvent() error = %v", err)
	}
	if got != "打开客厅空调" {
		t.Fatalf("handler text = %q, want %q", got, "打开客厅空调")
	}
}

func TestHandleEvent_IgnoresIrrelevantMessages(t *testing.T) {
	t.Parallel()

	called := false
	s := New(Config{}, func(_ *Session, _ string) {
		called = true
	})

	tests := []eventMessage{
		{Event: "playing", Data: mustRawJSON(t, "Playing")},
		{Event: "instruction", Data: mustRawJSON(t, "NewFile")},
		{Event: "instruction", Data: mustRawJSON(t, map[string]any{
			"NewLine": `{"header":{"namespace":"SpeechRecognizer","name":"RecognizeResult"},"payload":{"is_final":false,"results":[{"text":"你好"}]}}`,
		})},
	}

	for _, event := range tests {
		if err := s.handleEvent(&Session{}, event); err != nil {
			t.Fatalf("handleEvent() error = %v", err)
		}
	}

	if called {
		t.Fatal("handler was called for irrelevant event")
	}
}

func TestConnectionStatusTracksConnectLifecycle(t *testing.T) {
	t.Parallel()

	s := New(Config{}, nil)
	s.markConnected("192.168.1.10:12345")
	s.markConnected("192.168.1.11:12345")

	status := s.ConnectionStatus()
	if !status.Connected {
		t.Fatal("Connected = false, want true")
	}
	if status.ActiveSessions != 2 {
		t.Fatalf("ActiveSessions = %d, want 2", status.ActiveSessions)
	}
	if status.LastRemoteAddr != "192.168.1.11:12345" {
		t.Fatalf("LastRemoteAddr = %q, want latest remote addr", status.LastRemoteAddr)
	}
	if status.LastConnectedAt.IsZero() {
		t.Fatal("LastConnectedAt is zero")
	}

	s.markDisconnected("192.168.1.11:12345")
	status = s.ConnectionStatus()
	if !status.Connected {
		t.Fatal("Connected = false after one disconnect, want true")
	}
	if status.ActiveSessions != 1 {
		t.Fatalf("ActiveSessions = %d, want 1", status.ActiveSessions)
	}

	s.markDisconnected("192.168.1.10:12345")
	status = s.ConnectionStatus()
	if status.Connected {
		t.Fatal("Connected = true, want false")
	}
	if status.ActiveSessions != 0 {
		t.Fatalf("ActiveSessions = %d, want 0", status.ActiveSessions)
	}
	if status.LastDisconnectedAt.IsZero() {
		t.Fatal("LastDisconnectedAt is zero")
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
