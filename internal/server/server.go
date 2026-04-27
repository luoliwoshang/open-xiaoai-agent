package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/luoliwoshang/open-xiaoai-agent/internal/instruction"
)

type Config struct {
	Addr  string
	Debug bool
}

type ASRHandler func(session *Session, text string)

type Server struct {
	config   Config
	onASR    ASRHandler
	upgrader websocket.Upgrader
}

func New(config Config, onASR ASRHandler) *Server {
	return &Server{
		config: config,
		onASR:  onASR,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)
	log.Printf("listening on %s", s.config.Addr)
	return http.ListenAndServe(s.config.Addr, mux)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	session := newSession(conn)
	log.Printf("client connected: %s", r.RemoteAddr)
	defer log.Printf("client disconnected: %s", r.RemoteAddr)

	conn.SetReadLimit(16 << 20)
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	done := make(chan struct{})
	go s.keepAlive(conn, done)
	defer close(done)

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) &&
				!strings.Contains(err.Error(), "use of closed network connection") {
				log.Printf("read failed: %v", err)
			}
			return
		}

		switch messageType {
		case websocket.TextMessage:
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			if err := s.handleTextMessage(session, payload); err != nil {
				log.Printf("invalid text message: %v", err)
			}
		case websocket.BinaryMessage:
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			if s.config.Debug {
				log.Printf("received binary stream: %d bytes", len(payload))
			}
		}
	}
}

func (s *Server) keepAlive(conn *websocket.Conn, done <-chan struct{}) {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := conn.WriteControl(
				websocket.PingMessage,
				[]byte("ping"),
				time.Now().Add(5*time.Second),
			); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleTextMessage(session *Session, payload []byte) error {
	var msg appMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	switch {
	case msg.Event != nil:
		return s.handleEvent(session, *msg.Event)
	case msg.Request != nil:
		if s.config.Debug {
			log.Printf("received unsupported request: command=%s id=%s", msg.Request.Command, msg.Request.ID)
		}
	case msg.Response != nil:
		session.onResponse(*msg.Response)
		if s.config.Debug {
			log.Printf("received response: id=%s code=%v", msg.Response.ID, msg.Response.Code)
		}
	default:
		if s.config.Debug {
			log.Printf("received unknown text message: %s", string(payload))
		}
	}

	return nil
}

func (s *Server) handleEvent(session *Session, event eventMessage) error {
	if s.config.Debug {
		log.Printf("event=%s payload=%s", event.Event, string(event.Data))
	}
	if event.Event != "instruction" {
		return nil
	}

	text, err := instruction.FinalASRText(event.Data)
	if err != nil {
		return err
	}
	if text == "" || s.onASR == nil {
		return nil
	}

	s.onASR(session, text)
	return nil
}
