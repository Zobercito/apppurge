package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// PackageResult represents a found package from any source.
type PackageResult struct {
	Name         string
	Source       string   // APT, Pacman, Flatpak, Snap, AppImage
	ID           string   // Flatpak application ID
	Installation string   // Flatpak installation scope or installation name
	Path         string   // AppImage file path
	DesktopFiles []string // User desktop entries that launch this AppImage
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
	Packages  []PackageResult
	Errors    []ScanError
	Truncated bool
}

// ScanAll searches all package managers in parallel for the given term.
func ScanAll(term string) ScanResult {
	term = strings.TrimSpace(term)
	if len([]rune(term)) < minSearchLen {
		return ScanResult{Errors: []ScanError{{
			Source: "Search",
			Err:    fmt.Errorf("search term must be at least %d characters", minSearchLen),
		}}}
	}
	return scanInventory(term, true)
}

// ScanInstalled loads the installed inventory for interactive local suggestions.
func ScanInstalled() ScanResult { return scanInventory("", false) }

func scanInventory(term string, limit bool) ScanResult {
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
		{SourcePacman, scanPacman},
		{SourceFlatpak, scanFlatpak},
		{SourceSnap, scanSnap},
		{SourceAppImage, scanAppImage},
		{SourceWebApp, scanWebApps},
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
	sort.SliceStable(errors, func(i, j int) bool { return errors[i].Source < errors[j].Source })

	if !limit {
		return ScanResult{Packages: packages, Errors: errors}
	}
	// Apply the cap independently per source so a large APT result set does not
	// hide every result from the other package managers.
	counts := make(map[string]int)
	limited := make([]PackageResult, 0, len(packages))
	truncated := false
	for _, pkg := range packages {
		if counts[pkg.Source] >= maxScanResults {
			truncated = true
			continue
		}
		counts[pkg.Source]++
		limited = append(limited, pkg)
	}

	return ScanResult{Packages: limited, Errors: errors, Truncated: truncated}
}

func sortPackages(pkgs []PackageResult) {
	sourceOrder := map[string]int{
		SourceAPT: 0, SourcePacman: 1, SourceFlatpak: 2, SourceSnap: 3, SourceAppImage: 4, SourceWebApp: 5,
	}
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
	// Debian/Ubuntu
	"snapd":              true,
	"systemd":            true,
	"linux-base":         true,
	"ubuntu-desktop":     true,
	"debian-base":        true,
	"gnome-shell":        true,
	"kde-plasma-desktop": true,
	// Arch metapackage + common essentials (also safe to protect on Debian)
	"base":               true,
	"filesystem":         true,
	"glibc":              true,
	"gcc-libs":           true,
	"pacman":             true,
	"bash":               true,
	"sudo":               true,
	"coreutils":          true,
	"util-linux":         true,
	"systemd-sysvcompat": true,
	"archlinux-keyring":  true,
	"zlib":               true,
	"openssl":            true,
	"glib2":              true,
	"dbus":               true,
	"ncurses":            true,
	"readline":           true,
	"xz":                 true,
	"zstd":               true,
	"libgcrypt":          true,
	"pam":                true,
	"shadow":             true,
	"linux":              true,
	"linux-lts":          true,
	"linux-zen":          true,
	"linux-hardened":     true,
	"linux-rt":           true,
	"linux-firmware":     true,
	"linux-headers":      true,
	"linux-api-headers":  true,
	"linux-lts-headers":  true,
	"linux-zen-headers":  true,
	"linux-rt-headers":   true,
}

var aptProtectedPackages = map[string]bool{
	"apt": true, "base-files": true, "base-passwd": true,
	"dpkg": true, "init": true, "init-system-helpers": true,
	"libc-bin": true, "libc6": true, "systemd-sysv": true,
}

// pacmanEssentials caches dependencies of the "base" metapackage (Arch).
var (
	pacmanEssentials     map[string]bool
	pacmanEssentialsOnce sync.Once
	aptEssentials        map[string]bool
	aptEssentialsOnce    sync.Once
)

func loadAPTEssentials() {
	aptEssentialsOnce.Do(func() {
		aptEssentials = map[string]bool{}
		if _, err := exec.LookPath("dpkg-query"); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${binary:Package}\t${Essential}\n")
		out, err := cmd.Output()
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
			if len(fields) == 2 && strings.EqualFold(strings.TrimSpace(fields[1]), "yes") {
				name := fields[0]
				if idx := strings.IndexByte(name, ':'); idx >= 0 {
					name = name[:idx]
				}
				aptEssentials[strings.ToLower(name)] = true
			}
		}
	})
}

