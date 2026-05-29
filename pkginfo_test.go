package main

import (
	"os/exec"
	"testing"
)

func TestGetPackageInfoAPT(t *testing.T) {
	if _, err := exec.LookPath("dpkg-query"); err != nil {
		t.Skip("dpkg-query not available")
	}
	pkg := PackageResult{Name: "coreutils", Source: SourceAPT}
	info := GetPackageInfo(pkg)
	if info.Name != "coreutils" {
		t.Errorf("Expected name 'coreutils', got %q", info.Name)
	}
}

func TestGetPackageInfoFlatpak(t *testing.T) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		t.Skip("flatpak not available")
	}
	pkg := PackageResult{Name: "test", ID: "com.test.App", Source: SourceFlatpak}
	info := GetPackageInfo(pkg)
	if info.Name != "test" {
		t.Errorf("Expected name 'test', got %q", info.Name)
	}
}

func TestGetPackageInfoSnap(t *testing.T) {
	if _, err := exec.LookPath("snap"); err != nil {
		t.Skip("snap not available")
	}
	pkg := PackageResult{Name: "test", Source: SourceSnap}
	info := GetPackageInfo(pkg)
	if info.Name != "test" {
		t.Errorf("Expected name 'test', got %q", info.Name)
	}
}

func TestGetPackageInfoAppImage(t *testing.T) {
	pkg := PackageResult{Name: "test-app", Source: SourceAppImage}
	info := GetPackageInfo(pkg)
	if info.Name != "test-app" {
		t.Errorf("Expected name 'test-app', got %q", info.Name)
	}
}
