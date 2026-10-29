package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// appImageDesktopEntries finds user menu entries whose Exec program is exactly
// an AppImage path. It deliberately ignores wrappers and indirect launchers.
func appImageDesktopEntries() map[string][]string {
	entriesByAppImage := make(map[string][]string)
	home, err := os.UserHomeDir()
	if err != nil {
		return entriesByAppImage
	}
	root := filepath.Join(userDataHome(home), "applications")
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return entriesByAppImage
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return entriesByAppImage
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".desktop" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		appImagePath, ok := desktopEntryAppImage(path)
		if ok && filepath.IsAbs(appImagePath) {
			cleanPath := filepath.Clean(appImagePath)
			entriesByAppImage[cleanPath] = append(entriesByAppImage[cleanPath], path)
		}
	}
	for appImagePath := range entriesByAppImage {
		sort.Strings(entriesByAppImage[appImagePath])
	}
	return entriesByAppImage
}

func desktopEntryAppImage(path string) (appImagePath string, ok bool) {
	// The path comes from enumerated per-user application directories.
	//nolint:gosec // G304: scanning these desktop entries is intentional.
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() {
		if err := file.Close(); err != nil {
			appImagePath, ok = "", false
		}
	}()

	inDesktopEntry := false
	typeApplication := false
	execLine := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDesktopEntry = line == "[Desktop Entry]"
			continue
		}
		if !inDesktopEntry {
			continue
		}
		if strings.HasPrefix(line, "Type=") {
			typeApplication = strings.TrimPrefix(line, "Type=") == "Application"
		}
		if strings.HasPrefix(line, "Exec=") {
			execLine = strings.TrimPrefix(line, "Exec=")
		}
	}
	if scanner.Err() != nil || !typeApplication || execLine == "" {
		return "", false
	}
	program, ok := desktopExecProgram(execLine)
	return program, ok
}

// desktopExecProgram parses only the first Exec argument. It handles quoting
// and escaped whitespace without interpreting the entry as a shell command.
func desktopExecProgram(execLine string) (string, bool) {
	execLine = strings.TrimSpace(execLine)
	if execLine == "" {
		return "", false
	}

	var program strings.Builder
	escaped := false
	quoted := false
	started := false
	for _, char := range execLine {
		if escaped {
			program.WriteRune(char)
			escaped = false
			started = true
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
			started = true
			continue
		}
		if !quoted && (char == ' ' || char == '\t') {
			if started {
				break
			}
			continue
		}
		program.WriteRune(char)
		started = true
	}
	if escaped || quoted || program.Len() == 0 {
		return "", false
	}
	return program.String(), true
}

func candidateCompleteDesktopFiles(pkg PackageResult) []string {
	if pkg.Source != SourceAppImage || !filepath.IsAbs(pkg.Path) {
		return nil
	}
	var files []string
	for _, path := range pkg.DesktopFiles {
		if validateAppImageDesktopFile(path, pkg.Path) == nil {
			files = append(files, filepath.Clean(path))
		}
	}
	return files
}

func validateAppImageDesktopFile(path, appImagePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := filepath.Join(userDataHome(home), "applications")
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return os.ErrNotExist
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || rel == "." || strings.Contains(rel, string(os.PathSeparator)) || strings.HasPrefix(rel, "..") || strings.ToLower(filepath.Ext(rel)) != ".desktop" {
		return os.ErrPermission
	}
	info, err := os.Lstat(cleanPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrPermission
	}
	matchedPath, ok := desktopEntryAppImage(cleanPath)
	if !ok || filepath.Clean(matchedPath) != filepath.Clean(appImagePath) {
		return os.ErrPermission
	}
	return nil
}

func userDataHome(home string) string {
	if dataHome := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dataHome) {
		return filepath.Clean(dataHome)
	}
	return filepath.Join(home, ".local", "share")
}

func scanWebApps(ctx context.Context, term string) ([]PackageResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(userDataHome(home), "applications")
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	results := make([]PackageResult, 0)
	for _, entry := range entries {
		if ctx.Err() != nil {
			return results, nil
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".desktop" {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		name, execLine, ok := desktopEntryWebApp(path)
		if !ok {
			continue
		}
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if textMatches(name+" "+execLine, term) {
			results = append(results, PackageResult{Name: name, Source: SourceWebApp, Path: path, ID: execLine})
		}
	}
	return results, nil
}

func desktopEntryWebApp(path string) (name, execLine string, ok bool) {
	// The path comes from enumerated per-user application directories.
	//nolint:gosec // G304: scanning these desktop entries is intentional.
	file, err := os.Open(path)
	if err != nil {
		return "", "", false
	}
	defer func() {
		if err := file.Close(); err != nil {
			name, execLine, ok = "", "", false
		}
	}()

	inDesktopEntry := false
	typeApplication := false
	hidden := false
	noDisplay := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inDesktopEntry = line == "[Desktop Entry]"
			continue
		}
		if !inDesktopEntry {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "Type":
			typeApplication = value == "Application"
		case "Name":
			name = value
		case "Exec":
			execLine = value
		case "Hidden":
			hidden = strings.EqualFold(value, "true")
		case "NoDisplay":
			noDisplay = strings.EqualFold(value, "true")
		}
	}
	if scanner.Err() != nil || !typeApplication || hidden || noDisplay || execLine == "" {
		return "", "", false
	}
	program, valid := desktopExecProgram(execLine)
	if !valid || filepath.Base(program) != "omarchy-launch-webapp" {
		return "", "", false
	}
	return name, execLine, true
}

func validateWebAppDesktopEntry(path, expectedName, expectedExec string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := filepath.Join(userDataHome(home), "applications")
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return os.ErrNotExist
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || rel == "." || strings.Contains(rel, string(os.PathSeparator)) || strings.HasPrefix(rel, "..") || strings.ToLower(filepath.Ext(rel)) != ".desktop" {
		return os.ErrPermission
	}
	info, err := os.Lstat(cleanPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return os.ErrPermission
	}
	name, execLine, ok := desktopEntryWebApp(cleanPath)
	if !ok || name != expectedName || execLine != expectedExec {
		return os.ErrPermission
	}
	return nil
}

func removeWebAppLauncher(pkg PackageResult) error {
	if err := validateWebAppDesktopEntry(pkg.Path, pkg.Name, pkg.ID); err != nil {
		return err
	}
	return os.Remove(pkg.Path)
}

func removeAppImageDesktopFiles(paths []string, appImagePath string) error {
	for _, path := range paths {
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := validateAppImageDesktopFile(path, appImagePath); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}
