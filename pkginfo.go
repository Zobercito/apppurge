package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// PackageInfo contains detailed information about a package.
type PackageInfo struct {
	Name          string
	Version       string
	Description   string
	InstalledSize string
	Dependents    []string
}

// GetPackageInfo retrieves detailed information about a package.
func GetPackageInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}

	switch pkg.Source {
	case SourceAPT:
		info = getAPTInfo(pkg)
	case SourceFlatpak:
		info = getFlatpakInfo(pkg)
	case SourceSnap:
		info = getSnapInfo(pkg)
	case SourceAppImage:
		info = getAppImageInfo(pkg)
	}

	return info
}

func getAPTInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "dpkg-query", "-W",
		"-f=${Version} ${Installed-Size} ${Description}", pkg.Name).Output()
	if err == nil {
		fields := strings.Fields(string(out))
		if len(fields) >= 1 {
			info.Version = fields[0]
		}
		if len(fields) >= 2 {
			size, convErr := strconv.ParseInt(fields[1], 10, 64)
			if convErr == nil {
				info.InstalledSize = FormatBytes(size * 1024)
			} else {
				info.InstalledSize = fields[1]
			}
		}
		if len(fields) > 2 {
			info.Description = strings.Join(fields[2:], " ")
			if len(info.Description) > 100 {
				info.Description = info.Description[:97] + "..."
			}
		}
	}

	out, err = exec.CommandContext(ctx, "apt-cache", "rdepends", "--installed", pkg.Name).Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "|") {
				info.Dependents = append(info.Dependents, strings.TrimPrefix(line, "  "))
			}
		}
	}

	return info
}

func getFlatpakInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "flatpak", "info", pkg.ID).Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "Version:") {
				info.Version = strings.TrimSpace(strings.TrimPrefix(line, "Version:"))
			}
			if strings.HasPrefix(line, "Description:") {
				info.Description = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
			}
			if strings.HasPrefix(line, "Installed:") {
				info.InstalledSize = strings.TrimSpace(strings.TrimPrefix(line, "Installed:"))
			}
		}
	}

	return info
}

func getSnapInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "snap", "info", pkg.Name).Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "version:") {
				info.Version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
			}
			if strings.HasPrefix(line, "summary:") {
				info.Description = strings.TrimSpace(strings.TrimPrefix(line, "summary:"))
			}
			if strings.HasPrefix(line, "installed:") {
				info.InstalledSize = strings.TrimSpace(strings.TrimPrefix(line, "installed:"))
			}
		}
	}

	return info
}

func getAppImageInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}
	if pkg.Path != "" {
		if fi, err := os.Stat(pkg.Path); err == nil {
			info.InstalledSize = FormatBytes(fi.Size())
		}
	}
	return info
}
