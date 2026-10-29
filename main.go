package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const helpText = `🗑  AppPurge - Desinstalador interactivo

Uso:
  apppurge                         Iniciar el modo interactivo
  apppurge <nombre>                Buscar un programa
  apppurge --source apt <nombre>  Filtrar por fuente
  apppurge --dry-run <nombre>     Simular una desinstalación
  apppurge --dry-run --complete <nombre>  Simular el modo completo
  apppurge --version              Mostrar la versión
  apppurge -h, --help             Mostrar esta ayuda

Fuentes disponibles:
  • APT (paquetes del sistema Debian/Ubuntu)
  • Pacman (paquetes del sistema Arch/Omarchy)
  • Flatpak
  • Snap
  • AppImage (archivos .AppImage en tu carpeta personal)
  • Web App (accesos creados por Omarchy)

Modos de desinstalación:
  • Normal:    Quitar el programa y conservar los datos de usuario
  • Completa: Quitar el programa y las rutas de datos confirmadas

Opciones:
  --source apt|pacman|flatpak|snap|appimage|webapp
  --dry-run  Mostrar un plan sin desinstalar
  --complete Usar el modo completo con --dry-run
`

// version can be set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

func main() {
	options, err := parseCLIOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\nUsa --help para ver las opciones.\n", err)
		os.Exit(2)
	}
	if options.version {
		fmt.Printf("apppurge %s\n", version)
		return
	}
	if options.help {
		fmt.Print(helpText)
		return
	}
	if options.dryRun {
		mode := modeNormal
		if options.complete {
			mode = modeComplete
		}
		if err := runCLIDryRun(os.Stdout, options.searchTerm, options.source, mode); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	model := InitModel(options.searchTerm)
	model.sourceFilter = options.source
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
