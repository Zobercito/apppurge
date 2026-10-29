package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestValidatePathOnlyAcceptsFilesInsideAllowedDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	allowed := filepath.Join(home, "Downloads", "app.AppImage")
	if err := validatePath(allowed); err != nil {
		t.Fatalf("validatePath(%q) returned error: %v", allowed, err)
	}

	for _, path := range []string{
		filepath.Join(home, "Downloads"),
		filepath.Join(home, "DownloadsEvil", "app.AppImage"),
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Downloads", "personal-notes.txt"),
	} {
		if err := validatePath(path); err == nil {
			t.Errorf("validatePath(%q) unexpectedly succeeded", path)
		}
	}
}

func TestValidatePathRejectsSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "Downloads")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "outside")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := validatePath(filepath.Join(link, "app.AppImage")); err == nil {
		t.Fatal("validatePath accepted a path escaping through a symlink")
	}
}

func TestCandidateCompleteDataDirsAndExplicitRemoval(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configDir := filepath.Join(home, ".config", "test-app")
	shareDir := filepath.Join(home, ".local", "share", "test-app")
	for _, dir := range []string{configDir, shareDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	pkg := PackageResult{Name: "test-app", Source: SourceAppImage}
	dirs := candidateCompleteDataDirs(pkg)
	if !reflect.DeepEqual(dirs, []string{configDir, shareDir}) {
		t.Fatalf("candidateCompleteDataDirs() = %v; want [%s %s]", dirs, configDir, shareDir)
	}

	if err := removeUserDataDirs([]string{configDir}); err != nil {
		t.Fatalf("removeUserDataDirs() returned error: %v", err)
	}
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Errorf("confirmed directory still exists, stat error: %v", err)
	}
	if _, err := os.Stat(shareDir); err != nil {
		t.Errorf("unconfirmed directory was removed: %v", err)
	}

	for _, unsafe := range []string{
		filepath.Join(home, ".config"),
		filepath.Join(home, ".config", "..", "outside"),
	} {
		if err := removeUserDataDirs([]string{unsafe}); err == nil {
			t.Errorf("removeUserDataDirs(%q) unexpectedly succeeded", unsafe)
		}
	}
}

func TestPreviewPacmanRemovals(t *testing.T) {
	binDir := t.TempDir()
	pacman := filepath.Join(binDir, "pacman")
	if err := os.WriteFile(pacman, []byte("#!/bin/sh\nprintf 'app\\nshared-dep\\nshared-dep\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	got, errText := previewPacmanRemovals("app")
	if errText != "" {
		t.Fatalf("previewPacmanRemovals() error = %q", errText)
	}
	want := []string{"app", "shared-dep"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("previewPacmanRemovals() = %v; want %v", got, want)
	}
	if _, errText := previewPacmanRemovals("bad;name"); !strings.Contains(errText, "invalid") {
		t.Errorf("invalid name error = %q; want an invalid-name error", errText)
	}
}

func TestUninstallUnknownSource(t *testing.T) {
	pkg := PackageResult{Name: "valid-name", Source: "Unknown"}
	err := Uninstall(pkg, modeNormal)
	if err == nil {
		t.Error("Expected error for unknown source")
	}
}