func loadPacmanEssentials() {
	pacmanEssentialsOnce.Do(func() {
		pacmanEssentials = map[string]bool{}
		if _, err := exec.LookPath("pacman"); err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "pacman", "-Qgq", "base")
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		out, err := cmd.Output()
		if err != nil {
			return
		}
		for _, name := range strings.Fields(string(out)) {
			pacmanEssentials[name] = true
		}
	})
}

func isProtected(pkg PackageResult) bool {
	if pkg.Source == SourceFlatpak || pkg.Source == SourceAppImage || pkg.Source == SourceWebApp {
		return false
	}
	name := strings.ToLower(pkg.Name)
	if protectedPackages[name] {
		return true
	}
	if pkg.Source == SourceAPT {
		if aptProtectedPackages[name] {
			return true
		}
		loadAPTEssentials()
		if aptEssentials[name] {
			return true
		}
	}
	protectedPrefixes := []string{
		"linux-image", "linux-headers", "initramfs", "grub",
		"mkinitcpio", "limine",
	}
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	if pkg.Source == SourcePacman {
		loadPacmanEssentials()
		if pacmanEssentials[name] {
			return true
		}
	}
	return false
}

// searchAliases maps common abbreviations or alternative names to the actual
// package name. This allows users to find packages by well-known short names.
var searchAliases = map[string]string{
	"vscode":   "code",
	"vscodium": "codium",
	"chrome":   "google-chrome",
	"ff":       "firefox",
	"tb":       "thunderbird",
}

// aliasREs holds word-boundary regexps for each alias, pre-compiled at init.
// When an alias is used, the canonical name must appear as a whole word
// (not as a substring), avoiding false positives like "microcode" for "code".
var aliasREs map[string]*regexp.Regexp

func init() {
	aliasREs = make(map[string]*regexp.Regexp, len(searchAliases))
	for alias, canonical := range searchAliases {
		aliasREs[alias] = regexp.MustCompile(`\b` + regexp.QuoteMeta(canonical) + `\b`)
	}
}

// textMatches checks whether text contains the search term. It tries a direct
// substring match first, then falls back to known aliases using word-boundary
// matching so that e.g. searching "vscode" finds the package "code" but not
// "amd64-microcode".
func textMatches(text, term string) bool {
	lowerText := strings.ToLower(text)
	t := strings.ToLower(term)
	if strings.Contains(lowerText, t) {
		return true
	}
	if re, ok := aliasREs[t]; ok {
		return re.MatchString(lowerText)
	}
	return false
}

func scanAPT(ctx context.Context, term string) ([]PackageResult, error) {
	if _, err := exec.LookPath("dpkg-query"); err != nil {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${db:Status-Abbrev}\t${Package}\n")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseAPTInstalledPackages(string(out), term), nil
}

func parseAPTInstalledPackages(output, term string) []PackageResult {
	var results []PackageResult
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 {
			continue
		}
		status := strings.TrimSpace(fields[0])
		if len(status) < 2 || status[1] != 'i' {
			continue
		}
		name := strings.TrimSpace(fields[1])
		if idx := strings.Index(name, ":"); idx != -1 {
			name = name[:idx]
		}
		if seen[name] {
			continue
		}
		if textMatches(name, term) {
			pkg := PackageResult{Name: name, Source: SourceAPT}
			if isProtected(pkg) {
				continue
			}
			seen[name] = true
			results = append(results, pkg)
		}
	}
	return results
}

func scanPacman(ctx context.Context, term string) ([]PackageResult, error) {
	if _, err := exec.LookPath("pacman"); err != nil {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "pacman", "-Qq")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var results []PackageResult
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if textMatches(line, term) {
			pkg := PackageResult{Name: line, Source: SourcePacman}
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
	cmd := exec.CommandContext(ctx, "flatpak", "list", "--app", "--columns=name,application,installation")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var results []PackageResult
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		name := parts[0]
		id := ""
		installation := ""
		if len(parts) > 1 {
			id = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			installation = strings.TrimSpace(parts[2])
		}
		if !isValidFlatpakID(id) || installation == "" {
			continue
		}
		if textMatches(line, term) {
			results = append(results, PackageResult{Name: name, Source: SourceFlatpak, ID: id, Installation: installation})
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
		if textMatches(name, term) {
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
	desktopFilesByAppImage := appImageDesktopEntries()

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
			if textMatches(name, term) {
				cleanName := name[:len(name)-len(".appimage")]
				appImagePath := filepath.Join(dir, name)
				results = append(results, PackageResult{
					Name:         cleanName,
					Source:       SourceAppImage,
					Path:         appImagePath,
					DesktopFiles: desktopFilesByAppImage[filepath.Clean(appImagePath)],
				})
			}
		}
	}

	return results, nil
}
