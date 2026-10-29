package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected string
	}{
		{"zero bytes", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1024 * 1024, "1.0 MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0 GB"},
		{"mixed", 1536, "1.5 KB"},
		{"large", 5 * 1024 * 1024 * 1024, "5.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.input)
			if got != tt.expected {
				t.Errorf("FormatBytes(%d) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDryRunUnknownSource(t *testing.T) {
	pkg := PackageResult{Name: "test", Source: "Unknown"}
	_, err := DryRun(pkg, modeNormal)
	if err == nil {
		t.Error("Expected error for unknown source")
	}
}

func TestDryRunAPT(t *testing.T) {
	pkg := PackageResult{Name: "test-pkg", Source: SourceAPT}
	result, err := DryRun(pkg, modeNormal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
}

func TestDryRunAPTComplete(t *testing.T) {
	pkg := PackageResult{Name: "test-pkg", Source: SourceAPT}
	result, err := DryRun(pkg, modeComplete)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
	// DirsToRemove only lists directories that already exist under ~/.config
	// or ~/.local/share — for a fake package name that is usually empty.
}

func TestDryRunFlatpak(t *testing.T) {
	pkg := PackageResult{Name: "test-app", ID: "com.test.App", Source: SourceFlatpak}
	result, err := DryRun(pkg, modeNormal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
}

func TestDryRunSnap(t *testing.T) {
	pkg := PackageResult{Name: "test-snap", Source: SourceSnap}
	result, err := DryRun(pkg, modeNormal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
}

func TestDryRunAppImage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	appDir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pkg := PackageResult{Name: "test-app", Source: SourceAppImage, Path: filepath.Join(appDir, "test.AppImage")}
	result, err := DryRun(pkg, modeNormal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
}

func TestDryRunPacman(t *testing.T) {
	pkg := PackageResult{Name: "test-pkg", Source: SourcePacman}
	result, err := DryRun(pkg, modeNormal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
	if result.Commands[0] != "sudo pacman -R --noconfirm test-pkg" {
		t.Errorf("Unexpected command: %s", result.Commands[0])
	}
}

func TestDryRunPacmanComplete(t *testing.T) {
	pkg := PackageResult{Name: "test-pkg", Source: SourcePacman}
	result, err := DryRun(pkg, modeComplete)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
	if result.Commands[0] != "sudo pacman -Rns --noconfirm test-pkg" {
		t.Errorf("Unexpected command: %s", result.Commands[0])
	}
	if len(result.DirsToRemove) != 0 {
		t.Errorf("Expected no existing user-data dirs for fake package, got %v", result.DirsToRemove)
	}
}

func TestParsePacmanSize(t *testing.T) {
	mib := float64(1024 * 1024)
	tests := []struct {
		in   string
		want int64
	}{
		{"9.59 MiB", int64(9.59 * mib)},
		{"512.00 KiB", 512 * 1024},
		{"1.00 GiB", 1024 * 1024 * 1024},
		{"100 B", 100},
		{"", 0},
		{"bogus", 0},
	}
	for _, tt := range tests {
		got := parsePacmanSize(tt.in)
		if got != tt.want {
			t.Errorf("parsePacmanSize(%q) = %d; want %d", tt.in, got, tt.want)
		}
	}
}
