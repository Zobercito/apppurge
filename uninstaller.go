package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var validPkgName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9 ._+\-]*$`)

// Flatpak IDs look like org.example.App (at least one dot).
var validFlatpakID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(\.[A-Za-z0-9._-]+)+$`)

func isValidPkgName(name string) bool {
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") {
		return false
	}
	// Block shell injection characters
	for _, c := range name {
		switch c {
		case ';', '|', '&', '$', '`', '(', ')', '{', '}', '<', '>', '!', '#', '~', '\\', '\'', '"', '\n', '\r':
			return false
		}
	}
	return validPkgName.MatchString(name)
}

func isValidFlatpakID(id string) bool {
	if id == "" || strings.Contains(id, "..") || strings.Contains(id, "/") {
		return false
	}
	for _, c := range id {
		switch c {
		case ';', '|', '&', '$', '`', '(', ')', '{', '}', '<', '>', '!', '#', '~', '\\', '\'', '"', '\n', '\r', ' ':
			return false
		}
	}
	return validFlatpakID.MatchString(id)
}

// needsPrivilege reports whether uninstalling this package typically needs sudo.
func needsPrivilege(pkg PackageResult) bool {
	switch pkg.Source {
	case SourceAPT, SourcePacman, SourceSnap:
		return true
	case SourceFlatpak:
		return pkg.Installation != "user"
	default:
		return false
	}
}

func flatpakScopeArgs(pkg PackageResult) []string {
	switch pkg.Installation {
	case "user":
		return []string{"--user"}
	case "", "system":
		return []string{"--system"}
	default:
		return []string{"--installation=" + pkg.Installation}
	}
}

// Uninstall executes the appropriate uninstall commands based on source and mode.
func Uninstall(pkg PackageResult, mode string) error {
	return UninstallWithDataDirs(pkg, mode, nil)
}

// UninstallWithDataDirs removes only the user-data paths shown in the confirmation view.
func UninstallWithDataDirs(pkg PackageResult, mode string, dataDirs []string) error {
	return UninstallWithDataAndDesktopFiles(pkg, mode, dataDirs, nil)
}

// UninstallWithDataAndDesktopFiles also removes only the explicitly confirmed
// user desktop entries associated with an AppImage.
func UninstallWithDataAndDesktopFiles(pkg PackageResult, mode string, dataDirs, desktopFiles []string) error {
	switch pkg.Source {
	case SourceAPT, SourcePacman, SourceSnap:
		if !isValidPkgName(pkg.Name) {
			return fmt.Errorf("invalid package name: %s", pkg.Name)
		}
	case SourceFlatpak:
		if !isValidFlatpakID(pkg.ID) {
			return fmt.Errorf("invalid flatpak id: %s", pkg.ID)
		}
		if strings.ContainsAny(pkg.Installation, "\x00\r\n") {
			return fmt.Errorf("invalid flatpak installation")
		}
	case SourceAppImage:
		if err := validatePath(pkg.Path); err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
	case SourceWebApp:
		if err := validateWebAppDesktopEntry(pkg.Path, pkg.Name, pkg.ID); err != nil {
			return fmt.Errorf("invalid web app launcher: %w", err)
		}
	default:
		return fmt.Errorf("unknown source: %s", pkg.Source)
	}
	if isProtected(pkg) {
		return fmt.Errorf("protected package: %s", pkg.Name)
	}

	start := time.Now()
	var err error

	switch pkg.Source {
	case SourceAPT:
		err = uninstallAPT(pkg, mode, dataDirs)
	case SourcePacman:
		err = uninstallPacman(pkg, mode, dataDirs)
	case SourceFlatpak:
		err = uninstallFlatpak(pkg, mode, dataDirs)
	case SourceSnap:
		err = uninstallSnap(pkg, mode, dataDirs)
	case SourceAppImage:
		err = uninstallAppImage(pkg, mode, dataDirs, desktopFiles)
	case SourceWebApp:
		err = removeWebAppLauncher(pkg)
	default:
		err = fmt.Errorf("unknown source: %s", pkg.Source)
	}

	entry := HistoryEntry{
		Timestamp: time.Now(),
		Name:      pkg.Name,
		Source:    pkg.Source,
		Mode:      mode,
		Success:   err == nil,
		Duration:  time.Since(start),
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if err := AddEntry(entry); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save history: %v\n", err)
	}

	return err
}

