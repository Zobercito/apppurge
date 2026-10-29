package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// DryRunResult contains the commands that would be executed and estimated space.
type DryRunResult struct {
	Commands          []string
	PackagesToRemove  []string
	RemovalPreviewErr string
	DirsToRemove      []string
	FilesToRemove     []string
	EstimateBytes     int64
}

// DryRun simulates the uninstall and returns what would happen.
func DryRun(pkg PackageResult, mode string) (DryRunResult, error) {
	switch pkg.Source {
	case SourceAPT, SourcePacman, SourceSnap:
		if !isValidPkgName(pkg.Name) {
			return DryRunResult{}, fmt.Errorf("invalid package name: %s", pkg.Name)
		}
	case SourceFlatpak:
		if !isValidFlatpakID(pkg.ID) || strings.ContainsAny(pkg.Installation, "\x00\r\n") {
			return DryRunResult{}, fmt.Errorf("invalid Flatpak reference")
		}
	case SourceAppImage:
		if err := validatePath(pkg.Path); err != nil {
			return DryRunResult{}, fmt.Errorf("invalid path: %w", err)
		}
	case SourceWebApp:
		if err := validateWebAppDesktopEntry(pkg.Path, pkg.Name, pkg.ID); err != nil {
			return DryRunResult{}, fmt.Errorf("invalid web app launcher: %w", err)
		}
	default:
		return DryRunResult{}, fmt.Errorf("unknown source: %s", pkg.Source)
	}
	if isProtected(pkg) {
		return DryRunResult{}, fmt.Errorf("protected package: %s", pkg.Name)
	}

	result := DryRunResult{}

	switch pkg.Source {
	case SourceAPT:
		result = dryRunAPT(pkg, mode)
	case SourcePacman:
		result = dryRunPacman(pkg, mode)
	case SourceFlatpak:
		result = dryRunFlatpak(pkg, mode)
	case SourceSnap:
		result = dryRunSnap(pkg, mode)
	case SourceAppImage:
		result = dryRunAppImage(pkg, mode)
	case SourceWebApp:
		result = dryRunWebApp(pkg)
	default:
		return result, fmt.Errorf("unknown source: %s", pkg.Source)
	}
	if mode == modeComplete {
		switch pkg.Source {
		case SourceAPT:
			result.PackagesToRemove, result.RemovalPreviewErr = previewAPTRemovals(pkg.Name)
		case SourcePacman:
			result.PackagesToRemove, result.RemovalPreviewErr = previewPacmanRemovals(pkg.Name)
		}
	}

	// Estimate space
	result.EstimateBytes = estimatePackageSpace(pkg)

	return result, nil
}

func dryRunAPT(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	if mode == modeComplete {
		result.Commands = append(result.Commands, "sudo apt purge --autoremove -y "+pkg.Name)
	} else {
		result.Commands = append(result.Commands, "sudo apt remove -y "+pkg.Name)
	}
	if mode == modeComplete {
		result.DirsToRemove = append(result.DirsToRemove, candidateUserDataDirs(pkg.Name)...)
	}
	return result
}

func dryRunPacman(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	if mode == modeComplete {
		result.Commands = append(result.Commands, "sudo pacman -Rns --noconfirm "+pkg.Name)
	} else {
		result.Commands = append(result.Commands, "sudo pacman -R --noconfirm "+pkg.Name)
	}
	if mode == modeComplete {
		result.DirsToRemove = append(result.DirsToRemove, candidateUserDataDirs(pkg.Name)...)
	}
	return result
}

// previewPacmanRemovals asks pacman to resolve a recursive removal without changing the system.
func previewPacmanRemovals(name string) ([]string, string) {
	if !isValidPkgName(name) {
		return nil, "invalid package name"
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	// Pacman rejects combining --print with -n (--nosave). The -n flag only
	// controls preservation of package configuration files; it does not change
	// which packages -Rs selects, so this previews the same removal set safely.
	cmd := exec.CommandContext(ctx, "pacman", "-Rs", "--print", "--print-format", "%n", name)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, err.Error()
	}
	seen := make(map[string]bool)
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, ""
}

// previewAPTRemovals simulates purge/autoremove and extracts its removal set.
func previewAPTRemovals(name string) ([]string, string) {
	if !isValidPkgName(name) {
		return nil, "invalid package name"
	}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "apt-get", "-s", "purge", "--autoremove", name)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, err.Error()
	}
	seen := make(map[string]bool)
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || (fields[0] != "Remv" && fields[0] != "Purg") {
			continue
		}
		if !seen[fields[1]] {
			seen[fields[1]] = true
			names = append(names, fields[1])
		}
	}
	return names, ""
}

