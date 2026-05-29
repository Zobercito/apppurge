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
	Commands      []string
	DirsToRemove  []string
	EstimateBytes int64
}

// DryRun simulates the uninstall and returns what would happen.
func DryRun(pkg PackageResult, mode string) (DryRunResult, error) {
	if !isValidPkgName(pkg.Name) {
		return DryRunResult{}, fmt.Errorf("invalid package name: %s", pkg.Name)
	}
	if isProtected(pkg) {
		return DryRunResult{}, fmt.Errorf("protected package: %s", pkg.Name)
	}

	result := DryRunResult{}

	switch pkg.Source {
	case SourceAPT:
		result = dryRunAPT(pkg, mode)
	case SourceFlatpak:
		result = dryRunFlatpak(pkg, mode)
	case SourceSnap:
		result = dryRunSnap(pkg, mode)
	case SourceAppImage:
		result = dryRunAppImage(pkg, mode)
	default:
		return result, fmt.Errorf("unknown source: %s", pkg.Source)
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
		if home, err := os.UserHomeDir(); err == nil {
			result.DirsToRemove = append(result.DirsToRemove,
				filepath.Join(home, ".config", strings.ToLower(pkg.Name)),
				filepath.Join(home, ".local", "share", strings.ToLower(pkg.Name)),
			)
		}
	}
	return result
}

func dryRunFlatpak(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	target := pkg.ID
	if target == "" {
		target = pkg.Name
	}
	if mode == modeComplete {
		result.Commands = append(result.Commands, "flatpak uninstall --delete-data -y "+target)
	} else {
		result.Commands = append(result.Commands, "flatpak uninstall -y "+target)
	}
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
	result.Commands = append(result.Commands, "sudo snap remove "+pkg.Name)
	if mode == modeComplete {
		if home, err := os.UserHomeDir(); err == nil {
			result.DirsToRemove = append(result.DirsToRemove,
				filepath.Join(home, "snap", pkg.Name),
			)
		}
	}
	return result
}

func dryRunAppImage(pkg PackageResult, mode string) DryRunResult {
	result := DryRunResult{}
	result.Commands = append(result.Commands, "rm "+pkg.Path)
	if mode == modeComplete {
		if home, err := os.UserHomeDir(); err == nil {
			result.DirsToRemove = append(result.DirsToRemove,
				filepath.Join(home, ".config", strings.ToLower(pkg.Name)),
				filepath.Join(home, ".local", "share", strings.ToLower(pkg.Name)),
			)
		}
	}
	return result
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
	case SourceFlatpak:
		_, err := exec.CommandContext(ctx, "flatpak", "info", "--show-ref", pkg.ID).Output()
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
