package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HistoryEntry represents a single uninstall operation record.
type HistoryEntry struct {
	Timestamp time.Time     `json:"timestamp"`
	Name      string        `json:"name"`
	Source    string        `json:"source"`
	Mode      string        `json:"mode"`
	Success   bool          `json:"success"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
}

// History holds the list of uninstall records.
type History struct {
	Entries []HistoryEntry `json:"entries"`
}

// historyPath returns the path to the history file.
func historyPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(configDir, "appurge")
	return filepath.Join(appDir, "history.json"), nil
}

// ensureHistoryDir creates the history directory if it doesn't exist.
func ensureHistoryDir() error {
	path, err := historyPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}

// LoadHistory reads the history file and returns the History struct.
func LoadHistory() (History, error) {
	path, err := historyPath()
	if err != nil {
		return History{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return History{Entries: []HistoryEntry{}}, nil
		}
		return History{}, err
	}

	var h History
	if err := json.Unmarshal(data, &h); err != nil {
		return History{}, err
	}
	return h, nil
}

// SaveHistory writes the History struct to the history file.
func SaveHistory(h History) error {
	if err := ensureHistoryDir(); err != nil {
		return err
	}
	path, err := historyPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("create history temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set history permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write history: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync history: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close history: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace history: %w", err)
	}
	return nil
}

// AddEntry appends a new entry to the history and saves it.
func AddEntry(entry HistoryEntry) error {
	h, err := LoadHistory()
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	h.Entries = append(h.Entries, entry)

	// Keep only last 100 entries
	if len(h.Entries) > 100 {
		h.Entries = h.Entries[len(h.Entries)-100:]
	}

	return SaveHistory(h)
}