func dryRunFlatpak(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	args := append([]string{"flatpak"}, flatpakScopeArgs(pkg)...)
	args = append(args, "uninstall")
	if mode == modeComplete {
		args = append(args, "--delete-data")
	}
	args = append(args, "-y", pkg.ID)
	result.Commands = append(result.Commands, strings.Join(args, " "))
	if mode == modeComplete && pkg.ID != "" {
		if home, err := os.UserHomeDir(); err == nil {
			result.DirsToRemove = append(result.DirsToRemove,
				filepath.Join(home, ".var", "app", pkg.ID),
			)
		}
	}
	return result
}

func dryRunSnap(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	command := "sudo snap remove " + pkg.Name
	if mode == modeComplete {
		command += " --purge"
		if home, err := os.UserHomeDir(); err == nil {
			result.DirsToRemove = append(result.DirsToRemove,
				filepath.Join(home, "snap", pkg.Name),
			)
		}
	}
	result.Commands = append(result.Commands, command)
	return result
}

func dryRunAppImage(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	result.Commands = append(result.Commands, "rm "+pkg.Path)
	if mode == modeComplete {
		result.DirsToRemove = append(result.DirsToRemove, candidateUserDataDirs(pkg.Name)...)
		result.FilesToRemove = append(result.FilesToRemove, candidateCompleteDesktopFiles(pkg)...)
	}
	return result
}

func dryRunWebApp(pkg PackageResult) DryRunResult {
	return DryRunResult{FilesToRemove: []string{pkg.Path}}
}

func estimatePackageSpace(pkg PackageResult) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	switch pkg.Source {
	case SourceAPT:
		out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${Installed-Size}", pkg.Name).Output()
		if err == nil {
			if size, pErr := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); pErr == nil {
				return size * 1024
			}
		}
	case SourcePacman:
		cmd := exec.CommandContext(ctx, "pacman", "-Qi", pkg.Name)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		out, err := cmd.Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "Installed Size") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						return parsePacmanSize(strings.TrimSpace(parts[1]))
					}
				}
			}
		}
	case SourceFlatpak:
		args := append(flatpakScopeArgs(pkg), "info", "--show-ref", pkg.ID)
		_, err := exec.CommandContext(ctx, "flatpak", args...).Output()
		if err == nil {
			home, homeErr := os.UserHomeDir()
			flatpakDirs := []string{"/var/lib/flatpak/app/" + pkg.ID}
			if homeErr == nil {
				flatpakDirs = append([]string{filepath.Join(home, ".local", "share", "flatpak", "app", pkg.ID)}, flatpakDirs...)
			}
			for _, dir := range flatpakDirs {
				out2, err2 := exec.CommandContext(ctx, "du", "-sb", dir).Output()
				if err2 == nil && len(out2) > 0 {
					parts := strings.Fields(string(out2))
					if len(parts) > 0 {
						if size, pErr := strconv.ParseInt(parts[0], 10, 64); pErr == nil {
							return size
						}
					}
				}
			}
		}
	case SourceSnap:
		out, err := exec.CommandContext(ctx, "snap", "info", pkg.Name).Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.HasPrefix(line, "installed:") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						sizeStr := parts[1]
						if strings.HasSuffix(sizeStr, "MB") {
							if num, pErr := strconv.ParseFloat(strings.TrimSuffix(sizeStr, "MB"), 64); pErr == nil {
								return int64(num * 1024 * 1024)
							}
						}
						if strings.HasSuffix(sizeStr, "GB") {
							if num, pErr := strconv.ParseFloat(strings.TrimSuffix(sizeStr, "GB"), 64); pErr == nil {
								return int64(num * 1024 * 1024 * 1024)
							}
						}
					}
				}
			}
		}
	case SourceAppImage:
		info, err := os.Stat(pkg.Path)
		if err == nil {
			return info.Size()
		}
	}
	return 0
}

// parsePacmanSize parses sizes like "9.59 MiB" or "512.00 KiB" into bytes.
func parsePacmanSize(s string) int64 {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0
	}
	num, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	unit := strings.ToLower(fields[1])
	switch unit {
	case "b":
		return int64(num)
	case "kib", "kb":
		return int64(num * 1024)
	case "mib", "mb":
		return int64(num * 1024 * 1024)
	case "gib", "gb":
		return int64(num * 1024 * 1024 * 1024)
	}
	return 0
}

// FormatBytes converts bytes to human-readable format.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
