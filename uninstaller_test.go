package main

import (
	"testing"
)

func TestUninstallInvalidPackageName(t *testing.T) {
	invalidNames := []string{
		"foo; rm -rf /",
		"../etc/passwd",
		"foo/bar",
		"foo|bar",
	}

	for _, name := range invalidNames {
		pkg := PackageResult{Name: name, Source: "APT"}
		err := Uninstall(pkg, modeNormal)
		if err == nil {
			t.Errorf("Expected error for invalid package name %q", name)
		}
	}
}

func TestUninstallUnknownSource(t *testing.T) {
	pkg := PackageResult{Name: "valid-name", Source: "Unknown"}
	err := Uninstall(pkg, modeNormal)
	if err == nil {
		t.Error("Expected error for unknown source")
	}
}
