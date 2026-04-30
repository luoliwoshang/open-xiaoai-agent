package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var requestSeq atomic.Uint64
var sessionSeq atomic.Uint64

type Session struct {
	conn *websocket.Conn
	id   string

	writeMu   sync.Mutex
	pending   map[string]chan responseMessage
	pendingMu sync.Mutex
}

func newSession(conn *websocket.Conn) *Session {
	return &Session{
		conn:    conn,
		id:      fmt.Sprintf("session-%d", sessionSeq.Add(1)),
		pending: map[string]chan responseMessage{},
	}
}

func (s *Session) HistoryKey() string {
	return s.id
}

func (s *Session) RunShell(script string, timeout time.Duration) (CommandResult, error) {
	resp, err := s.call("run_shell", script, timeout)
	if err != nil {
		return CommandResult{}, err
	}

	var result CommandResult
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		if err := json.Unmarshal(resp.Data, &result); err != nil {
			return CommandResult{}, fmt.Errorf("decode run_shell result: %w", err)
		}
	}

	return result, nil
}

// AbortXiaoAI 通过重启设备侧 mico_aivs_lab 进程来打断原生小爱当前流程。
// 这里的“abort”不是精细化取消某条指令，而是用一次可预期的重启动作
// 尽快停掉原生 ASR/TTS 后续链路，把播放控制权交回当前 Agent 服务。
func (s *Session) AbortXiaoAI(timeout time.Duration) error {
	result, err := s.RunShell("/etc/init.d/mico_aivs_lab restart >/dev/null 2>&1", timeout)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("exit_code=%d stderr=%s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return nil
}

func (s *Session) writeJSON(msg appMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	_ = s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return s.conn.WriteJSON(msg)
}

func (s *Session) onResponse(resp responseMessage) {
	s.pendingMu.Lock()
	ch, ok := s.pending[resp.ID]
	if ok {
		delete(s.pending, resp.ID)
	}
	s.pendingMu.Unlock()

	if ok {
		ch <- resp
	}
}

func (s *Session) call(command string, payload any, timeout time.Duration) (responseMessage, error) {
	id := fmt.Sprintf("req-%d", requestSeq.Add(1))

	var rawPayload json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return responseMessage{}, fmt.Errorf("encode payload: %w", err)
		}
		rawPayload = data
	}

	req := appMessage{
		Request: &requestMessage{
			ID:      id,
			Command: command,
			Payload: rawPayload,
		},
	}

	ch := make(chan responseMessage, 1)
	s.pendingMu.Lock()
	s.pending[id] = ch
	s.pendingMu.Unlock()

	if err := s.writeJSON(req); err != nil {
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
		return responseMessage{}, err
	}

	select {
	case resp := <-ch:
		if resp.Code != nil && *resp.Code != 0 {
			return resp, fmt.Errorf("code=%d msg=%s", *resp.Code, resp.Msg)
		}
		return resp, nil
	case <-time.After(timeout):
		s.pendingMu.Lock()
		delete(s.pending, id)
		s.pendingMu.Unlock()
		return responseMessage{}, fmt.Errorf("request timeout: %s", command)
	}
}
