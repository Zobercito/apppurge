package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PackageResult represents a found package from any source.
type PackageResult struct {
	Name   string
	Source string // APT, Flatpak, Snap, AppImage
	ID     string // Flatpak application ID
	Path   string // AppImage file path
}

// ScanError represents an error that occurred during scanning a specific source.
type ScanError struct {
	Source string
	Err    error
}

func (e ScanError) Error() string {
	return e.Source + ": " + e.Err.Error()
}

// ScanResult holds the results and any errors from scanning all sources.
type ScanResult struct {
	Packages []PackageResult
	Errors   []ScanError
}

// ScanAll searches all package managers in parallel for the given term.
func ScanAll(term string) ScanResult {
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		packages []PackageResult
		errors   []ScanError
	)

	type scannerFunc func(context.Context, string) ([]PackageResult, error)
	scanners := []struct {
		name string
		fn   scannerFunc
	}{
		{SourceAPT, scanAPT},
		{SourceFlatpak, scanFlatpak},
		{SourceSnap, scanSnap},
		{SourceAppImage, scanAppImage},
	}

	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	wg.Add(len(scanners))
	for _, s := range scanners {
		go func(name string, fn scannerFunc) {
			defer wg.Done()
			found, err := fn(ctx, term)
			if err != nil {
				mu.Lock()
				errors = append(errors, ScanError{Source: name, Err: err})
				mu.Unlock()
				return
			}
			if len(found) > 0 {
				mu.Lock()
				packages = append(packages, found...)
				mu.Unlock()
			}
		}(s.name, s.fn)
	}
	wg.Wait()

	sortPackages(packages)

	return ScanResult{Packages: packages, Errors: errors}
}

func sortPackages(pkgs []PackageResult) {
	sourceOrder := map[string]int{SourceAPT: 0, SourceFlatpak: 1, SourceSnap: 2, SourceAppImage: 3}
	sort.SliceStable(pkgs, func(i, j int) bool {
		si := sourceOrder[pkgs[i].Source]
		sj := sourceOrder[pkgs[j].Source]
		if si != sj {
			return si < sj
		}
		return strings.ToLower(pkgs[i].Name) < strings.ToLower(pkgs[j].Name)
	})
}

var protectedPackages = map[string]bool{
	"snapd":              true,
	"systemd":            true,
	"linux-base":         true,
	"ubuntu-desktop":     true,
	"debian-base":        true,
	"gnome-shell":        true,
	"kde-plasma-desktop": true,
}

func isProtected(pkg PackageResult) bool {
	name := strings.ToLower(pkg.Name)
	if protectedPackages[name] {
		return true
	}
	protectedPrefixes := []string{"linux-image", "linux-headers", "initramfs", "grub"}
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func scanAPT(ctx context.Context, term string) ([]PackageResult, error) {
	if _, err := exec.LookPath("dpkg-query"); err != nil {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Package}\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var results []PackageResult
	lower := strings.ToLower(term)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name := line
		if idx := strings.Index(name, ":"); idx != -1 {
			name = name[:idx]
		}
		if strings.Contains(strings.ToLower(name), lower) {
			pkg := PackageResult{Name: name, Source: SourceAPT}
			if isProtected(pkg) {
				continue
			}
			results = append(results, pkg)
		}
	}
	return results, nil
}

func scanFlatpak(ctx context.Context, term string) ([]PackageResult, error) {
	if _, err := exec.LookPath("flatpak"); err != nil {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "flatpak", "list", "--app", "--columns=name,application")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var results []PackageResult
	lower := strings.ToLower(term)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		name := parts[0]
		id := ""
		if len(parts) > 1 {
			id = strings.TrimSpace(parts[1])
		}
		if strings.Contains(strings.ToLower(line), lower) {
			results = append(results, PackageResult{Name: name, Source: SourceFlatpak, ID: id})
		}
	}
	return results, nil
}

func scanSnap(ctx context.Context, term string) ([]PackageResult, error) {
	if _, err := exec.LookPath("snap"); err != nil {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "snap", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var results []PackageResult
	lower := strings.ToLower(term)
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		name := fields[0]
		pkg := PackageResult{Name: name, Source: SourceSnap}
		if isProtected(pkg) {
			continue
		}
		if strings.Contains(strings.ToLower(name), lower) {
			results = append(results, pkg)
		}
	}
	return results, nil
}

func scanAppImage(ctx context.Context, term string) ([]PackageResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	searchDirs := []string{
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Descargas"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, ".local", "share", "Applications"),
	}

	var results []PackageResult
	lower := strings.ToLower(term)

	for _, dir := range searchDirs {
		if ctx.Err() != nil {
			break
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".appimage") {
				continue
			}
			if strings.Contains(strings.ToLower(name), lower) {
				cleanName := strings.TrimSuffix(strings.ToLower(name), ".appimage")
				if len(cleanName) > 0 {
					cleanName = name[:len(cleanName)]
				}
				results = append(results, PackageResult{
					Name:   cleanName,
					Source: SourceAppImage,
					Path:   filepath.Join(dir, name),
				})
			}
		}
	}

	return results, nil
}
