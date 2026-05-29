package main

import "testing"

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
	if len(result.DirsToRemove) == 0 {
		t.Error("Expected dirs to remove in complete mode")
	}
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
	pkg := PackageResult{Name: "test-app", Source: SourceAppImage, Path: "/tmp/test.AppImage"}
	result, err := DryRun(pkg, modeNormal)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(result.Commands) == 0 {
		t.Error("Expected at least one command")
	}
}