func uninstallAPT(pkg PackageResult, mode string, dataDirs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	var args []string
	if mode == modeComplete {
		args = []string{"apt", "purge", "--autoremove", "-y", pkg.Name}
	} else {
		args = []string{"apt", "remove", "-y", pkg.Name}
	}
	if err := runPrivilegedCtx(ctx, args...); err != nil {
		return err
	}
	if mode == modeComplete {
		if err := removeUserDataDirs(dataDirs); err != nil {
			return fmt.Errorf("package removed, but user-data cleanup failed: %w", err)
		}
	}
	return nil
}

func uninstallPacman(pkg PackageResult, mode string, dataDirs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	// -R: remove package. -Rns: also remove unneeded deps and skip .pacsave configs.
	var args []string
	if mode == modeComplete {
		args = []string{"pacman", "-Rns", "--noconfirm", pkg.Name}
	} else {
		args = []string{"pacman", "-R", "--noconfirm", pkg.Name}
	}
	if err := runPrivilegedCtx(ctx, args...); err != nil {
		return err
	}
	if mode == modeComplete {
		if err := removeUserDataDirs(dataDirs); err != nil {
			return fmt.Errorf("package removed, but user-data cleanup failed: %w", err)
		}
	}
	return nil
}

func uninstallFlatpak(pkg PackageResult, mode string, dataDirs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	if !isValidFlatpakID(pkg.ID) {
		return fmt.Errorf("invalid flatpak id: %s", pkg.ID)
	}
	args := append([]string{"flatpak"}, flatpakScopeArgs(pkg)...)
	args = append(args, "uninstall")
	if mode == modeComplete {
		args = append(args, "--delete-data")
	}
	args = append(args, "-y", pkg.ID)

	if needsPrivilege(pkg) {
		if err := runPrivilegedCtx(ctx, args...); err != nil {
			return err
		}
	} else {
		if err := runCmdCtx(ctx, args...); err != nil {
			return err
		}
	}

	if mode == modeComplete {
		if err := removeUserDataDirs(dataDirs); err != nil {
			return fmt.Errorf("package removed, but user-data cleanup failed: %w", err)
		}
	}
	return nil
}

func uninstallSnap(pkg PackageResult, mode string, dataDirs []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	args := []string{"snap", "remove", pkg.Name}
	if mode == modeComplete {
		// --purge prevents snapd from creating its automatic data snapshot.
		args = append(args, "--purge")
	}
	if err := runPrivilegedCtx(ctx, args...); err != nil {
		return err
	}
	if mode == modeComplete {
		if err := removeUserDataDirs(dataDirs); err != nil {
			return fmt.Errorf("package removed, but user-data cleanup failed: %w", err)
		}
	}
	return nil
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	if !strings.EqualFold(filepath.Ext(path), ".appimage") {
		return fmt.Errorf("file must have an .AppImage extension")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute")
	}
	for _, segment := range strings.Split(path, string(os.PathSeparator)) {
		if segment == ".." {
			return fmt.Errorf("path contains parent traversal")
		}
	}
	cleaned := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	allowedDirs := []string{
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Descargas"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, ".local", "share", "Applications"),
	}
	for _, dir := range allowedDirs {
		cleanDir := filepath.Clean(dir)
		if cleaned == cleanDir || !isPathInside(cleaned, cleanDir) {
			continue
		}
		resolvedHome, err := filepath.EvalSymlinks(home)
		if err != nil {
			return err
		}
		rootInfo, rootErr := os.Lstat(cleanDir)
		rootExists := rootErr == nil
		if rootErr != nil && !os.IsNotExist(rootErr) {
			return rootErr
		}
		var resolvedRoot string
		if rootExists {
			if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
				continue
			}
			resolvedRoot, err = filepath.EvalSymlinks(cleanDir)
			if err != nil || !isPathInside(resolvedRoot, resolvedHome) {
				continue
			}
		}
		// Resolve the nearest existing ancestor. This supports valid paths below
		// allowed folders that have not been created yet, while still rejecting
		// symlinks that escape the home directory or the allowed root.
		for ancestor := filepath.Dir(cleaned); ; ancestor = filepath.Dir(ancestor) {
			info, statErr := os.Lstat(ancestor)
			if statErr == nil {
				if !info.IsDir() {
					break
				}
				resolvedAncestor, resolveErr := filepath.EvalSymlinks(ancestor)
				if resolveErr != nil {
					return resolveErr
				}
				if !isPathInside(resolvedAncestor, resolvedHome) ||
					(rootExists && !isPathInside(resolvedAncestor, resolvedRoot)) {
					break
				}
				return nil
			}
			if !os.IsNotExist(statErr) {
				return statErr
			}
			parent := filepath.Dir(ancestor)
			if parent == ancestor {
				break
			}
		}
	}
	return fmt.Errorf("path not in allowed directories")
}

