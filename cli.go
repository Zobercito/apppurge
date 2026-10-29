package main

import (
	"fmt"
	"io"
	"strings"
)

type cliOptions struct {
	searchTerm string
	source     string
	dryRun     bool
	complete   bool
	help       bool
	version    bool
}

func parseCLIOptions(args []string) (cliOptions, error) {
	var options cliOptions
	var terms []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			options.help = true
		case arg == "--version":
			options.version = true
		case arg == "--dry-run":
			options.dryRun = true
		case arg == "--complete":
			options.complete = true
		case arg == "--source":
			if i+1 >= len(args) {
				return cliOptions{}, fmt.Errorf("--source requiere una fuente")
			}
			i++
			source, ok := parseSource(args[i])
			if !ok {
				return cliOptions{}, fmt.Errorf("fuente desconocida %q; usa apt, pacman, flatpak, snap, appimage o webapp", args[i])
			}
			options.source = source
		case strings.HasPrefix(arg, "--source="):
			source, ok := parseSource(strings.TrimPrefix(arg, "--source="))
			if !ok {
				return cliOptions{}, fmt.Errorf("fuente desconocida en %q; usa apt, pacman, flatpak, snap, appimage o webapp", arg)
			}
			options.source = source
		case strings.HasPrefix(arg, "-"):
			return cliOptions{}, fmt.Errorf("opción desconocida %q; usa --help para ver las opciones", arg)
		default:
			terms = append(terms, arg)
		}
	}
	options.searchTerm = strings.Join(terms, " ")
	if options.complete && !options.dryRun {
		return cliOptions{}, fmt.Errorf("--complete solo se puede usar junto con --dry-run")
	}
	if options.dryRun && strings.TrimSpace(options.searchTerm) == "" {
		return cliOptions{}, fmt.Errorf("--dry-run requiere un término de búsqueda")
	}
	return options, nil
}

func parseSource(value string) (string, bool) {
	switch strings.ToLower(value) {
	case "apt":
		return SourceAPT, true
	case "pacman":
		return SourcePacman, true
	case "flatpak":
		return SourceFlatpak, true
	case "snap":
		return SourceSnap, true
	case "appimage":
		return SourceAppImage, true
	case "webapp", "web-app":
		return SourceWebApp, true
	default:
		return "", false
	}
}

func runCLIDryRun(w io.Writer, term, source, mode string) error {
	scan := ScanAll(term)
	packages := make([]PackageResult, 0, len(scan.Packages))
	for _, pkg := range scan.Packages {
		if source == "" || pkg.Source == source {
			packages = append(packages, pkg)
		}
	}
	if len(packages) == 0 {
		for _, scanErr := range scan.Errors {
			if source == "" || scanErr.Source == source {
				if _, err := fmt.Fprintf(w, "Aviso: %s\n", scanErr); err != nil {
					return fmt.Errorf("escribir aviso de búsqueda: %w", err)
				}
			}
		}
		return fmt.Errorf("no se encontró un programa instalado que coincida con %q", term)
	}
	if len(packages) > 1 {
		if _, err := fmt.Fprintf(w, "La búsqueda coincide con %d programas; concreta el término o elige una fuente:\n", len(packages)); err != nil {
			return fmt.Errorf("escribir resultados de búsqueda: %w", err)
		}
		for _, pkg := range packages {
			if _, err := fmt.Fprintf(w, "  - %s [%s]\n", safeDisplayText(pkg.Name), pkg.Source); err != nil {
				return fmt.Errorf("escribir resultados de búsqueda: %w", err)
			}
		}
		return fmt.Errorf("la simulación requiere exactamente un programa coincidente")
	}

	pkg := packages[0]
	plan, err := DryRun(pkg, mode)
	if err != nil {
		return err
	}
	write := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	if err := write("Simulación para %s [%s]\n", safeDisplayText(pkg.Name), pkg.Source); err != nil {
		return fmt.Errorf("escribir simulación: %w", err)
	}
	if pkg.Source == SourceFlatpak {
		if err := write("Instalación: %s · ID: %s\n", safeDisplayText(pkg.Installation), safeDisplayText(pkg.ID)); err != nil {
			return fmt.Errorf("escribir simulación: %w", err)
		}
	}
	if err := write("Tamaño estimado del paquete (sin dependencias ni datos de usuario): %s\n", FormatBytes(plan.EstimateBytes)); err != nil {
		return fmt.Errorf("escribir simulación: %w", err)
	}
	for _, command := range plan.Commands {
		if err := write("Se ejecutaría: %s\n", safeDisplayText(command)); err != nil {
			return fmt.Errorf("escribir simulación: %w", err)
		}
	}
	if plan.RemovalPreviewErr != "" {
		if err := write("Vista previa de paquetes adicionales no disponible: %s\n", safeDisplayText(plan.RemovalPreviewErr)); err != nil {
			return fmt.Errorf("escribir simulación: %w", err)
		}
	} else {
		for _, name := range plan.PackagesToRemove {
			if err := write("También se quitaría el paquete: %s\n", safeDisplayText(name)); err != nil {
				return fmt.Errorf("escribir simulación: %w", err)
			}
		}
	}
	for _, dir := range plan.DirsToRemove {
		if err := write("Se eliminaría la carpeta de datos: %s\n", safeDisplayText(dir)); err != nil {
			return fmt.Errorf("escribir simulación: %w", err)
		}
	}
	for _, file := range plan.FilesToRemove {
		if err := write("Se eliminaría el acceso del menú: %s\n", safeDisplayText(file)); err != nil {
			return fmt.Errorf("escribir simulación: %w", err)
		}
	}
	return nil
}
