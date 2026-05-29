package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHistorySaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	h := History{Entries: []HistoryEntry{
		{
			Timestamp: time.Now(),
			Name:      "test-app",
			Source:    SourceAPT,
			Mode:      modeComplete,
			Success:   true,
		},
	}}

	if err := SaveHistory(h); err != nil {
		t.Fatalf("Failed to save history: %v", err)
	}

	loaded, err := LoadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	if len(loaded.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(loaded.Entries))
	}
	if loaded.Entries[0].Name != "test-app" {
		t.Errorf("Expected name 'test-app', got %s", loaded.Entries[0].Name)
	}
}

func TestAddEntry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	entry := HistoryEntry{
		Timestamp: time.Now(),
		Name:      "added-app",
		Source:    SourceSnap,
		Mode:      modeNormal,
		Success:   true,
		Duration:  5 * time.Second,
	}

	if err := AddEntry(entry); err != nil {
		t.Fatalf("Failed to add entry: %v", err)
	}

	h, err := LoadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	if len(h.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(h.Entries))
	}
}

func TestHistoryMaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	for i := 0; i < 110; i++ {
		entry := HistoryEntry{
			Timestamp: time.Now(),
			Name:      "app",
			Source:    SourceAPT,
			Mode:      modeNormal,
			Success:   true,
		}
		if err := AddEntry(entry); err != nil {
			t.Fatalf("Failed to add entry %d: %v", i, err)
		}
	}

	h, err := LoadHistory()
	if err != nil {
		t.Fatalf("Failed to load history: %v", err)
	}

	if len(h.Entries) > 100 {
		t.Errorf("Expected max 100 entries, got %d", len(h.Entries))
	}
}

func TestLoadHistoryNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "nonexistent"))

	h, err := LoadHistory()
	if err != nil {
		t.Fatalf("Expected no error for non-existent history file, got: %v", err)
	}

	if len(h.Entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(h.Entries))
	}
}
