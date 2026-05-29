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

// Uninstall executes the appropriate uninstall commands based on source and mode.
func Uninstall(pkg PackageResult, mode string) error {
	if !isValidPkgName(pkg.Name) {
		return fmt.Errorf("nombre de paquete inválido: %s", pkg.Name)
	}
	if isProtected(pkg) {
		return fmt.Errorf("paquete protegido: %s", pkg.Name)
	}

	start := time.Now()
	var err error

	switch pkg.Source {
	case SourceAPT:
		err = uninstallAPT(pkg, mode)
	case SourceFlatpak:
		err = uninstallFlatpak(pkg, mode)
	case SourceSnap:
		err = uninstallSnap(pkg, mode)
	case SourceAppImage:
		err = uninstallAppImage(pkg, mode)
	default:
		err = fmt.Errorf("fuente desconocida: %s", pkg.Source)
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
		fmt.Fprintf(os.Stderr, "advertencia: no se pudo guardar historial: %v\n", err)
	}

	return err
}

func uninstallAPT(pkg PackageResult, mode string) error {
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
		if err := removeUserData(pkg.Name); err != nil {
			fmt.Fprintf(os.Stderr, "advertencia: %v\n", err)
		}
	}
	return nil
}

func uninstallFlatpak(pkg PackageResult, mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("no se pudo obtener el directorio home: %w", err)
	}

	target := pkg.ID
	if target == "" {
		target = pkg.Name
	}
	var args []string
	if mode == modeComplete {
		args = []string{"flatpak", "uninstall", "--delete-data", "-y", target}
	} else {
		args = []string{"flatpak", "uninstall", "-y", target}
	}
	if err := runCmdCtx(ctx, args...); err != nil {
		return err
	}
	if mode == modeComplete && pkg.ID != "" {
		_ = os.RemoveAll(filepath.Join(home, ".var", "app", pkg.ID))
	}
	return nil
}

func uninstallSnap(pkg PackageResult, mode string) error {
	ctx, cancel := context.WithTimeout(context.Background(), uninstallTimeout)
	defer cancel()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("no se pudo obtener el directorio home: %w", err)
	}

	if err := runPrivilegedCtx(ctx, "snap", "remove", pkg.Name); err != nil {
		return err
	}
	if mode == modeComplete {
		_ = os.RemoveAll(filepath.Join(home, "snap", pkg.Name))
	}
	return nil
}

func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("ruta vacía")
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("ruta contiene ..")
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
		if strings.HasPrefix(cleaned, dir) {
			return nil
		}
	}
	return fmt.Errorf("ruta no está en directorios permitidos")
}

func uninstallAppImage(pkg PackageResult, mode string) error {
	if err := validatePath(pkg.Path); err != nil {
		return fmt.Errorf("ruta inválida: %w", err)
	}
	if err := os.Remove(pkg.Path); err != nil {
		return fmt.Errorf("no se pudo eliminar %s: %w", pkg.Path, err)
	}
	if mode == modeComplete {
		if err := removeUserData(pkg.Name); err != nil {
			fmt.Fprintf(os.Stderr, "advertencia: %v\n", err)
		}
	}
	return nil
}

// removeUserData removes common config/data directories for a package.
// Only removes directories that exist and are non-empty.
func removeUserData(name string) error {
	if !isValidPkgName(name) {
		return fmt.Errorf("nombre de paquete inválido para borrar datos: %s", name)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("no se pudo obtener el directorio home: %w", err)
	}

	names := []string{strings.ToLower(name), name}
	for _, n := range names {
		if !isValidPkgName(n) {
			continue
		}
		configPath := filepath.Join(home, ".config", n)
		if info, err := os.Stat(configPath); err == nil && info.IsDir() {
			if err := os.RemoveAll(configPath); err != nil {
				return fmt.Errorf("no se pudo eliminar directorio %s: %w", configPath, err)
			}
		}
		localPath := filepath.Join(home, ".local", "share", n)
		if info, err := os.Stat(localPath); err == nil && info.IsDir() {
			if err := os.RemoveAll(localPath); err != nil {
				return fmt.Errorf("no se pudo eliminar directorio %s: %w", localPath, err)
			}
		}
	}
	return nil
}

// runPrivilegedCtx runs a command with sudo if not already root.
func runPrivilegedCtx(ctx context.Context, args ...string) error {
	if os.Geteuid() != 0 {
		args = append([]string{"sudo", "--prompt=Contraseña de sudo: "}, args...)
	}
	return runCmdCtx(ctx, args...)
}

// runCmdCtx executes a command with context timeout and returns any error.
func runCmdCtx(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("no se proporcionaron argumentos")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("el comando excedió el tiempo límite (%v)", uninstallTimeout)
	}
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
