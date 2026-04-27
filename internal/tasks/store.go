package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() (fileState, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fileState{}, fmt.Errorf("create task data dir: %w", err)
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileState{Version: 1}, nil
		}
		return fileState{}, fmt.Errorf("read task store: %w", err)
	}
	if len(data) == 0 {
		return fileState{Version: 1}, nil
	}

	var state fileState
	if err := json.Unmarshal(data, &state); err != nil {
		return fileState{}, fmt.Errorf("decode task store: %w", err)
	}
	if state.Version == 0 {
		state.Version = 1
	}
	return state, nil
}

func (s *Store) Save(state fileState) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create task data dir: %w", err)
	}

	state.Version = 1
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task store: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write task temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace task store: %w", err)
	}
	return nil
}
