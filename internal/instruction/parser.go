package instruction

import (
	"encoding/json"
	"fmt"
	"strings"
)

type fileMonitorEvent struct {
	NewLine string `json:"NewLine,omitempty"`
}

type logLine struct {
	Header struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"header"`
	Payload struct {
		IsFinal bool `json:"is_final"`
		Results []struct {
			Text string `json:"text"`
		} `json:"results"`
	} `json:"payload"`
}

// FinalASRText returns the final ASR text carried by an instruction event.
// It ignores non-ASR events and file rollover notifications.
func FinalASRText(data json.RawMessage) (string, error) {
	line, err := parseLine(data)
	if err != nil || line == "" {
		return "", err
	}

	var msg logLine
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", fmt.Errorf("decode instruction log: %w", err)
	}

	if msg.Header.Namespace != "SpeechRecognizer" || msg.Header.Name != "RecognizeResult" {
		return "", nil
	}
	if !msg.Payload.IsFinal || len(msg.Payload.Results) == 0 {
		return "", nil
	}

	return strings.TrimSpace(msg.Payload.Results[0].Text), nil
}

func parseLine(data json.RawMessage) (string, error) {
	if len(data) == 0 || string(data) == "null" {
		return "", nil
	}

	var newFile string
	if err := json.Unmarshal(data, &newFile); err == nil {
		if newFile == "NewFile" {
			return "", nil
		}
	}

	var event fileMonitorEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return "", fmt.Errorf("decode file monitor event: %w", err)
	}

	return event.NewLine, nil
}
