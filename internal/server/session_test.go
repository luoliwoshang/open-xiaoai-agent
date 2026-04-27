package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSessionRunShell(t *testing.T) {
	t.Parallel()

	session, peer, cleanup := newSessionPair(t)
	defer cleanup()

	done := make(chan struct{})
	go func() {
		defer close(done)

		var msg appMessage
		if err := peer.ReadJSON(&msg); err != nil {
			t.Errorf("ReadJSON() error = %v", err)
			return
		}
		if msg.Request == nil {
			t.Errorf("request = nil, want request message")
			return
		}
		if msg.Request.Command != "run_shell" {
			t.Errorf("command = %q, want %q", msg.Request.Command, "run_shell")
			return
		}

		var script string
		if err := json.Unmarshal(msg.Request.Payload, &script); err != nil {
			t.Errorf("decode payload error = %v", err)
			return
		}
		if script != "echo ok" {
			t.Errorf("script = %q, want %q", script, "echo ok")
			return
		}

		data, err := json.Marshal(CommandResult{
			Stdout:   "ok\n",
			ExitCode: 0,
		})
		if err != nil {
			t.Errorf("json.Marshal() error = %v", err)
			return
		}

		session.onResponse(responseMessage{
			ID:   msg.Request.ID,
			Data: data,
		})
	}()

	result, err := session.RunShell("echo ok", time.Second)
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if result.Stdout != "ok\n" {
		t.Fatalf("stdout = %q, want %q", result.Stdout, "ok\n")
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}

	<-done
}

func TestSessionAbortXiaoAI(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		session, peer, cleanup := newSessionPair(t)
		defer cleanup()

		done := make(chan struct{})
		go func() {
			defer close(done)

			var msg appMessage
			if err := peer.ReadJSON(&msg); err != nil {
				t.Errorf("ReadJSON() error = %v", err)
				return
			}

			var script string
			if err := json.Unmarshal(msg.Request.Payload, &script); err != nil {
				t.Errorf("decode payload error = %v", err)
				return
			}
			want := "/etc/init.d/mico_aivs_lab restart >/dev/null 2>&1"
			if script != want {
				t.Errorf("script = %q, want %q", script, want)
				return
			}

			data, _ := json.Marshal(CommandResult{ExitCode: 0})
			session.onResponse(responseMessage{
				ID:   msg.Request.ID,
				Data: data,
			})
		}()

		if err := session.AbortXiaoAI(time.Second); err != nil {
			t.Fatalf("AbortXiaoAI() error = %v", err)
		}

		<-done
	})

	t.Run("non-zero exit code", func(t *testing.T) {
		t.Parallel()

		session, peer, cleanup := newSessionPair(t)
		defer cleanup()

		done := make(chan struct{})
		go func() {
			defer close(done)

			var msg appMessage
			if err := peer.ReadJSON(&msg); err != nil {
				t.Errorf("ReadJSON() error = %v", err)
				return
			}

			data, _ := json.Marshal(CommandResult{
				ExitCode: 1,
				Stderr:   "boom",
			})
			session.onResponse(responseMessage{
				ID:   msg.Request.ID,
				Data: data,
			})
		}()

		err := session.AbortXiaoAI(time.Second)
		if err == nil {
			t.Fatal("AbortXiaoAI() error = nil, want non-nil")
		}
		if !strings.Contains(err.Error(), "exit_code=1") {
			t.Fatalf("AbortXiaoAI() error = %q, want exit code detail", err)
		}

		<-done
	})
}

func newSessionPair(t *testing.T) (*Session, *websocket.Conn, func()) {
	t.Helper()

	peerCh := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error = %v", err)
			return
		}
		peerCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		httpServer.Close()
		t.Fatalf("Dial() error = %v", err)
	}

	peer := <-peerCh
	session := newSession(conn)
	cleanup := func() {
		_ = conn.Close()
		_ = peer.Close()
		httpServer.Close()
	}

	return session, peer, cleanup
}
