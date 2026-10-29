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
	Name            string
	Version         string
	Description     string
	InstalledSize   string
	Dependents      []string
	DependentsKnown bool
	DependentsError string
}

// GetPackageInfo retrieves detailed information about a package.
func GetPackageInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}

	switch pkg.Source {
	case SourceAPT:
		info = getAPTInfo(pkg)
	case SourcePacman:
		info = getPacmanInfo(pkg)
	case SourceFlatpak:
		info = getFlatpakInfo(pkg)
	case SourceSnap:
		info = getSnapInfo(pkg)
	case SourceAppImage:
		info = getAppImageInfo(pkg)
	case SourceWebApp:
		info = PackageInfo{
			Name:        pkg.Name,
			Description: "Lanzador web de usuario. Quitar el lanzador no elimina los datos del navegador.",
		}
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
	if err != nil {
		info.DependentsError = err.Error()
		return info
	}
	info.DependentsKnown = true
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		raw := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip tree-drawing continuation lines
		if strings.HasPrefix(line, "|") {
			continue
		}
		// Skip header lines (e.g. "Reverse Depends:") — real deps are indented
		if len(raw) > 0 && raw[0] != ' ' && raw[0] != '\t' {
			continue
		}
		info.Dependents = append(info.Dependents, strings.TrimPrefix(line, "  "))
	}

	return info
}

func getPacmanInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pacman", "-Qi", pkg.Name)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		info.DependentsError = err.Error()
		return info
	}

	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Version":
			info.Version = val
		case "Description":
			info.Description = val
			if len(info.Description) > 100 {
				info.Description = info.Description[:97] + "..."
			}
		case "Installed Size":
			if size := parsePacmanSize(val); size > 0 {
				info.InstalledSize = FormatBytes(size)
			} else {
				info.InstalledSize = val
			}
		case "Required By":
			info.DependentsKnown = true
			if val != "" && !strings.EqualFold(val, "None") {
				for _, dep := range strings.Fields(val) {
					info.Dependents = append(info.Dependents, dep)
				}
			}
		}
	}
	if !info.DependentsKnown {
		info.DependentsError = "pacman output did not include Required By"
	}

	return info
}

func getFlatpakInfo(pkg PackageResult) PackageInfo {
	info := PackageInfo{Name: pkg.Name}
	ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
	defer cancel()

	args := append(flatpakScopeArgs(pkg), "info", pkg.ID)
	out, err := exec.CommandContext(ctx, "flatpak", args...).Output()
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
