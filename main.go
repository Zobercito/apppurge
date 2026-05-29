package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const helpText = `🗑  AppPurge - Desinstalador interactivo de programas

Uso:
  appurge              Inicia el modo interactivo (te pide el nombre)
  appurge <nombre>     Busca directamente el programa indicado
  appurge -h, --help   Muestra esta ayuda

Fuentes soportadas:
  • APT (paquetes del sistema)
  • Flatpak
  • Snap
  • AppImage (archivos .AppImage en tu home)

Modos de desinstalación:
  • Normal:   Elimina el programa, conserva tus configuraciones
  • Completa: Elimina el programa y sus datos (purga conservadora)
`

func main() {
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "-h" || arg == "--help" {
			fmt.Print(helpText)
			os.Exit(0)
		}
	}

	term := ""
	if len(os.Args) > 1 {
		term = strings.Join(os.Args[1:], " ")
	}

	p := tea.NewProgram(InitModel(term), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
