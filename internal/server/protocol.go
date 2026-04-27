package server

import "encoding/json"

type appMessage struct {
	Event    *eventMessage    `json:"Event,omitempty"`
	Request  *requestMessage  `json:"Request,omitempty"`
	Response *responseMessage `json:"Response,omitempty"`
}

type eventMessage struct {
	ID    string          `json:"id"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type requestMessage struct {
	ID      string          `json:"id"`
	Command string          `json:"command"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type responseMessage struct {
	ID   string          `json:"id"`
	Code *int            `json:"code,omitempty"`
	Msg  string          `json:"msg,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type CommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}