// isPathInside reports whether path is the root directory or a file/dir inside it.
// Uses filepath.Rel to avoid HasPrefix false positives (e.g. DownloadsEvil).
func isPathInside(path, root string) bool {
	cleaned := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleaned == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleaned)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func uninstallAppImage(pkg PackageResult, mode string, dataDirs, desktopFiles []string) error {
	if err := validatePath(pkg.Path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if err := os.Remove(pkg.Path); err != nil {
		return fmt.Errorf("could not remove %s: %w", pkg.Path, err)
	}
	if mode == modeComplete {
		if err := removeUserDataDirs(dataDirs); err != nil {
			return fmt.Errorf("AppImage removed, but user-data cleanup failed: %w", err)
		}
		if err := removeAppImageDesktopFiles(desktopFiles, pkg.Path); err != nil {
			return fmt.Errorf("AppImage removed, but desktop-entry cleanup failed: %w", err)
		}
	}
	return nil
}

// candidateUserDataDirs returns existing ~/.config/<name> and ~/.local/share/<name>
// directories that are safe single-segment targets for complete uninstall.
func candidateUserDataDirs(name string) []string {
	// Very short names commonly collide with shared or manually managed config
	// directories (for example ~/.config/go). Leave those for the user to clean.
	if !isValidPkgName(name) || len([]rune(name)) < 4 {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	roots := []string{
		filepath.Join(home, ".config"),
		filepath.Join(home, ".local", "share"),
	}

	seen := map[string]bool{}
	var dirs []string
	for _, n := range []string{strings.ToLower(name), name} {
		if !isValidPkgName(n) {
			continue
		}
		for _, root := range roots {
			dir := filepath.Join(root, n)
			if seen[dir] {
				continue
			}
			if !isSafeUserDataDir(dir, root) {
				continue
			}
			info, err := os.Lstat(dir)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// candidateCompleteDataDirs returns existing user-data paths shown before confirmation.
func candidateCompleteDataDirs(pkg PackageResult) []string {
	dirs := candidateUserDataDirs(pkg.Name)
	home, err := os.UserHomeDir()
	if err != nil {
		return dirs
	}
	var extra string
	switch pkg.Source {
	case SourceFlatpak:
		if isValidFlatpakID(pkg.ID) {
			extra = filepath.Join(home, ".var", "app", pkg.ID)
		}
	case SourceSnap:
		if isValidPkgName(pkg.Name) {
			extra = filepath.Join(home, "snap", pkg.Name)
		}
	}
	if extra != "" {
		if isSafeUserDataDir(extra, filepath.Dir(extra)) {
			info, err := os.Lstat(extra)
			if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				dirs = append(dirs, extra)
			}
		}
	}
	return dirs
}

// isSafeUserDataDir requires dir to be exactly one path segment under root
// (e.g. ~/.config/firefox), never ~/.config itself or nested escapes.
func isSafeUserDataDir(dir, root string) bool {
	cleaned := filepath.Clean(dir)
	cleanRoot := filepath.Clean(root)
	rootInfo, err := os.Lstat(cleanRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		return false
	}
	resolvedRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil || !isPathInside(resolvedRoot, resolvedHome) {
		return false
	}
	rel, err := filepath.Rel(cleanRoot, cleaned)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "..") {
		return false
	}
	return !strings.Contains(rel, string(os.PathSeparator))
}

// removeUserDataDirs deletes only the provided directories after re-validating
// that each is a safe single-segment path under ~/.config or ~/.local/share.
func removeUserDataDirs(dirs []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %w", err)
	}
	roots := []string{
		filepath.Join(home, ".config"),
		filepath.Join(home, ".local", "share"),
		filepath.Join(home, ".var", "app"),
		filepath.Join(home, "snap"),
	}
	for _, dir := range dirs {
		ok := false
		for _, root := range roots {
			if isSafeUserDataDir(dir, root) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("refusing to remove unsafe path: %s", dir)
		}
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("could not inspect directory %s: %w", dir, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to remove non-directory or symlink: %s", dir)
		}
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("could not remove directory %s: %w", dir, err)
		}
	}
	return nil
}

// runPrivilegedCtx runs a command with sudo if not already root.
func runPrivilegedCtx(ctx context.Context, args ...string) error {
	if os.Geteuid() != 0 {
		args = append([]string{"sudo", "-n", "--prompt=Password (sudo): "}, args...)
	}
	return runCmdCtx(ctx, args...)
}

// runCmdCtx executes a command with context timeout and returns any error.
func runCmdCtx(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out (%v)", uninstallTimeout)
	}
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
