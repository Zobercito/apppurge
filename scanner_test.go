package main

import (
	"context"
	"os/exec"
	"testing"
)

func TestScanAPT(t *testing.T) {
	if _, err := exec.LookPath("dpkg-query"); err != nil {
		t.Skip("dpkg-query not available")
	}
	ctx := context.Background()
	results, err := scanAPT(ctx, "core")
	if err != nil {
		t.Fatalf("scanAPT returned error: %v", err)
	}
	if len(results) == 0 {
		t.Log("No APT packages found for 'core' (may be expected)")
		return
	}
	for _, r := range results {
		if r.Source != SourceAPT {
			t.Errorf("Expected source APT, got %s", r.Source)
		}
		if r.Name == "" {
			t.Error("Package name should not be empty")
		}
	}
}

func TestScanAPTLookPath(t *testing.T) {
	if _, err := exec.LookPath("dpkg-query"); err == nil {
		t.Skip("dpkg-query available, skipping lookpath test")
	}
	ctx := context.Background()
	results, err := scanAPT(ctx, "test")
	if err != nil {
		t.Errorf("Expected nil error when dpkg-query not found, got: %v", err)
	}
	if results != nil {
		t.Errorf("Expected nil results when dpkg-query not found, got: %v", results)
	}
}

func TestSortPackages(t *testing.T) {
	pkgs := []PackageResult{
		{Name: "zebra", Source: SourceSnap},
		{Name: "alpha", Source: SourceAPT},
		{Name: "beta", Source: SourceFlatpak},
		{Name: "gamma", Source: SourceAPT},
	}

	sortPackages(pkgs)

	expected := []string{SourceAPT, SourceAPT, SourceFlatpak, SourceSnap}
	for i, pkg := range pkgs {
		if pkg.Source != expected[i] {
			t.Errorf("Position %d: expected source %s, got %s", i, expected[i], pkg.Source)
		}
	}

	if pkgs[0].Name != "alpha" || pkgs[1].Name != "gamma" {
		t.Errorf("APT packages not sorted alphabetically: %s, %s", pkgs[0].Name, pkgs[1].Name)
	}
}

func TestIsValidPkgName(t *testing.T) {
	valid := []string{
		"firefox",
		"vlc",
		"code",
		"my-app",
		"my_app",
		"app123",
	}

	for _, name := range valid {
		if !isValidPkgName(name) {
			t.Errorf("Expected %q to be valid", name)
		}
	}

	invalid := []string{
		"foo; rm -rf /",
		"../etc/passwd",
		"foo/bar",
		"",
		"foo|bar",
		"foo`bar",
	}

	for _, name := range invalid {
		if isValidPkgName(name) {
			t.Errorf("Expected %q to be invalid", name)
		}
	}
}

func TestIsProtected(t *testing.T) {
	protected := []PackageResult{
		{Name: "snapd", Source: SourceSnap},
		{Name: "systemd", Source: SourceAPT},
		{Name: "linux-image-generic", Source: SourceAPT},
		{Name: "linux-headers-generic", Source: SourceAPT},
		{Name: "initramfs-tools", Source: SourceAPT},
		{Name: "grub-efi", Source: SourceAPT},
	}

	for _, pkg := range protected {
		if !isProtected(pkg) {
			t.Errorf("Expected %s to be protected", pkg.Name)
		}
	}

	unprotected := []PackageResult{
		{Name: "firefox", Source: SourceAPT},
		{Name: "vlc", Source: SourceFlatpak},
		{Name: "code", Source: SourceSnap},
		{Name: "gimp", Source: SourceAPT},
	}

	for _, pkg := range unprotected {
		if isProtected(pkg) {
			t.Errorf("Expected %s to NOT be protected", pkg.Name)
		}
	}
}

func TestScanAll(t *testing.T) {
	if _, err := exec.LookPath("dpkg-query"); err != nil {
		t.Skip("dpkg-query not available")
	}
	result := ScanAll("coreutils")
	if len(result.Packages) == 0 {
		t.Log("No packages found for 'coreutils' (may be expected)")
	}
	for _, pkg := range result.Packages {
		if pkg.Name == "" {
			t.Error("Package name should not be empty")
		}
	}
}
